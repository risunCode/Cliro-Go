package codex

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	models "cliro/internal/proxy/models"

	"github.com/google/uuid"
)

func ResponsesToIR(req ResponsesRequest) (models.Request, error) {
	if err := validateModel(req.Model); err != nil {
		return models.Request{}, err
	}

	metadata := make(map[string]any)
	mergeMetadata(metadata, req.Metadata)
	if strings.TrimSpace(req.PreviousResponseID) != "" {
		metadata["previousResponseID"] = strings.TrimSpace(req.PreviousResponseID)
	}
	if req.ParallelToolCalls != nil {
		metadata["parallelToolCalls"] = *req.ParallelToolCalls
	}
	if req.Store != nil {
		metadata["store"] = *req.Store
	}

	messages := responseInputToIRMessages(req.Input, metadata)
	if instructions := strings.TrimSpace(req.Instructions); instructions != "" {
		messages = append([]models.Message{{Role: models.RoleSystem, Content: instructions}}, messages...)
	}
	if len(messages) == 0 {
		messages = []models.Message{{Role: models.RoleUser, Content: req.Input}}
	}
	messages = sanitizeToolHistory(messages)

	tools := make([]models.Tool, 0, len(req.Tools))
	for _, tool := range req.Tools {
		toolType := strings.TrimSpace(tool.Type)
		if toolType == "" {
			toolType = "function"
		}
		tools = append(tools, models.Tool{
			Type:        toolType,
			Name:        tool.Function.Name,
			Description: tool.Function.Description,
			Schema:      tool.Function.Parameters,
		})
	}

	return models.Request{
		Protocol:    models.ProtocolOpenAI,
		Endpoint:    models.EndpointOpenAIResponses,
		Model:       req.Model,
		Thinking:    parseThinkingConfig(req.Reasoning),
		Messages:    messages,
		Stream:      req.Stream,
		Temperature: req.Temperature,
		TopP:        req.TopP,
		MaxTokens:   req.MaxOutputTokens,
		Tools:       tools,
		ToolChoice:  req.ToolChoice,
		User:        req.User,
		Metadata:    metadata,
	}, nil
}

func ChatToIR(req ChatRequest) (models.Request, error) {
	if err := validateModel(req.Model); err != nil {
		return models.Request{}, err
	}

	messages := make([]models.Message, 0, len(req.Messages))
	for _, msg := range req.Messages {
		toolCalls := make([]models.ToolCall, 0, len(msg.ToolCalls))
		for _, toolCall := range msg.ToolCalls {
			id := strings.TrimSpace(toolCall.ID)
			if id == "" {
				id = "toolu_" + uuid.NewString()[:8]
			}
			toolCalls = append(toolCalls, models.ToolCall{
				ID:        id,
				Name:      toolCall.Function.Name,
				Arguments: toolCall.Function.Arguments,
			})
		}
		messages = append(messages, models.Message{
			Role:           roleFromString(msg.Role),
			Content:        normalizeOpenAIMessageContent(msg.Content, roleFromString(msg.Role)),
			Name:           msg.Name,
			ToolCalls:      toolCalls,
			ToolCallID:     msg.ToolCallID,
			ThinkingBlocks: thinkingBlocksFromAdditionalKwargs(msg.AdditionalKwargs),
		})
	}
	messages = sanitizeToolHistory(messages)

	tools := make([]models.Tool, 0, len(req.Tools))
	for _, tool := range req.Tools {
		toolType := strings.TrimSpace(tool.Type)
		if toolType == "" {
			toolType = "function"
		}
		tools = append(tools, models.Tool{
			Type:        toolType,
			Name:        tool.Function.Name,
			Description: tool.Function.Description,
			Schema:      tool.Function.Parameters,
		})
	}

	return models.Request{
		Protocol:    models.ProtocolOpenAI,
		Endpoint:    models.EndpointOpenAIChat,
		Model:       req.Model,
		Thinking:    parseThinkingConfig(req.Reasoning),
		Messages:    messages,
		Stream:      req.Stream,
		Temperature: req.Temperature,
		TopP:        req.TopP,
		MaxTokens:   req.MaxTokens,
		Tools:       tools,
		ToolChoice:  req.ToolChoice,
		User:        req.User,
		Metadata:    metadataFromChatRequest(req),
	}, nil
}

