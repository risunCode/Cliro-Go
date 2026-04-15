package anthropic

import (
	"encoding/json"
	"fmt"
	"strings"

	models "cliro/internal/proxy/models"

	"github.com/google/uuid"
)

const anthropicFallbackUserContent = "."

func MessagesToIR(req MessagesRequest) (models.Request, error) {
	model := strings.TrimSpace(req.Model)
	if model == "" {
		return models.Request{}, fmt.Errorf("model is required")
	}

	messages := make([]models.Message, 0, len(req.Messages)+1)
	if systemText := strings.TrimSpace(anthropicSystemToText(req.System)); systemText != "" {
		messages = append(messages, models.Message{Role: models.RoleSystem, Content: systemText})
	}
	for _, message := range req.Messages {
		messages = append(messages, anthropicMessageToIRMessages(message)...)
	}
	messages = mergeConsecutiveMessages(messages)
	messages = sanitizeToolHistory(messages)
	if len(messages) == 0 {
		return models.Request{}, fmt.Errorf("messages are empty")
	}

	tools := make([]models.Tool, 0, len(req.Tools))
	for _, tool := range req.Tools {
		toolType := strings.TrimSpace(tool.Type)
		if toolType == "" {
			toolType = "function"
		}
		name := strings.TrimSpace(tool.Name)
		if name == "" {
			name = toolType
		}
		if name == "" {
			continue
		}
		tools = append(tools, models.Tool{
			Type:        toolType,
			Name:        name,
			Description: strings.TrimSpace(tool.Description),
			Schema:      tool.InputSchema,
		})
	}

	var maxTokens *int
	if req.MaxTokens > 0 {
		maxTokens = &req.MaxTokens
	}

	return models.Request{
		Protocol:    models.ProtocolAnthropic,
		Endpoint:    models.EndpointAnthropicMessages,
		Model:       model,
		Thinking:    parseThinkingConfig(req.Thinking),
		Messages:    messages,
		Stream:      req.Stream,
		Temperature: req.Temperature,
		TopP:        req.TopP,
		MaxTokens:   maxTokens,
		Tools:       tools,
		ToolChoice:  req.ToolChoice,
		User:        strings.TrimSpace(req.User),
		Metadata:    map[string]any{},
	}, nil
}

func anthropicMessageToIRMessages(message Message) []models.Message {
	messageContent := stripAnthropicCacheControl(message.Content)
	role := strings.ToLower(strings.TrimSpace(message.Role))
	if role == "" {
		role = "user"
	}
	if role != "assistant" {
		role = "user"
	}

	text, toolCalls, toolResults, thinkingBlocks := convertAnthropicMessageContent(role, messageContent)
	if role == "assistant" {
		if strings.TrimSpace(text) == "" && len(toolCalls) == 0 && len(thinkingBlocks) == 0 {
			return nil
		}
		assistantMessage := models.Message{Role: models.RoleAssistant, ToolCalls: toolCalls, ThinkingBlocks: thinkingBlocks}
		if strings.TrimSpace(text) != "" {
			assistantMessage.Content = buildAnthropicCanonicalContent(text, nil, nil, thinkingBlocks)
		}
		return []models.Message{assistantMessage}
	}

	out := append([]models.Message(nil), toolResults...)
	if strings.TrimSpace(text) != "" {
		out = append(out, models.Message{Role: models.RoleUser, Content: buildAnthropicCanonicalContent(text, nil, nil, thinkingBlocks), ThinkingBlocks: thinkingBlocks})
	}
	return out
}

func mergeConsecutiveMessages(messages []models.Message) []models.Message {
	if len(messages) <= 1 {
		return messages
	}

	merged := make([]models.Message, 0, len(messages))
	current := cloneMessage(messages[0])
	for idx := 1; idx < len(messages); idx++ {
		next := messages[idx]
		if current.Role != next.Role {
			merged = append(merged, current)
			current = cloneMessage(next)
			continue
		}
		if current.Role == models.RoleTool && strings.TrimSpace(current.ToolCallID) != strings.TrimSpace(next.ToolCallID) {
			merged = append(merged, current)
			current = cloneMessage(next)
			continue
		}

		current.Content = mergeMessageContent(current.Content, next.Content)
		current.ToolCalls = append(current.ToolCalls, next.ToolCalls...)
		current.Name = firstNonEmpty(current.Name, next.Name)
		current.ToolCallID = firstNonEmpty(current.ToolCallID, next.ToolCallID)
		current.ThinkingBlocks = append(current.ThinkingBlocks, next.ThinkingBlocks...)
	}

	merged = append(merged, current)
	return merged
}

