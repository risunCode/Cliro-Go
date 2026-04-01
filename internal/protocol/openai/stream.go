package openai

import (
	"time"

	contract "cliro-go/internal/contract"
)

type ChatStreamChunk struct {
	ID      string             `json:"id"`
	Object  string             `json:"object"`
	Created int64              `json:"created"`
	Model   string             `json:"model"`
	Choices []ChatStreamChoice `json:"choices"`
}

type ChatStreamChoice struct {
	Index        int            `json:"index"`
	Delta        map[string]any `json:"delta"`
	FinishReason any            `json:"finish_reason"`
}

type CompletionsStreamChunk struct {
	ID      string                    `json:"id"`
	Object  string                    `json:"object"`
	Created int64                     `json:"created"`
	Model   string                    `json:"model"`
	Choices []CompletionsStreamChoice `json:"choices"`
}

type CompletionsStreamChoice struct {
	Index            int    `json:"index"`
	Text             string `json:"text"`
	ReasoningContent string `json:"reasoning_content,omitempty"`
	FinishReason     any    `json:"finish_reason"`
}

func IRStreamToChunk(id string, model string, event contract.Event) ChatStreamChunk {
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

	return ChatStreamChunk{
		ID:      id,
		Object:  "chat.completion.chunk",
		Created: time.Now().Unix(),
		Model:   model,
		Choices: []ChatStreamChoice{{
			Index:        0,
			Delta:        delta,
			FinishReason: finishReason,
		}},
	}
}
