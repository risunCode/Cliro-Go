package encode

import "cliro-go/internal/adapter/ir"

func IRStreamToAnthropicEvent(event ir.Event) map[string]any {
	if event.Done {
		return map[string]any{"type": "message_stop"}
	}
	if event.ThinkDelta != "" {
		return map[string]any{
			"type":  "content_block_delta",
			"index": 0,
			"delta": map[string]any{
				"type":     "thinking_delta",
				"thinking": event.ThinkDelta,
			},
		}
	}
	return map[string]any{
		"type":  "content_block_delta",
		"index": 0,
		"delta": map[string]any{
			"type": "text_delta",
			"text": event.TextDelta,
		},
	}
}