func cloneMessage(message models.Message) models.Message {
	cloned := message
	if len(message.ToolCalls) > 0 {
		cloned.ToolCalls = append([]models.ToolCall(nil), message.ToolCalls...)
	}
	if len(message.ThinkingBlocks) > 0 {
		cloned.ThinkingBlocks = append([]models.ThinkingBlock(nil), message.ThinkingBlocks...)
	}
	return cloned
}

func sanitizeToolHistory(messages []models.Message) []models.Message {
	if len(messages) == 0 {
		return nil
	}

	declared := make(map[string]struct{})
	seenResults := make(map[string]struct{})
	sanitized := make([]models.Message, 0, len(messages))
	for _, message := range messages {
		current := cloneMessage(message)
		if current.Role == models.RoleAssistant && len(current.ToolCalls) > 0 {
			filteredCalls := make([]models.ToolCall, 0, len(current.ToolCalls))
			seenCalls := make(map[string]struct{}, len(current.ToolCalls))
			for _, call := range current.ToolCalls {
				name := strings.TrimSpace(call.Name)
				if name == "" {
					continue
				}
				id := strings.TrimSpace(call.ID)
				if id == "" {
					id = "toolu_" + uuid.NewString()[:8]
				}
				if _, ok := seenCalls[id]; ok {
					continue
				}
				seenCalls[id] = struct{}{}
				declared[id] = struct{}{}
				call.ID = id
				call.Name = name
				if strings.TrimSpace(call.Arguments) == "" {
					call.Arguments = "{}"
				}
				filteredCalls = append(filteredCalls, call)
			}
			current.ToolCalls = filteredCalls
			if len(current.ToolCalls) == 0 && strings.TrimSpace(models.ContentText(current.Content)) == "" && len(current.ThinkingBlocks) == 0 {
				continue
			}
			sanitized = append(sanitized, current)
			continue
		}

		if current.Role == models.RoleTool {
			id := strings.TrimSpace(current.ToolCallID)
			if id == "" {
				current.Role = models.RoleUser
				current.ToolCallID = ""
				if strings.TrimSpace(models.ContentText(current.Content)) == "" {
					continue
				}
				sanitized = append(sanitized, current)
				continue
			}
			if _, ok := declared[id]; !ok {
				current.Role = models.RoleUser
				current.ToolCallID = ""
				if strings.TrimSpace(models.ContentText(current.Content)) == "" {
					continue
				}
				sanitized = append(sanitized, current)
				continue
			}
			if _, ok := seenResults[id]; ok {
				continue
			}
			seenResults[id] = struct{}{}
		}

		sanitized = append(sanitized, current)
	}
	return sanitized
}

func mergeMessageContent(current any, next any) any {
	left := strings.TrimSpace(messageContentToText(current))
	right := strings.TrimSpace(messageContentToText(next))
	switch {
	case left == "" && right == "":
		return nil
	case left == "":
		return right
	case right == "":
		return left
	default:
		return left + "\n\n" + right
	}
}

func appendVisibleTextFragment(parts []string, fragment string) []string {
	if strings.TrimSpace(fragment) == "" {
		return parts
	}
	return append(parts, fragment)
}

func messageContentToText(content any) string {
	switch typed := content.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(typed)
	case []any:
		parts := make([]string, 0, len(typed))
		for _, item := range typed {
			parts = appendVisibleTextFragment(parts, messageContentToText(item))
		}
		return strings.TrimSpace(strings.Join(parts, ""))
	case map[string]any:
		if text, ok := typed["text"].(string); ok && strings.TrimSpace(text) != "" {
			return strings.TrimSpace(text)
		}
		if thinking, ok := typed["thinking"].(string); ok && strings.TrimSpace(thinking) != "" {
			return strings.TrimSpace(thinking)
		}
		encoded, _ := json.Marshal(typed)
		return strings.TrimSpace(string(encoded))
	default:
		encoded, _ := json.Marshal(typed)
		return strings.TrimSpace(string(encoded))
	}
}

