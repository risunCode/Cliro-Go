package encode

import (
	"time"

	"cliro-go/internal/adapter/ir"
	"cliro-go/internal/protocol/openai"
)

func IRStreamToOpenAIChunk(id string, model string, event ir.Event) openai.ChatStreamChunk {
	delta := map[string]any{}
	if event.TextDelta != "" {
		delta["content"] = event.TextDelta
	}
	if event.ThinkDelta != "" {
		delta["reasoning_content"] = event.ThinkDelta
	}
	if event.ToolDelta != nil {
		delta["tool_calls"] = event.ToolDelta
	}

	finishReason := any(nil)
	if event.Done {
		if event.Type != "" {
			finishReason = event.Type
		} else {
			finishReason = "stop"
		}
	}

	return openai.ChatStreamChunk{
		ID:      id,
		Object:  "chat.completion.chunk",
		Created: time.Now().Unix(),
		Model:   model,
		Choices: []openai.ChatStreamChoice{{
			Index:        0,
			Delta:        delta,
			FinishReason: finishReason,
		}},
	}
}
