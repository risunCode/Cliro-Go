package anthropic

import contract "cliro-go/internal/contract"

type StreamEvent struct {
	Type  string `json:"type"`
	Index int    `json:"index,omitempty"`
	Delta any    `json:"delta,omitempty"`
}

func IRStreamToEvent(event contract.Event) map[string]any {
	if event.Done {
		return map[string]any{"type": "message_stop"}
	}
	if event.SignatureDelta != "" {
		return map[string]any{
			"type":  "content_block_delta",
			"index": 0,
			"delta": map[string]any{
				"type":      "signature_delta",
				"signature": event.SignatureDelta,
			},
		}
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