func convertAnthropicMessageContent(role string, content any) (string, []models.ToolCall, []models.Message, []models.ThinkingBlock) {
	switch typed := content.(type) {
	case string:
		return strings.TrimSpace(typed), nil, nil, nil
	case []any:
		textParts := make([]string, 0, len(typed))
		imageBlocks := make([]models.ContentBlock, 0)
		toolCalls := make([]models.ToolCall, 0)
		toolResults := make([]models.Message, 0)
		thinkingBlocks := make([]models.ThinkingBlock, 0)

		for _, item := range typed {
			sanitized := stripAnthropicCacheControl(item)
			block, ok := sanitized.(map[string]any)
			if !ok {
				textParts = appendVisibleTextFragment(textParts, anthropicContentToText(sanitized))
				continue
			}

			blockType, _ := block["type"].(string)
			switch strings.ToLower(strings.TrimSpace(blockType)) {
			case "text":
				if text, ok := block["text"].(string); ok {
					textParts = appendVisibleTextFragment(textParts, text)
				}
			case "image":
				if source, ok := block["source"].(map[string]any); ok {
					data, _ := source["data"].(string)
					mediaType, _ := source["media_type"].(string)
					if strings.TrimSpace(data) != "" || strings.TrimSpace(mediaType) != "" {
						imageBlocks = append(imageBlocks, models.ImageDataBlock(mediaType, data))
					}
				}
			case "thinking":
				if text, ok := block["thinking"].(string); ok && strings.TrimSpace(text) != "" {
					signature, _ := block["signature"].(string)
					thinkingBlocks = append(thinkingBlocks, models.ThinkingBlock{Thinking: text, Signature: strings.TrimSpace(signature)})
				}
			case "redacted_thinking":
				continue
			case "tool_use":
				if role != "assistant" {
					continue
				}
				name, _ := block["name"].(string)
				name = strings.TrimSpace(name)
				if name == "" {
					continue
				}
				id, _ := block["id"].(string)
				id = strings.TrimSpace(id)
				if id == "" {
					id = "toolu_" + uuid.NewString()[:8]
				}

				input := map[string]any{}
				if parsed, ok := block["input"].(map[string]any); ok && parsed != nil {
					input = parsed
				}
				encodedInput, _ := json.Marshal(input)

				toolCalls = append(toolCalls, models.ToolCall{
					ID:        id,
					Name:      name,
					Arguments: string(encodedInput),
				})
			case "tool_result":
				if role == "assistant" {
					continue
				}
				toolUseID, _ := block["tool_use_id"].(string)
				toolUseID = strings.TrimSpace(toolUseID)
				if toolUseID == "" {
					continue
				}
				toolContent := strings.TrimSpace(anthropicContentToText(stripAnthropicCacheControl(block["content"])))
				if toolContent == "" {
					toolContent = anthropicFallbackUserContent
				}
				toolResults = append(toolResults, models.Message{
					Role:       models.RoleTool,
					ToolCallID: toolUseID,
					Content:    []models.ContentBlock{models.ToolResultContentBlock(toolUseID, toolContent, false, contentImagesFromAnthropicToolResult(block["content"]))},
				})
			default:
				textParts = appendVisibleTextFragment(textParts, anthropicContentToText(block))
			}
		}

		toolResults = attachAnthropicImages(toolResults, imageBlocks)
		return strings.TrimSpace(strings.Join(textParts, "")), toolCalls, toolResults, thinkingBlocks
	default:
		return strings.TrimSpace(anthropicContentToText(stripAnthropicCacheControl(content))), nil, nil, nil
	}
}

func anthropicSystemToText(system any) string {
	switch typed := system.(type) {
	case string:
		return strings.TrimSpace(typed)
	case []any:
		parts := make([]string, 0, len(typed))
		for _, item := range typed {
			parts = appendVisibleTextFragment(parts, anthropicContentToText(item))
		}
		return strings.TrimSpace(strings.Join(parts, ""))
	default:
		return strings.TrimSpace(anthropicContentToText(system))
	}
}