func CompletionsToIR(req CompletionsRequest) (models.Request, error) {
	if err := validateModel(req.Model); err != nil {
		return models.Request{}, err
	}

	messages := []models.Message{{
		Role:    models.RoleUser,
		Content: req.Prompt,
	}}

	return models.Request{
		Protocol:    models.ProtocolOpenAI,
		Endpoint:    models.EndpointOpenAICompletions,
		Model:       req.Model,
		Messages:    messages,
		Stream:      req.Stream,
		Temperature: req.Temperature,
		MaxTokens:   req.MaxTokens,
		User:        req.User,
		Metadata:    map[string]any{},
	}, nil
}

func responseInputToIRMessages(input any, metadata map[string]any) []models.Message {
	switch typed := input.(type) {
	case nil:
		return nil
	case string:
		if strings.TrimSpace(typed) == "" {
			return nil
		}
		return []models.Message{{Role: models.RoleUser, Content: typed}}
	case []any:
		messages := make([]models.Message, 0, len(typed))
		for _, item := range typed {
			messages = append(messages, responsesInputItemToIRMessages(item, metadata)...)
		}
		return messages
	case map[string]any:
		return responsesInputItemToIRMessages(typed, metadata)
	default:
		return []models.Message{{Role: models.RoleUser, Content: typed}}
	}
}

func responsesInputItemToIRMessages(item any, metadata map[string]any) []models.Message {
	object, ok := item.(map[string]any)
	if !ok {
		return responseInputToIRMessages(item, metadata)
	}

	if additional, ok := object["additional_kwargs"].(map[string]any); ok {
		captureAdditionalKwargsMetadata(additional, metadata)
	}

	itemType := strings.ToLower(strings.TrimSpace(asString(object["type"])))
	if itemType == "" && strings.TrimSpace(asString(object["role"])) != "" {
		itemType = "message"
	}

	switch itemType {
	case "message":
		role := roleFromString(strings.TrimSpace(asString(object["role"])))
		if role == "" {
			role = models.RoleUser
		}
		content := normalizeOpenAIMessageContent(object["content"], role)
		if models.ContentEmpty(content) {
			return nil
		}
		return []models.Message{{Role: role, Content: content}}
	case "function_call", "custom_tool_call":
		callID := strings.TrimSpace(asString(object["call_id"]))
		if callID == "" {
			callID = "toolu_" + uuid.NewString()[:8]
		}
		name := strings.TrimSpace(asString(object["name"]))
		arguments := strings.TrimSpace(asString(object["arguments"]))
		if itemType == "custom_tool_call" {
			arguments = strings.TrimSpace(asString(object["input"]))
		}
		if arguments == "" {
			if encoded, err := json.Marshal(object["input"]); err == nil && string(encoded) != "null" {
				arguments = string(encoded)
			}
		}
		return []models.Message{{
			Role: models.RoleAssistant,
			ToolCalls: []models.ToolCall{{
				ID:        callID,
				Name:      name,
				Arguments: firstNonEmpty(arguments, "{}"),
			}},
		}}
	case "function_call_output", "custom_tool_call_output":
		toolCallID := strings.TrimSpace(asString(object["call_id"]))
		if toolCallID == "" {
			return nil
		}
		return []models.Message{{Role: models.RoleTool, ToolCallID: toolCallID, Content: []models.ContentBlock{models.ToolResultContentBlock(toolCallID, normalizeResponsesToolOutput(object["output"]), false, nil)}}}
	case "input_text":
		text := strings.TrimSpace(asString(object["text"]))
		if text == "" {
			return nil
		}
		return []models.Message{{Role: models.RoleUser, Content: text}}
	case "output_text", "text", "refusal":
		text := strings.TrimSpace(asString(object["text"]))
		if text == "" {
			text = strings.TrimSpace(asString(object["refusal"]))
		}
		if text == "" {
			return nil
		}
		return []models.Message{{Role: models.RoleAssistant, Content: text}}
	default:
		content := normalizeOpenAIMessageContent(object["content"], roleFromString(strings.TrimSpace(asString(object["role"]))))
		if models.ContentEmpty(content) {
			content = strings.TrimSpace(asString(object["text"]))
		}
		if models.ContentEmpty(content) {
			encoded, _ := json.Marshal(object)
			content = strings.TrimSpace(string(encoded))
		}
		return []models.Message{{Role: models.RoleUser, Content: content}}
	}
}

