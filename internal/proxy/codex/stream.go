package codex

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	models "cliro/internal/proxy/models"

	"github.com/google/uuid"
)

func irToolCallsToOpenAIToolCalls(calls []models.ToolCall) []map[string]any {
	out := make([]map[string]any, 0, len(calls))
	for _, call := range calls {
		name := strings.TrimSpace(call.Name)
		if name == "" {
			continue
		}
		id := strings.TrimSpace(call.ID)
		if id == "" {
			id = "toolu_" + uuid.NewString()[:8]
		}
		arguments := strings.TrimSpace(call.Arguments)
		if arguments == "" {
			arguments = "{}"
		}

		if !json.Valid([]byte(arguments)) {
			encoded, _ := json.Marshal(map[string]any{"value": arguments})
			arguments = string(encoded)
		}

		out = append(out, map[string]any{
			"id":   id,
			"type": "function",
			"function": map[string]any{
				"name":      name,
				"arguments": arguments,
			},
		})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func EncodeIRToolCallsForStream(calls []models.ToolCall) []StreamToolCall {
	base := irToolCallsToOpenAIToolCalls(calls)
	out := make([]StreamToolCall, 0, len(base))
	for _, item := range base {
		function, _ := item["function"].(map[string]any)
		out = append(out, StreamToolCall{ID: item["id"].(string), Function: function})
	}
	return out
}

type StreamToolCall struct {
	ID       string
	Function map[string]any
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

type StreamState struct {
	Started      bool
	HasText      bool
	HasThinking  bool
	HasToolCalls bool
	Completed    bool
}

func (s *StreamState) Apply(event models.Event) bool {
	if s == nil {
		return true
	}
	if s.Completed {
		return false
	}
	s.Started = true
	if event.TextDelta != "" {
		s.HasText = true
	}
	if event.ThinkDelta != "" {
		s.HasThinking = true
	}
	if event.ToolDelta != nil {
		s.HasToolCalls = true
	}
	if event.Done {
		s.Completed = true
	}
	return true
}

type ResponsesStreamEventEmitter func(eventName string, payload map[string]any)

type ResponsesStreamState struct {
	emit         ResponsesStreamEventEmitter
	responseID   string
	model        string
	createdAt    int64
	started      bool
	messageOpen  bool
	messageIndex int
	messageItem  string
	nextIndex    int
	reasoningID  string
}

func NewResponsesStreamState(responseID string, model string, createdAt int64, emit ResponsesStreamEventEmitter) *ResponsesStreamState {
	if emit == nil {
		emit = func(string, map[string]any) {}
	}
	if createdAt <= 0 {
		createdAt = time.Now().Unix()
	}
	return &ResponsesStreamState{emit: emit, responseID: responseID, model: model, createdAt: createdAt}
}

func (s *ResponsesStreamState) Start() {
	if s == nil || s.started {
		return
	}
	s.started = true
	payload := map[string]any{
		"id":         s.responseID,
		"object":     "response",
		"created_at": s.createdAt,
		"status":     "in_progress",
		"model":      s.model,
		"output":     []any{},
	}
	s.emit("response.created", map[string]any{"type": "response.created", "response": payload})
	s.emit("response.in_progress", map[string]any{"type": "response.in_progress", "response": payload})
}

func (s *ResponsesStreamState) EmitReasoningDelta(delta string) {
	if s == nil || strings.TrimSpace(delta) == "" {
		return
	}
	if s.reasoningID == "" {
		s.reasoningID = "rs_" + uuid.NewString()[:8]
	}
	s.emit("response.reasoning_summary_text.delta", map[string]any{
		"type":          "response.reasoning_summary_text.delta",
		"response_id":   s.responseID,
		"item_id":       s.reasoningID,
		"output_index":  s.messageIndex,
		"summary_index": 0,
		"delta":         delta,
	})
	if s.nextIndex <= s.messageIndex {
		s.nextIndex = s.messageIndex + 1
	}
}

func (s *ResponsesStreamState) EmitTextDelta(delta string) {
	if s == nil || delta == "" {
		return
	}
	s.ensureMessageItem()
	s.emit("response.output_text.delta", map[string]any{
		"type":          "response.output_text.delta",
		"response_id":   s.responseID,
		"item_id":       s.messageItem,
		"output_index":  s.messageIndex,
		"content_index": 0,
		"delta":         delta,
	})
	if s.nextIndex <= s.messageIndex {
		s.nextIndex = s.messageIndex + 1
	}
}

func (s *ResponsesStreamState) CloseMessageItem(text string, reasoning string) {
	if s == nil {
		return
	}
	s.ensureMessageItem()
	if !s.messageOpen {
		return
	}
	part := map[string]any{
		"type":        "output_text",
		"text":        text,
		"annotations": []any{},
	}
	if strings.TrimSpace(reasoning) != "" {
		part["reasoning_content"] = reasoning
	}
	s.emit("response.output_text.done", map[string]any{
		"type":              "response.output_text.done",
		"response_id":       s.responseID,
		"item_id":           s.messageItem,
		"output_index":      s.messageIndex,
		"content_index":     0,
		"text":              text,
		"reasoning_content": reasoning,
	})
	s.emit("response.content_part.done", map[string]any{
		"type":          "response.content_part.done",
		"response_id":   s.responseID,
		"item_id":       s.messageItem,
		"output_index":  s.messageIndex,
		"content_index": 0,
		"part":          part,
	})
	s.emit("response.output_item.done", map[string]any{
		"type":         "response.output_item.done",
		"response_id":  s.responseID,
		"output_index": s.messageIndex,
		"item": map[string]any{
			"id":      s.messageItem,
			"type":    "message",
			"role":    "assistant",
			"status":  "completed",
			"content": []any{part},
		},
	})
	s.messageOpen = false
	if s.nextIndex <= s.messageIndex {
		s.nextIndex = s.messageIndex + 1
	}
}

func (s *ResponsesStreamState) EmitFunctionCall(call models.ToolCall) {
	if s == nil {
		return
	}
	name := strings.TrimSpace(call.Name)
	if name == "" {
		return
	}
	itemID := strings.TrimSpace(call.ID)
	if itemID == "" {
		itemID = "fc_" + uuid.NewString()[:8]
	}
	arguments := strings.TrimSpace(call.Arguments)
	if arguments == "" {
		arguments = "{}"
	}
	outputIndex := s.nextIndex
	s.nextIndex++
	s.emit("response.output_item.added", map[string]any{
		"type":         "response.output_item.added",
		"response_id":  s.responseID,
		"output_index": outputIndex,
		"item": map[string]any{
			"id":        itemID,
			"call_id":   itemID,
			"type":      "function_call",
			"name":      name,
			"arguments": "",
			"status":    "in_progress",
		},
	})
	for _, chunk := range chunkStreamText(arguments, 160) {
		s.emit("response.function_call_arguments.delta", map[string]any{
			"type":         "response.function_call_arguments.delta",
			"response_id":  s.responseID,
			"item_id":      itemID,
			"output_index": outputIndex,
			"delta":        chunk,
		})
	}
	s.emit("response.function_call_arguments.done", map[string]any{
		"type":         "response.function_call_arguments.done",
		"response_id":  s.responseID,
		"item_id":      itemID,
		"output_index": outputIndex,
		"arguments":    arguments,
	})
	s.emit("response.output_item.done", map[string]any{
		"type":         "response.output_item.done",
		"response_id":  s.responseID,
		"output_index": outputIndex,
		"item": map[string]any{
			"id":        itemID,
			"call_id":   itemID,
			"type":      "function_call",
			"name":      name,
			"arguments": arguments,
			"status":    "completed",
		},
	})
}

func (s *ResponsesStreamState) Complete(response ResponsesResponse) {
	if s == nil {
		return
	}
	s.emit("response.completed", map[string]any{"type": "response.completed", "response": response})
}

func (s *ResponsesStreamState) ensureMessageItem() {
	if s == nil || s.messageOpen {
		return
	}
	if s.messageItem == "" {
		s.messageItem = "msg_" + uuid.NewString()[:8]
	}
	s.messageIndex = s.nextIndex
	s.nextIndex++
	s.messageOpen = true
	s.emit("response.output_item.added", map[string]any{
		"type":         "response.output_item.added",
		"response_id":  s.responseID,
		"output_index": s.messageIndex,
		"item": map[string]any{
			"id":      s.messageItem,
			"type":    "message",
			"role":    "assistant",
			"status":  "in_progress",
			"content": []any{},
		},
	})
	s.emit("response.content_part.added", map[string]any{
		"type":          "response.content_part.added",
		"response_id":   s.responseID,
		"item_id":       s.messageItem,
		"output_index":  s.messageIndex,
		"content_index": 0,
		"part": map[string]any{
			"type":        "output_text",
			"text":        "",
			"annotations": []any{},
		},
	})
}

func chunkStreamText(text string, size int) []string {
	trimmed := text
	if size <= 0 || len(trimmed) <= size {
		if trimmed == "" {
			return nil
		}
		return []string{trimmed}
	}
	chunks := make([]string, 0, (len(trimmed)+size-1)/size)
	for len(trimmed) > 0 {
		if len(trimmed) <= size {
			chunks = append(chunks, trimmed)
			break
		}
		chunks = append(chunks, trimmed[:size])
		trimmed = trimmed[size:]
	}
	return chunks
}

type ChatStreamChunk struct {
	ID      string             `json:"id"`
	Object  string             `json:"object"`
	Created int64              `json:"created"`
	Model   string             `json:"model"`
	Choices []ChatStreamChoice `json:"choices"`
}

func WriteSSEChunk(w http.ResponseWriter, chunk ChatStreamChunk) error {
	encoded, err := json.Marshal(chunk)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "data: %s\n\n", encoded)
	return err
}

func WriteResponsesSSEEvent(w http.ResponseWriter, event string, payload any) {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return
	}
	_, _ = fmt.Fprintf(w, "event: %s\n", strings.TrimSpace(event))
	_, _ = fmt.Fprintf(w, "data: %s\n\n", encoded)
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

func IRStreamToChunk(id string, model string, event models.Event) ChatStreamChunk {
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
