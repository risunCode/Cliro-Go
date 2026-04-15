package codex

import (
	"encoding/json"
	"fmt"
	"strings"

	models "cliro/internal/proxy/models"

	"github.com/google/uuid"
)

func (s *Service) buildRequestPayload(req ChatRequest) (map[string]any, error) {
	payload, _, err := s.buildRequestPayloadWithToolNames(req)
	return payload, err
}

func (s *Service) buildRequestPayloadWithToolNames(req ChatRequest) (map[string]any, ToolNameMapping, error) {
	mapping := BuildToolNameMapping(req.Tools, req.Messages, DefaultToolNameLimit)
	input := make([]any, 0, len(req.Messages))
	for _, msg := range req.Messages {
		items := s.codexMessageItems(msg, mapping)
		input = append(input, items...)
	}
	if len(input) == 0 {
		return nil, mapping, fmt.Errorf("messages are empty")
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
	if req.Thinking.Requested && len(req.Thinking.RawParams) > 0 {
		params := make(map[string]any)
		for k, v := range req.Thinking.RawParams {
			if k == "budget_tokens" {
				if effort := budgetTokensToEffort(v); effort != "" {
					params["effort"] = effort
				}
			} else if k == "effort" {
				params[k] = v
			}
		}
		if effort, ok := params["effort"].(string); ok && effort == "none" {
			// -none suffix: explicitly disable reasoning block
			delete(payload, "reasoning")
		} else if len(params) > 0 {
			if _, hasSummary := params["summary"]; !hasSummary {
				params["summary"] = "auto"
			}
			payload["reasoning"] = params
		}
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
		payload["tools"] = s.codexTools(req.Tools, mapping)
	}
	if req.ToolChoice != nil && req.ToolChoice != "" {
		payload["tool_choice"] = req.ToolChoice
	}
	return payload, mapping, nil
}

func (s *Service) codexMessageItems(msg Message, mapping ToolNameMapping) []any {
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
			name := mapping.Remap(toolCall.Function.Name)
			if name == "" {
				continue
			}
			arguments := strings.TrimSpace(toolCall.Function.Arguments)
			if arguments == "" {
				arguments = "{}"
			}
			items = append(items, map[string]any{"type": "function_call", "call_id": firstNonEmpty(toolCall.ID, "toolu_"+uuid.NewString()[:8]), "name": name, "arguments": arguments})
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

func (s *Service) codexTools(tools []Tool, mapping ToolNameMapping) []any {
	converted := make([]any, 0, len(tools))
	for _, tool := range tools {
		name := mapping.Remap(tool.Function.Name)
		if name == "" {
			continue
		}
		schema, _ := NormalizeToolSchema(tool.Function.Parameters).(map[string]any)
		converted = append(converted, map[string]any{
			"type":        "function",
			"name":        name,
			"description": strings.TrimSpace(tool.Function.Description),
			"parameters":  repairToolSchema(schema),
		})
	}
	if len(converted) == 0 {
		return nil
	}
	return converted
}

// repairToolSchema walks JSON Schema properties and injects a missing "items: {}"
// on any property declared as type "array" without an items field. This prevents
// upstream Codex 400 errors when tool schemas are incomplete.
func repairToolSchema(schema map[string]any) map[string]any {
	if schema == nil {
		return schema
	}
	props, ok := schema["properties"].(map[string]any)
	if !ok {
		return schema
	}
	for key, val := range props {
		prop, ok := val.(map[string]any)
		if !ok {
			continue
		}
		if t, _ := prop["type"].(string); t == "array" {
			if _, hasItems := prop["items"]; !hasItems {
				prop["items"] = map[string]any{}
				props[key] = prop
			}
		}
		// Recurse into nested object schemas
		if nested, ok := prop["properties"].(map[string]any); ok {
			_ = nested
			prop["properties"] = repairToolSchema(prop)["properties"]
			props[key] = prop
		}
	}
	schema["properties"] = props
	return schema
}

func messageToText(content any) string {
	if text := models.ContentText(content); strings.TrimSpace(text) != "" {
		return text
	}
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

func budgetTokensToEffort(budgetTokens any) string {
	var tokens int
	switch v := budgetTokens.(type) {
	case int:
		tokens = v
	case float64:
		tokens = int(v)
	default:
		return ""
	}

	if tokens <= 0 {
		return ""
	} else if tokens <= 6000 {
		return "low"
	} else if tokens <= 12000 {
		return "medium"
	} else if tokens <= 24000 {
		return "high"
	} else {
		return "xhigh"
	}
}
