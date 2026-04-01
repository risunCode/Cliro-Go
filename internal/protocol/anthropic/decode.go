package anthropic

import (
	"cliro-go/internal/util"
	"encoding/json"
	"fmt"
	"strings"

	contract "cliro-go/internal/contract"

	"github.com/google/uuid"
)

const anthropicFallbackUserContent = "."

func MessagesToIR(req MessagesRequest) (contract.Request, error) {
	model := strings.TrimSpace(req.Model)
	if model == "" {
		return contract.Request{}, fmt.Errorf("model is required")
	}

	messages := make([]contract.Message, 0, len(req.Messages)+1)
	if systemText := strings.TrimSpace(anthropicSystemToText(req.System)); systemText != "" {
		messages = append(messages, contract.Message{Role: contract.RoleSystem, Content: systemText})
	}
	for _, message := range req.Messages {
		messages = append(messages, anthropicMessageToIRMessages(message)...)
	}
	messages = mergeConsecutiveMessages(messages)
	if len(messages) == 0 {
		return contract.Request{}, fmt.Errorf("messages are empty")
	}

	tools := make([]contract.Tool, 0, len(req.Tools))
	for _, tool := range req.Tools {
		name := strings.TrimSpace(tool.Name)
		if name == "" {
			continue
		}
		tools = append(tools, contract.Tool{
			Name:        name,
			Description: strings.TrimSpace(tool.Description),
			Schema:      tool.InputSchema,
		})
	}

	var maxTokens *int
	if req.MaxTokens > 0 {
		maxTokens = &req.MaxTokens
	}

	return contract.Request{
		Protocol:    contract.ProtocolAnthropic,
		Endpoint:    contract.EndpointAnthropicMessages,
		Model:       model,
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

func anthropicMessageToIRMessages(message Message) []contract.Message {
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
		assistantMessage := contract.Message{Role: contract.RoleAssistant, ToolCalls: toolCalls, ThinkingBlocks: thinkingBlocks}
		if strings.TrimSpace(text) != "" {
			assistantMessage.Content = text
		}
		return []contract.Message{assistantMessage}
	}

	out := append([]contract.Message(nil), toolResults...)
	if strings.TrimSpace(text) != "" {
		out = append(out, contract.Message{Role: contract.RoleUser, Content: text, ThinkingBlocks: thinkingBlocks})
	}
	return out
}

func mergeConsecutiveMessages(messages []contract.Message) []contract.Message {
	if len(messages) <= 1 {
		return messages
	}

	merged := make([]contract.Message, 0, len(messages))
	current := cloneMessage(messages[0])
	for idx := 1; idx < len(messages); idx++ {
		next := messages[idx]
		if current.Role != next.Role {
			merged = append(merged, current)
			current = cloneMessage(next)
			continue
		}
		if current.Role == contract.RoleTool && strings.TrimSpace(current.ToolCallID) != strings.TrimSpace(next.ToolCallID) {
			merged = append(merged, current)
			current = cloneMessage(next)
			continue
		}

		current.Content = mergeMessageContent(current.Content, next.Content)
		current.ToolCalls = append(current.ToolCalls, next.ToolCalls...)
		current.Name = util.FirstNonEmpty(current.Name, next.Name)
		current.ToolCallID = util.FirstNonEmpty(current.ToolCallID, next.ToolCallID)
		current.ThinkingBlocks = append(current.ThinkingBlocks, next.ThinkingBlocks...)
	}

	merged = append(merged, current)
	return merged
}

func cloneMessage(message contract.Message) contract.Message {
	cloned := message
	if len(message.ToolCalls) > 0 {
		cloned.ToolCalls = append([]contract.ToolCall(nil), message.ToolCalls...)
	}
	if len(message.ThinkingBlocks) > 0 {
		cloned.ThinkingBlocks = append([]contract.ThinkingBlock(nil), message.ThinkingBlocks...)
	}
	return cloned
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

func messageContentToText(content any) string {
	switch typed := content.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(typed)
	case []any:
		parts := make([]string, 0, len(typed))
		for _, item := range typed {
			text := strings.TrimSpace(messageContentToText(item))
			if text != "" {
				parts = append(parts, text)
			}
		}
		return strings.TrimSpace(strings.Join(parts, "\n"))
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

func convertAnthropicMessageContent(role string, content any) (string, []contract.ToolCall, []contract.Message, []contract.ThinkingBlock) {
	switch typed := content.(type) {
	case string:
		return strings.TrimSpace(typed), nil, nil, nil
	case []any:
		textParts := make([]string, 0, len(typed))
		toolCalls := make([]contract.ToolCall, 0)
		toolResults := make([]contract.Message, 0)
		thinkingBlocks := make([]contract.ThinkingBlock, 0)

		for _, item := range typed {
			sanitized := stripAnthropicCacheControl(item)
			block, ok := sanitized.(map[string]any)
			if !ok {
				fallback := strings.TrimSpace(anthropicContentToText(sanitized))
				if fallback != "" {
					textParts = append(textParts, fallback)
				}
				continue
			}

			blockType, _ := block["type"].(string)
			switch strings.ToLower(strings.TrimSpace(blockType)) {
			case "text":
				if text, ok := block["text"].(string); ok && strings.TrimSpace(text) != "" {
					textParts = append(textParts, text)
				}
			case "thinking":
				if text, ok := block["thinking"].(string); ok && strings.TrimSpace(text) != "" {
					signature, _ := block["signature"].(string)
					thinkingBlocks = append(thinkingBlocks, contract.ThinkingBlock{Thinking: text, Signature: strings.TrimSpace(signature)})
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

				toolCalls = append(toolCalls, contract.ToolCall{
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
				toolResults = append(toolResults, contract.Message{
					Role:       contract.RoleTool,
					ToolCallID: toolUseID,
					Content:    toolContent,
				})
			default:
				fallback := strings.TrimSpace(anthropicContentToText(block))
				if fallback != "" {
					textParts = append(textParts, fallback)
				}
			}
		}

		return strings.TrimSpace(strings.Join(textParts, "\n")), toolCalls, toolResults, thinkingBlocks
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
			text := strings.TrimSpace(anthropicContentToText(item))
			if text != "" {
				parts = append(parts, text)
			}
		}
		return strings.TrimSpace(strings.Join(parts, "\n"))
	default:
		return strings.TrimSpace(anthropicContentToText(system))
	}
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
					if text, ok := block["text"].(string); ok && strings.TrimSpace(text) != "" {
						parts = append(parts, text)
					}
					continue
				case "thinking":
					if thinking, ok := block["thinking"].(string); ok && strings.TrimSpace(thinking) != "" {
						parts = append(parts, thinking)
					}
					continue
				case "redacted_thinking":
					if data := strings.TrimSpace(anthropicContentToText(block["data"])); data != "" {
						parts = append(parts, "[Redacted Thinking: "+data+"]")
					}
					continue
				case "tool_result":
					contentText := strings.TrimSpace(anthropicContentToText(block["content"]))
					if contentText != "" {
						parts = append(parts, contentText)
					}
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
			fallback := strings.TrimSpace(anthropicContentToText(item))
			if fallback != "" {
				parts = append(parts, fallback)
			}
		}
		return strings.TrimSpace(strings.Join(parts, "\n"))
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

