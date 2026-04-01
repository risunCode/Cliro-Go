package anthropic

import (
	"encoding/json"
	"strings"

	contract "cliro-go/internal/contract"
	"cliro-go/internal/contract/rules"

	"github.com/google/uuid"
)

func IRToMessages(resp contract.Response) MessagesResponse {
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
	return contract.StableThinkingSignature(thinking)
}

func resolveThinkingSignature(thinking string, signature string) string {
	trimmed := strings.TrimSpace(signature)
	if trimmed != "" {
		return trimmed
	}
	return contract.StableThinkingSignature(thinking)
}

func remappedToolCallArguments(name string, arguments string) map[string]any {
	input := map[string]any{}
	if strings.TrimSpace(arguments) != "" {
		_ = json.Unmarshal([]byte(arguments), &input)
	}
	return rules.RemapToolCallArgs(name, input)
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

func anthropicInputTokens(usage contract.Usage) int {
	if usage.InputTokens > 0 {
		return usage.InputTokens
	}
	return usage.PromptTokens
}

func anthropicOutputTokens(usage contract.Usage) int {
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
