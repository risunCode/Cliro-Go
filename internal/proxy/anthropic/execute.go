package anthropic

import (
	"fmt"
	"net/http"
	"strings"

	models "cliro/internal/proxy/models"
	sharedproxy "cliro/internal/proxy/shared"
)

func StopReasonForResponse(resp models.Response) string {
	if len(resp.ToolCalls) > 0 {
		return "tool_use"
	}
	if resp.StopReason == "" || resp.StopReason == "stop" {
		return "end_turn"
	}
	return resp.StopReason
}

func StreamAnthropicMessages(w http.ResponseWriter, requestedModel string, response models.Response, newID func() string) error {
	flusher, ok := w.(http.Flusher)
	if !ok {
		return fmt.Errorf("streaming not supported")
	}
	messageID := "msg_" + newID()
	if strings.HasPrefix(strings.TrimSpace(response.ID), "msg_") {
		messageID = strings.TrimSpace(response.ID)
	}
	model := sharedproxy.FirstNonEmpty(strings.TrimSpace(requestedModel), strings.TrimSpace(response.Model))
	thinkingPresent := strings.TrimSpace(response.Thinking) != ""
	textPresent := strings.TrimSpace(response.Text) != ""
	thinkingSignature := resolveThinkingSignature(response.Thinking, response.ThinkingSignature)
	emit := func(eventName string, payload map[string]any) { WriteSSEEvent(w, eventName, payload); flusher.Flush() }
	streamState := NewMessageStreamState(messageID, model, emit)
	streamState.StartMessage(anthropicInputTokens(response.Usage))
	nextIndex := 0
	thinkingLifecycle := NewThinkingBlockLifecycle(nextIndex, emit)
	if thinkingPresent {
		for _, chunk := range sharedproxy.ChunkText(response.Thinking, 160) {
			thinkingLifecycle.EmitThinkingDelta(chunk)
		}
		nextIndex = thinkingLifecycle.PrepareForNextBlock(thinkingSignature)
		streamState.MarkIndex(nextIndex)
	}
	if textPresent {
		for _, chunk := range sharedproxy.ChunkText(response.Text, 160) {
			streamState.EmitTextDelta(chunk)
		}
		streamState.CloseTextBlock()
	}
	nextIndex = streamState.NextIndex()
	for _, toolCall := range response.ToolCalls {
		nextIndex = EmitStreamToolCall(w, flusher, nextIndex, toolCall, newID)
	}
	streamState.MarkIndex(nextIndex)
	streamState.Complete(anthropicStopReason(response.StopReason, len(response.ToolCalls) > 0), anthropicOutputTokens(response.Usage))
	return nil
}

func EmitStreamToolCall(w http.ResponseWriter, flusher http.Flusher, nextIndex int, toolCall models.ToolCall, newID func() string) int {
	name := strings.TrimSpace(toolCall.Name)
	if name == "" {
		return nextIndex
	}
	id := strings.TrimSpace(toolCall.ID)
	if id == "" {
		id = "toolu_" + newID()
	}
	toolIndex := nextIndex
	nextIndex++
	WriteSSEEvent(w, "content_block_start", map[string]any{"type": "content_block_start", "index": toolIndex, "content_block": map[string]any{"type": "tool_use", "id": id, "name": name, "input": map[string]any{}}})
	flusher.Flush()
	WriteSSEEvent(w, "content_block_delta", map[string]any{"type": "content_block_delta", "index": toolIndex, "delta": map[string]any{"type": "input_json_delta", "partial_json": anthropicToolArgumentsJSON(toolCall)}})
	flusher.Flush()
	WriteSSEEvent(w, "content_block_stop", map[string]any{"type": "content_block_stop", "index": toolIndex})
	flusher.Flush()
	return nextIndex
}
