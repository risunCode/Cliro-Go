package codex

import (
	"cliro-go/internal/util"
	"encoding/json"
	"fmt"
	"strings"

	"cliro-go/internal/provider"

	"github.com/google/uuid"
)

func (s *Service) buildRequestPayload(req provider.ChatRequest) (map[string]any, error) {
	input := make([]any, 0, len(req.Messages))
	for _, msg := range req.Messages {
		items := s.codexMessageItems(msg)
		input = append(input, items...)
	}
	if len(input) == 0 {
		return nil, fmt.Errorf("messages are empty")
	}
	payload := map[string]any{
		"model":               req.Model,
		"input":               input,
		"instructions":        defaultCodexInstructions(),
		"stream":              true,
		"store":               false,
		"include":             []string{"reasoning.encrypted_content"},
		"parallel_tool_calls": true,
	}
	if req.Metadata != nil {
		if previousResponseID, ok := req.Metadata["previousResponseID"].(string); ok && strings.TrimSpace(previousResponseID) != "" {
			payload["previous_response_id"] = strings.TrimSpace(previousResponseID)
		}
		if parallelToolCalls, ok := req.Metadata["parallelToolCalls"].(bool); ok {
			payload["parallel_tool_calls"] = parallelToolCalls
		}
		if instructions, ok := req.Metadata["instructions"].(string); ok && strings.TrimSpace(instructions) != "" {
			payload["instructions"] = defaultCodexInstructions() + "\n\n## Request Context\n\n" + strings.TrimSpace(instructions)
		}
	}
	if len(req.Tools) > 0 {
		payload["tools"] = s.codexTools(req.Tools)
	}
	if req.ToolChoice != nil && req.ToolChoice != "" {
		payload["tool_choice"] = req.ToolChoice
	}
	return payload, nil
}

func (s *Service) codexMessageItems(msg provider.Message) []any {
	role := strings.ToLower(strings.TrimSpace(msg.Role))
	switch role {
	case "system", "developer":
		text := strings.TrimSpace(messageToText(msg.Content))
		if text == "" {
			return nil
		}
		return []any{map[string]any{"type": "message", "role": "developer", "content": []any{map[string]any{"type": "input_text", "text": text}}}}
	case "assistant":
		items := make([]any, 0, 1+len(msg.ToolCalls))
		if text := strings.TrimSpace(messageToText(msg.Content)); text != "" {
			items = append(items, map[string]any{"type": "message", "role": "assistant", "content": []any{map[string]any{"type": "output_text", "text": text}}})
		}
		for _, toolCall := range msg.ToolCalls {
			name := strings.TrimSpace(toolCall.Function.Name)
			if name == "" {
				continue
			}
			arguments := strings.TrimSpace(toolCall.Function.Arguments)
			if arguments == "" {
				arguments = "{}"
			}
			items = append(items, map[string]any{"type": "function_call", "call_id": util.FirstNonEmpty(toolCall.ID, "toolu_"+uuid.NewString()[:8]), "name": name, "arguments": arguments})
		}
		return items
	case "tool":
		toolCallID := strings.TrimSpace(msg.ToolCallID)
		if toolCallID == "" {
			return nil
		}
		return []any{map[string]any{"type": "function_call_output", "call_id": toolCallID, "output": messageToText(msg.Content)}}
	default:
		text := strings.TrimSpace(messageToText(msg.Content))
		if text == "" {
			return nil
		}
		return []any{map[string]any{"type": "message", "role": "user", "content": []any{map[string]any{"type": "input_text", "text": text}}}}
	}
}

func (s *Service) codexTools(tools []provider.Tool) []any {
	converted := make([]any, 0, len(tools))
	for _, tool := range tools {
		name := strings.TrimSpace(tool.Function.Name)
		if name == "" {
			continue
		}
		converted = append(converted, map[string]any{
			"type":        "function",
			"name":        name,
			"description": strings.TrimSpace(tool.Function.Description),
			"parameters":  tool.Function.Parameters,
		})
	}
	if len(converted) == 0 {
		return nil
	}
	return converted
}

func messageToText(content any) string {
	switch typed := content.(type) {
	case nil:
		return ""
	case string:
		return typed
	case []any:
		parts := make([]string, 0, len(typed))
		for _, item := range typed {
			if object, ok := item.(map[string]any); ok {
				text, _ := object["text"].(string)
				if strings.TrimSpace(text) != "" {
					parts = append(parts, text)
				}
			}
		}
		return strings.Join(parts, "\n")
	default:
		data, _ := json.Marshal(typed)
		return strings.TrimSpace(string(data))
	}
}