func normalizeOpenAIMessageContent(content any, role models.Role) any {
	switch typed := content.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(typed)
	case []models.ContentBlock:
		return append([]models.ContentBlock(nil), typed...)
	case []any:
		blocks := make([]models.ContentBlock, 0, len(typed))
		for _, item := range typed {
			partObject, ok := item.(map[string]any)
			if !ok {
				if text := strings.TrimSpace(asString(item)); text != "" {
					blocks = append(blocks, models.TextBlock(text))
				}
				continue
			}
			partType := strings.ToLower(strings.TrimSpace(asString(partObject["type"])))
			switch partType {
			case "input_text", "output_text", "text":
				if text := strings.TrimSpace(asString(partObject["text"])); text != "" {
					blocks = append(blocks, models.TextBlock(text))
				}
			case "input_image", "image", "image_url":
				if imageBlock, ok := openAIImageToContentBlock(partObject); ok {
					blocks = append(blocks, imageBlock)
				}
			case "refusal":
				if text := strings.TrimSpace(asString(partObject["refusal"])); text != "" {
					blocks = append(blocks, models.TextBlock(text))
				}
			case "tool_result":
				toolCallID := strings.TrimSpace(asString(partObject["tool_call_id"]))
				if toolCallID != "" {
					blocks = append(blocks, models.ToolResultContentBlock(toolCallID, normalizeResponsesToolOutput(partObject["content"]), false, nil))
				}
			case "thinking", "reasoning":
				text := strings.TrimSpace(asString(partObject["thinking"]))
				if text == "" {
					text = strings.TrimSpace(asString(partObject["text"]))
				}
				if text != "" {
					blocks = append(blocks, models.ThinkingContentBlock(text, strings.TrimSpace(asString(partObject["signature"]))))
				}
			default:
				if text := strings.TrimSpace(asString(partObject["text"])); text != "" {
					blocks = append(blocks, models.TextBlock(text))
				}
			}
		}
		if len(blocks) == 0 {
			return ""
		}
		if len(blocks) == 1 && blocks[0].Type == models.ContentTypeText {
			return blocks[0].Text
		}
		return blocks
	case map[string]any:
		return normalizeOpenAIMessageContent([]any{typed}, role)
	default:
		encoded, _ := json.Marshal(typed)
		return strings.TrimSpace(string(encoded))
	}
}

func openAIImageToContentBlock(block map[string]any) (models.ContentBlock, bool) {
	if imageURL, ok := block["image_url"].(map[string]any); ok {
		if rawURL, _ := imageURL["url"].(string); strings.TrimSpace(rawURL) != "" {
			if strings.HasPrefix(strings.TrimSpace(rawURL), "data:") {
				mediaType, data := splitDataURL(rawURL)
				if strings.TrimSpace(data) != "" {
					return models.ImageDataBlock(mediaType, data), true
				}
			}
			return models.ImageURLBlock("", rawURL), true
		}
	}
	if source, ok := block["source"].(map[string]any); ok {
		data, _ := source["data"].(string)
		mediaType, _ := source["media_type"].(string)
		if strings.TrimSpace(data) != "" {
			return models.ImageDataBlock(mediaType, data), true
		}
	}
	return models.ContentBlock{}, false
}

func splitDataURL(raw string) (string, string) {
	trimmed := strings.TrimSpace(raw)
	if !strings.HasPrefix(trimmed, "data:") {
		return "", ""
	}
	comma := strings.Index(trimmed, ",")
	if comma <= 5 {
		return "", ""
	}
	meta := trimmed[5:comma]
	data := trimmed[comma+1:]
	if semi := strings.Index(meta, ";"); semi >= 0 {
		meta = meta[:semi]
	}
	return meta, strings.TrimSpace(data)
}