func buildAnthropicCanonicalContent(text string, images []models.ContentBlock, toolResults []models.ContentBlock, thinkingBlocks []models.ThinkingBlock) any {
	blocks := make([]models.ContentBlock, 0, 1+len(images)+len(toolResults)+len(thinkingBlocks))
	if strings.TrimSpace(text) != "" {
		blocks = append(blocks, models.TextBlock(text))
	}
	blocks = append(blocks, images...)
	blocks = append(blocks, toolResults...)
	for _, thinking := range thinkingBlocks {
		blocks = append(blocks, models.ThinkingContentBlock(thinking.Thinking, thinking.Signature))
	}
	if len(blocks) == 0 {
		return ""
	}
	if len(blocks) == 1 && blocks[0].Type == models.ContentTypeText {
		return blocks[0].Text
	}
	return blocks
}

func contentImagesFromAnthropicToolResult(content any) []models.ImageBlock {
	blocks, ok := stripAnthropicCacheControl(content).([]any)
	if !ok {
		return nil
	}
	images := make([]models.ImageBlock, 0)
	for _, item := range blocks {
		block, ok := item.(map[string]any)
		if !ok {
			continue
		}
		blockType, _ := block["type"].(string)
		if strings.TrimSpace(strings.ToLower(blockType)) != "image" {
			continue
		}
		if source, ok := block["source"].(map[string]any); ok {
			data, _ := source["data"].(string)
			mediaType, _ := source["media_type"].(string)
			images = append(images, models.ImageBlock{MediaType: strings.TrimSpace(mediaType), Data: strings.TrimSpace(data)})
		}
	}
	return images
}

func attachAnthropicImages(toolResults []models.Message, images []models.ContentBlock) []models.Message {
	if len(images) == 0 {
		return toolResults
	}
	if len(toolResults) == 0 {
		return []models.Message{{Role: models.RoleUser, Content: images}}
	}
	return toolResults
}

func anthropicContentToText(content any) string {
	content = stripAnthropicCacheControl(content)
	switch typed := content.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(typed)
	case []any:
		parts := make([]string, 0, len(typed))
		for _, item := range typed {
			if block, ok := item.(map[string]any); ok {
				blockType, _ := block["type"].(string)
				switch strings.ToLower(strings.TrimSpace(blockType)) {
				case "text":
					if text, ok := block["text"].(string); ok {
						parts = appendVisibleTextFragment(parts, text)
					}
					continue
				case "thinking":
					if thinking, ok := block["thinking"].(string); ok {
						parts = appendVisibleTextFragment(parts, thinking)
					}
					continue
				case "redacted_thinking":
					if data := strings.TrimSpace(anthropicContentToText(block["data"])); data != "" {
						parts = append(parts, "[Redacted Thinking: "+data+"]")
					}
					continue
				case "tool_result":
					parts = appendVisibleTextFragment(parts, anthropicContentToText(block["content"]))
					continue
				case "tool_use":
					if name, ok := block["name"].(string); ok && strings.TrimSpace(name) != "" {
						parts = append(parts, name)
					}
					if input := strings.TrimSpace(anthropicContentToText(block["input"])); input != "" {
						parts = append(parts, input)
					}
					continue
				}
			}
			parts = appendVisibleTextFragment(parts, anthropicContentToText(item))
		}
		return strings.TrimSpace(strings.Join(parts, ""))
	case map[string]any:
		if blockType, _ := typed["type"].(string); strings.EqualFold(strings.TrimSpace(blockType), "redacted_thinking") {
			if data := strings.TrimSpace(anthropicContentToText(typed["data"])); data != "" {
				return "[Redacted Thinking: " + data + "]"
			}
		}
		if text, ok := typed["text"].(string); ok && strings.TrimSpace(text) != "" {
			return strings.TrimSpace(text)
		}
		if thinking, ok := typed["thinking"].(string); ok && strings.TrimSpace(thinking) != "" {
			return strings.TrimSpace(thinking)
		}
		encoded, _ := json.Marshal(typed)
		return strings.TrimSpace(string(encoded))
	default:
		encoded, _ := json.Marshal(typed)
		return strings.TrimSpace(string(encoded))
	}
}

