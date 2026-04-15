package codex

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	models "cliro/internal/proxy/models"
	sharedproxy "cliro/internal/proxy/shared"
)

func StopReasonForResponse(resp models.Response) string {
	if len(resp.ToolCalls) > 0 {
		return "tool_calls"
	}
	return "stop"
}

func StreamOpenAIChat(w http.ResponseWriter, requestedModel string, response models.Response, newID func() string, nowUnix func() int64) error {
	flusher, ok := w.(http.Flusher)
	if !ok {
		return fmt.Errorf("streaming not supported")
	}
	chatID := sharedproxy.FirstNonEmpty(strings.TrimSpace(response.ID), "chatcmpl-"+newID())
	model := sharedproxy.FirstNonEmpty(strings.TrimSpace(requestedModel), strings.TrimSpace(response.Model))
	state := &StreamState{}
	initialChunk := ChatStreamChunk{ID: chatID, Object: "chat.completion.chunk", Created: nowUnix(), Model: model, Choices: []ChatStreamChoice{{Index: 0, Delta: map[string]any{"role": "assistant"}, FinishReason: nil}}}
	if err := WriteSSEChunk(w, initialChunk); err != nil {
		return err
	}
	state.Started = true
	flusher.Flush()
	for _, chunk := range sharedproxy.ChunkText(response.Thinking, 160) {
		event := models.Event{ThinkDelta: chunk}
		if !state.Apply(event) {
			continue
		}
		if err := WriteSSEChunk(w, IRStreamToChunk(chatID, model, event)); err != nil {
			return err
		}
		flusher.Flush()
	}
	for _, chunk := range sharedproxy.ChunkText(response.Text, 160) {
		event := models.Event{TextDelta: chunk}
		if !state.Apply(event) {
			continue
		}
		if err := WriteSSEChunk(w, IRStreamToChunk(chatID, model, event)); err != nil {
			return err
		}
		flusher.Flush()
	}
	for index, toolCall := range EncodeIRToolCallsForStream(response.ToolCalls) {
		event := models.Event{ToolDelta: []map[string]any{{"index": index, "id": toolCall.ID, "type": "function", "function": toolCall.Function}}}
		if !state.Apply(event) {
			continue
		}
		if err := WriteSSEChunk(w, IRStreamToChunk(chatID, model, event)); err != nil {
			return err
		}
		flusher.Flush()
	}
	finishReason := sharedproxy.FirstNonEmpty(strings.TrimSpace(response.StopReason), "stop")
	finalEvent := models.Event{Done: true, Type: finishReason}
	if !state.Apply(finalEvent) {
		return nil
	}
	if err := WriteSSEChunk(w, IRStreamToChunk(chatID, model, finalEvent)); err != nil {
		return err
	}
	_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
	flusher.Flush()
	return nil
}

func StreamOpenAICompletions(w http.ResponseWriter, requestedModel string, response models.Response, newID func() string) error {
	flusher, ok := w.(http.Flusher)
	if !ok {
		return fmt.Errorf("streaming not supported")
	}
	completionID := sharedproxy.FirstNonEmpty(strings.TrimSpace(response.ID), "cmpl-"+newID())
	model := sharedproxy.FirstNonEmpty(strings.TrimSpace(requestedModel), strings.TrimSpace(response.Model))
	for _, chunk := range sharedproxy.ChunkText(response.Thinking, 160) {
		payload := CompletionsStreamChunk{ID: completionID, Object: "text_completion", Created: time.Now().Unix(), Model: model, Choices: []CompletionsStreamChoice{{Index: 0, ReasoningContent: chunk, FinishReason: nil}}}
		encoded, _ := json.Marshal(payload)
		_, _ = fmt.Fprintf(w, "data: %s\n\n", encoded)
		flusher.Flush()
	}
	for _, chunk := range sharedproxy.ChunkText(response.Text, 160) {
		payload := CompletionsStreamChunk{ID: completionID, Object: "text_completion", Created: time.Now().Unix(), Model: model, Choices: []CompletionsStreamChoice{{Index: 0, Text: chunk, FinishReason: nil}}}
		encoded, _ := json.Marshal(payload)
		_, _ = fmt.Fprintf(w, "data: %s\n\n", encoded)
		flusher.Flush()
	}
	finishReason := sharedproxy.FirstNonEmpty(strings.TrimSpace(response.StopReason), "stop")
	finalChunk := CompletionsStreamChunk{ID: completionID, Object: "text_completion", Created: time.Now().Unix(), Model: model, Choices: []CompletionsStreamChoice{{Index: 0, Text: "", FinishReason: finishReason}}}
	encoded, _ := json.Marshal(finalChunk)
	_, _ = fmt.Fprintf(w, "data: %s\n\n", encoded)
	_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
	flusher.Flush()
	return nil
}

func StreamOpenAIResponses(w http.ResponseWriter, requestedModel string, response models.Response, newID func() string, nowUnix func() int64) error {
	flusher, ok := w.(http.Flusher)
	if !ok {
		return fmt.Errorf("streaming not supported")
	}
	responseID := sharedproxy.FirstNonEmpty(strings.TrimSpace(response.ID), "resp_"+newID())
	model := sharedproxy.FirstNonEmpty(strings.TrimSpace(requestedModel), strings.TrimSpace(response.Model))
	createdAt := nowUnix()
	state := NewResponsesStreamState(responseID, model, createdAt, func(event string, payload map[string]any) { WriteResponsesSSEEvent(w, event, payload); flusher.Flush() })
	state.Start()
	if strings.TrimSpace(response.Text) != "" || strings.TrimSpace(response.Thinking) != "" || len(response.ToolCalls) == 0 {
		for _, chunk := range sharedproxy.ChunkText(response.Thinking, 160) {
			state.EmitReasoningDelta(chunk)
		}
		for _, chunk := range sharedproxy.ChunkText(response.Text, 160) {
			state.EmitTextDelta(chunk)
		}
		state.CloseMessageItem(response.Text, response.Thinking)
	}
	for _, toolCall := range response.ToolCalls {
		state.EmitFunctionCall(toolCall)
	}
	state.Complete(IRToResponses(response))
	return nil
}
