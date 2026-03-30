package encode

import (
	"encoding/json"
	"strings"

	"cliro-go/internal/adapter/ir"
	"cliro-go/internal/protocol/anthropic"

	"github.com/google/uuid"
)

func IRToAnthropicMessages(resp ir.Response) anthropic.MessagesResponse {
	content := []map[string]any{}
	if resp.Thinking != "" {
		content = append(content, map[string]any{
			"type":      "thinking",
			"thinking":  resp.Thinking,
			"signature": generateThinkingSignature(),
		})
	}
	if resp.Text != "" {
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
		input := map[string]any{}
		if strings.TrimSpace(call.Arguments) != "" {
			_ = json.Unmarshal([]byte(call.Arguments), &input)
		}
		content = append(content, map[string]any{
			"type":  "tool_use",
			"id":    id,
			"name":  name,
			"input": input,
		})
	}

	stopReason := firstNonEmpty(resp.StopReason, "end_turn")
	if stopReason == "tool_calls" || len(resp.ToolCalls) > 0 {
		stopReason = "tool_use"
	}

	return anthropic.MessagesResponse{
		ID:           "msg_" + uuid.NewString(),
		Type:         "message",
		Role:         "assistant",
		Model:        resp.Model,
		Content:      content,
		StopReason:   stopReason,
		StopSequence: nil,
		Usage: anthropic.Usage{
			InputTokens:  resp.Usage.PromptTokens,
			OutputTokens: resp.Usage.CompletionTokens,
		},
	}
}

func generateThinkingSignature() string {
	raw := strings.ReplaceAll(uuid.NewString(), "-", "")
	if len(raw) > 32 {
		raw = raw[:32]
	}
	return "sig_" + raw
}