func stripAnthropicCacheControl(value any) any {
	switch typed := value.(type) {
	case []any:
		cleaned := make([]any, 0, len(typed))
		for _, item := range typed {
			cleaned = append(cleaned, stripAnthropicCacheControl(item))
		}
		return cleaned
	case map[string]any:
		cleaned := make(map[string]any, len(typed))
		for key, item := range typed {
			if key == "cache_control" {
				continue
			}
			cleaned[key] = stripAnthropicCacheControl(item)
		}
		return cleaned
	default:
		return value
	}
}

func parseThinkingConfig(thinking map[string]any) models.ThinkingConfig {
	if len(thinking) == 0 {
		return models.ThinkingConfig{}
	}
	filtered := make(map[string]any)
	for k, v := range thinking {
		switch k {
		case "budget_tokens":
			filtered[k] = v
		case "effort":
			if budgetTokens := effortToBudgetTokens(v); budgetTokens > 0 {
				filtered["budget_tokens"] = budgetTokens
			}
		}
	}
	return models.ThinkingConfig{
		Requested: true,
		RawParams: filtered,
	}
}

func effortToBudgetTokens(effort any) int {
	effortStr, ok := effort.(string)
	if !ok {
		return 0
	}
	switch strings.ToLower(strings.TrimSpace(effortStr)) {
	case "low", "minimal":
		return 4096
	case "medium":
		return 10000
	case "high":
		return 16384
	case "xhigh":
		return 32768
	default:
		return 0
	}
}

func IRToMessages(resp models.Response) MessagesResponse {
	content := []map[string]any{}
	if strings.TrimSpace(resp.Thinking) != "" {
		content = append(content, map[string]any{
			"type":      "thinking",
			"thinking":  resp.Thinking,
			"signature": resolveThinkingSignature(resp.Thinking, resp.ThinkingSignature),
		})
	}
	if strings.TrimSpace(resp.Text) != "" {
		content = append(content, map[string]any{"type": "text", "text": resp.Text})
	}
	for _, call := range resp.ToolCalls {
		name := strings.TrimSpace(call.Name)
		if name == "" {
			continue
		}
		id := strings.TrimSpace(call.ID)
		if id == "" {
			id = "toolu_" + uuid.NewString()[:8]
		}
		input := remappedToolCallArguments(name, call.Arguments)
		content = append(content, map[string]any{
			"type":  "tool_use",
			"id":    id,
			"name":  name,
			"input": input,
		})
	}

	stopReason := anthropicStopReason(resp.StopReason, len(resp.ToolCalls) > 0)

	return MessagesResponse{
		ID:           anthropicMessageID(resp.ID),
		Type:         "message",
		Role:         "assistant",
		Model:        resp.Model,
		Content:      content,
		StopReason:   stopReason,
		StopSequence: nil,
		Usage: Usage{
			InputTokens:  anthropicInputTokens(resp.Usage),
			OutputTokens: anthropicOutputTokens(resp.Usage),
		},
	}
}

func StableThinkingSignature(thinking string) string {
	return models.StableThinkingSignature(thinking)
}

func resolveThinkingSignature(thinking string, signature string) string {
	trimmed := strings.TrimSpace(signature)
	if trimmed != "" {
		return trimmed
	}
	return models.StableThinkingSignature(thinking)
}

func remappedToolCallArguments(name string, arguments string) map[string]any {
	input := map[string]any{}
	if strings.TrimSpace(arguments) != "" {
		_ = json.Unmarshal([]byte(arguments), &input)
	}
	return models.RemapToolCallArgs(name, input)
}

func anthropicStopReason(stopReason string, hasToolCalls bool) string {
	if hasToolCalls {
		return "tool_use"
	}
	switch strings.TrimSpace(stopReason) {
	case "", "stop", "end_turn":
		return "end_turn"
	case "tool_calls", "tool_use":
		return "tool_use"
	default:
		return strings.TrimSpace(stopReason)
	}
}

func anthropicInputTokens(usage models.Usage) int {
	if usage.InputTokens > 0 {
		return usage.InputTokens
	}
	return usage.PromptTokens
}

func anthropicOutputTokens(usage models.Usage) int {
	if usage.OutputTokens > 0 {
		return usage.OutputTokens
	}
	return usage.CompletionTokens
}

func anthropicMessageID(candidate string) string {
	trimmed := strings.TrimSpace(candidate)
	if strings.HasPrefix(trimmed, "msg_") {
		return trimmed
	}
	return "msg_" + uuid.NewString()
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