func normalizeResponsesToolOutput(output any) string {
	switch typed := output.(type) {
	case nil:
		return ""
	case string:
		return typed
	default:
		encoded, _ := json.Marshal(typed)
		return strings.TrimSpace(string(encoded))
	}
}

func metadataFromChatRequest(req ChatRequest) map[string]any {
	metadata := make(map[string]any)
	mergeMetadata(metadata, req.Metadata)
	for _, message := range req.Messages {
		captureAdditionalKwargsMetadata(message.AdditionalKwargs, metadata)
	}
	return metadata
}

func captureAdditionalKwargsMetadata(additional map[string]any, metadata map[string]any) {
	if len(additional) == 0 || metadata == nil {
		return
	}
	if conversationID, ok := additional["conversationId"].(string); ok && strings.TrimSpace(conversationID) != "" {
		metadata["conversationId"] = strings.TrimSpace(conversationID)
	}
	if continuationID, ok := additional["continuationId"].(string); ok && strings.TrimSpace(continuationID) != "" {
		metadata["continuationId"] = strings.TrimSpace(continuationID)
	}
	if profileARN, ok := additional["profileArn"].(string); ok && strings.TrimSpace(profileARN) != "" {
		metadata["profileArn"] = strings.TrimSpace(profileARN)
	}
}

func thinkingBlocksFromAdditionalKwargs(additional map[string]any) []models.ThinkingBlock {
	if len(additional) == 0 {
		return nil
	}
	raw, ok := additional["thinking_blocks"]
	if !ok {
		return nil
	}

	switch typed := raw.(type) {
	case []models.ThinkingBlock:
		return append([]models.ThinkingBlock(nil), typed...)
	case []any:
		blocks := make([]models.ThinkingBlock, 0, len(typed))
		for _, item := range typed {
			block, ok := item.(map[string]any)
			if !ok {
				continue
			}
			thinking, _ := block["thinking"].(string)
			signature, _ := block["signature"].(string)
			if strings.TrimSpace(thinking) == "" && strings.TrimSpace(signature) == "" {
				continue
			}
			blocks = append(blocks, models.ThinkingBlock{Thinking: thinking, Signature: signature})
		}
		if len(blocks) == 0 {
			return nil
		}
		return blocks
	default:
		return nil
	}
}

func mergeMetadata(dst map[string]any, src map[string]any) {
	if dst == nil || len(src) == 0 {
		return
	}
	for key, value := range src {
		dst[key] = value
	}
}

func roleFromString(value string) models.Role {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "system":
		return models.RoleSystem
	case "developer":
		return models.RoleDeveloper
	case "assistant":
		return models.RoleAssistant
	case "tool":
		return models.RoleTool
	default:
		return models.RoleUser
	}
}

func validateModel(model string) error {
	if strings.TrimSpace(model) == "" {
		return fmt.Errorf("model is required")
	}
	return nil
}

func sanitizeToolHistory(messages []models.Message) []models.Message {
	if len(messages) == 0 {
		return nil
	}

	declared := make(map[string]struct{})
	seenResults := make(map[string]struct{})
	sanitized := make([]models.Message, 0, len(messages))
	for _, message := range messages {
		current := message
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
			if len(current.ToolCalls) == 0 && strings.TrimSpace(models.ContentText(current.Content)) == "" {
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

func openAIContentToText(content any) string {
	if text := models.ContentText(content); strings.TrimSpace(text) != "" {
		return strings.TrimSpace(text)
	}
	switch typed := content.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(typed)
	case []any:
		parts := make([]string, 0, len(typed))
		for _, item := range typed {
			text := strings.TrimSpace(openAIContentToText(item))
			if text != "" {
				parts = append(parts, text)
			}
		}
		return strings.TrimSpace(strings.Join(parts, "\n"))
	case map[string]any:
		for _, key := range []string{"text", "output", "content", "refusal"} {
			if value, ok := typed[key]; ok {
				text := strings.TrimSpace(openAIContentToText(value))
				if text != "" {
					return text
				}
			}
		}
		encoded, _ := json.Marshal(typed)
		return strings.TrimSpace(string(encoded))
	default:
		encoded, _ := json.Marshal(typed)
		return strings.TrimSpace(string(encoded))
	}
}

func parseThinkingConfig(reasoning map[string]any) models.ThinkingConfig {
	if len(reasoning) == 0 {
		return models.ThinkingConfig{}
	}
	return models.ThinkingConfig{
		Requested: true,
		RawParams: reasoning,
	}
}

func asString(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	default:
		encoded, _ := json.Marshal(typed)
		return strings.TrimSpace(string(encoded))
	}
}

func IRToChat(resp models.Response) ChatResponse {
	message := ChatMessage{Role: "assistant", Content: resp.Text}
	if resp.Thinking != "" {
		message.ReasoningContent = resp.Thinking
		if message.AdditionalKwargs == nil {
			message.AdditionalKwargs = make(map[string]any)
		}
		message.AdditionalKwargs["thinking_blocks"] = []map[string]any{{
			"thinking":  resp.Thinking,
			"signature": resp.ThinkingSignature,
		}}
	}
	finishReason := firstNonEmpty(resp.StopReason, "stop")
	if len(resp.ToolCalls) > 0 {
		message.ToolCalls = irToolCallsToOpenAIToolCalls(resp.ToolCalls)
		message.Content = nil
		if finishReason == "stop" {
			finishReason = "tool_calls"
		}
	}

	return ChatResponse{
		ID:      resp.ID,
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   resp.Model,
		Choices: []ChatChoice{{
			Index:        0,
			Message:      message,
			FinishReason: finishReason,
		}},
		Usage: Usage{
			PromptTokens:     resp.Usage.PromptTokens,
			CompletionTokens: resp.Usage.CompletionTokens,
			TotalTokens:      resp.Usage.TotalTokens,
		},
	}
}

func IRToCompletions(resp models.Response) CompletionsResponse {
	return CompletionsResponse{
		ID:      resp.ID,
		Object:  "text_completion",
		Created: time.Now().Unix(),
		Model:   resp.Model,
		Choices: []CompletionsChoice{{
			Index:            0,
			Text:             resp.Text,
			ReasoningContent: resp.Thinking,
			FinishReason:     firstNonEmpty(resp.StopReason, "stop"),
		}},
		Usage: Usage{
			PromptTokens:     resp.Usage.PromptTokens,
			CompletionTokens: resp.Usage.CompletionTokens,
			TotalTokens:      resp.Usage.TotalTokens,
		},
	}
}

func IRToResponses(resp models.Response) ResponsesResponse {
	output := make([]ResponsesOutputItem, 0, 1+len(resp.ToolCalls))
	if strings.TrimSpace(resp.Text) != "" || strings.TrimSpace(resp.Thinking) != "" || len(resp.ToolCalls) == 0 {
		messageContent := ResponsesContentPart{
			Type:        "output_text",
			Text:        resp.Text,
			Annotations: []any{},
		}
		if resp.Thinking != "" {
			messageContent.ReasoningContent = resp.Thinking
		}
		output = append(output, ResponsesOutputItem{
			ID:      resp.ID,
			Type:    "message",
			Role:    "assistant",
			Status:  "completed",
			Content: []ResponsesContentPart{messageContent},
		})
	}
	for _, call := range resp.ToolCalls {
		name := strings.TrimSpace(call.Name)
		if name == "" {
			continue
		}
		id := strings.TrimSpace(call.ID)
		if id == "" {
			id = "fc_" + uuid.NewString()[:8]
		}
		arguments := strings.TrimSpace(call.Arguments)
		if arguments == "" {
			arguments = "{}"
		}
		output = append(output, ResponsesOutputItem{
			ID:        "fc_" + id,
			Type:      "function_call",
			Status:    "completed",
			CallID:    id,
			Name:      name,
			Arguments: arguments,
		})
	}

	return ResponsesResponse{
		ID:         firstNonEmpty(resp.ID, "resp_"+uuid.NewString()[:8]),
		Object:     "response",
		CreatedAt:  time.Now().Unix(),
		Status:     "completed",
		Model:      resp.Model,
		Output:     output,
		OutputText: resp.Text,
		Usage: ResponsesUsage{
			InputTokens:  resp.Usage.PromptTokens,
			OutputTokens: resp.Usage.CompletionTokens,
			TotalTokens:  resp.Usage.TotalTokens,
		},
	}
}
