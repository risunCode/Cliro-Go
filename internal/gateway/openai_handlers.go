package gateway

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"cliro-go/internal/adapter/decode"
	"cliro-go/internal/adapter/encode"
	"cliro-go/internal/adapter/ir"
	"cliro-go/internal/protocol/openai"
)

func (s *Server) handleChatCompletions(w http.ResponseWriter, r *http.Request) {
	r, requestID := s.prepareRequestContext(r)
	s.applyCommonHeaders(w)
	w.Header().Set("X-Request-ID", requestID)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if secErr := s.validateSecurityHeaders(r); secErr.Message != "" {
		s.logRequestEvent("warn", requestID, "rejected", fmt.Sprintf("route=%q", "openai_chat"), fmt.Sprintf("reason=%q", secErr.Message))
		s.writeOpenAIError(w, secErr.Status, secErr.Type, secErr.Message)
		return
	}
	if r.Method != http.MethodPost {
		s.writeOpenAIError(w, http.StatusMethodNotAllowed, "invalid_request_error", "method not allowed")
		return
	}

	req, err := openai.DecodeChatRequest(r.Body)
	if err != nil {
		s.logRequestEvent("warn", requestID, "rejected", fmt.Sprintf("route=%q", "openai_chat"), fmt.Sprintf("reason=%q", "invalid JSON"))
		s.writeOpenAIError(w, http.StatusBadRequest, "invalid_request_error", "invalid JSON")
		return
	}
	req.Stream = s.resolveStreamFlag(req.Model, string(ir.EndpointOpenAIChat), req.Stream)
	s.logRequestEvent("info", requestID, "accepted", fmt.Sprintf("route=%q", "openai_chat"), fmt.Sprintf("model=%q", strings.TrimSpace(req.Model)), fmt.Sprintf("stream=%t", req.Stream))

	s.processOpenAIChat(w, r, requestID, req)
}

func (s *Server) processOpenAIChat(w http.ResponseWriter, r *http.Request, requestID string, req openai.ChatRequest) {
	irRequest, err := decode.OpenAIChatToIR(req)
	if err != nil {
		s.logRequestEvent("warn", requestID, "rejected", fmt.Sprintf("route=%q", "openai_chat"), fmt.Sprintf("reason=%q", err.Error()))
		s.writeOpenAIError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}

	irResponse, status, message, execErr := s.executeRequest(r.Context(), irRequest)
	if execErr != nil {
		s.logRequestEvent("warn", requestID, "failed", fmt.Sprintf("route=%q", "openai_chat"), fmt.Sprintf("status=%d", status), fmt.Sprintf("reason=%q", message))
		errorType := "server_error"
		if status == http.StatusBadRequest {
			errorType = "invalid_request_error"
		} else if status == http.StatusServiceUnavailable {
			errorType = "provider_unavailable"
		}
		s.writeOpenAIError(w, status, errorType, message)
		return
	}

	if req.Stream {
		s.logRequestEvent("info", requestID, "completed", fmt.Sprintf("route=%q", "openai_chat"), fmt.Sprintf("status=%q", "streaming"), fmt.Sprintf("prompt_tokens=%d", irResponse.Usage.PromptTokens), fmt.Sprintf("completion_tokens=%d", irResponse.Usage.CompletionTokens), fmt.Sprintf("total_tokens=%d", irResponse.Usage.TotalTokens))
		s.streamOpenAIChat(w, req.Model, irResponse)
		return
	}

	response := encode.IRToOpenAIChat(irResponse)
	response.Model = firstNonEmpty(strings.TrimSpace(req.Model), strings.TrimSpace(response.Model))
	s.logRequestEvent("info", requestID, "completed", fmt.Sprintf("route=%q", "openai_chat"), fmt.Sprintf("status=%q", "completed"), fmt.Sprintf("prompt_tokens=%d", irResponse.Usage.PromptTokens), fmt.Sprintf("completion_tokens=%d", irResponse.Usage.CompletionTokens), fmt.Sprintf("total_tokens=%d", irResponse.Usage.TotalTokens))
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(response)
}

func (s *Server) streamOpenAIChat(w http.ResponseWriter, requestedModel string, response ir.Response) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		s.writeOpenAIError(w, http.StatusInternalServerError, "server_error", "streaming not supported")
		return
	}

	chatID := firstNonEmpty(strings.TrimSpace(response.ID), "chatcmpl-"+newSSEID())
	model := firstNonEmpty(strings.TrimSpace(requestedModel), strings.TrimSpace(response.Model))

	initialChunk := openai.ChatStreamChunk{
		ID:      chatID,
		Object:  "chat.completion.chunk",
		Created: nowUnix(),
		Model:   model,
		Choices: []openai.ChatStreamChoice{{
			Index:        0,
			Delta:        map[string]any{"role": "assistant"},
			FinishReason: nil,
		}},
	}
	if err := writeOpenAISSEChunk(w, initialChunk); err != nil {
		return
	}
	flusher.Flush()

	for _, chunk := range chunkText(response.Thinking, 160) {
		event := ir.Event{ThinkDelta: chunk}
		if err := writeOpenAISSEChunk(w, encode.IRStreamToOpenAIChunk(chatID, model, event)); err != nil {
			return
		}
		flusher.Flush()
	}

	for _, chunk := range chunkText(response.Text, 160) {
		event := ir.Event{TextDelta: chunk}
		if err := writeOpenAISSEChunk(w, encode.IRStreamToOpenAIChunk(chatID, model, event)); err != nil {
			return
		}
		flusher.Flush()
	}

	for index, toolCall := range encodeIRToolCallsForStream(response.ToolCalls) {
		event := ir.Event{ToolDelta: []map[string]any{{
			"index":    index,
			"id":       toolCall.ID,
			"type":     "function",
			"function": toolCall.Function,
		}}}
		if err := writeOpenAISSEChunk(w, encode.IRStreamToOpenAIChunk(chatID, model, event)); err != nil {
			return
		}
		flusher.Flush()
	}

	finishReason := firstNonEmpty(strings.TrimSpace(response.StopReason), "stop")
	if err := writeOpenAISSEChunk(w, encode.IRStreamToOpenAIChunk(chatID, model, ir.Event{Done: true, Type: finishReason})); err != nil {
		return
	}
	_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
	flusher.Flush()
}

func (s *Server) handleCompletions(w http.ResponseWriter, r *http.Request) {
	r, requestID := s.prepareRequestContext(r)
	s.applyCommonHeaders(w)
	w.Header().Set("X-Request-ID", requestID)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if secErr := s.validateSecurityHeaders(r); secErr.Message != "" {
		s.logRequestEvent("warn", requestID, "rejected", fmt.Sprintf("route=%q", "openai_completions"), fmt.Sprintf("reason=%q", secErr.Message))
		s.writeOpenAIError(w, secErr.Status, secErr.Type, secErr.Message)
		return
	}
	if r.Method != http.MethodPost {
		s.writeOpenAIError(w, http.StatusMethodNotAllowed, "invalid_request_error", "method not allowed")
		return
	}

	req, err := openai.DecodeCompletionsRequest(r.Body)
	if err != nil {
		s.logRequestEvent("warn", requestID, "rejected", fmt.Sprintf("route=%q", "openai_completions"), fmt.Sprintf("reason=%q", "invalid JSON"))
		s.writeOpenAIError(w, http.StatusBadRequest, "invalid_request_error", "invalid JSON")
		return
	}
	req.Stream = s.resolveStreamFlag(req.Model, string(ir.EndpointOpenAICompletions), req.Stream)
	s.logRequestEvent("info", requestID, "accepted", fmt.Sprintf("route=%q", "openai_completions"), fmt.Sprintf("model=%q", strings.TrimSpace(req.Model)), fmt.Sprintf("stream=%t", req.Stream))

	s.processOpenAICompletions(w, r, requestID, req)
}

func (s *Server) processOpenAICompletions(w http.ResponseWriter, r *http.Request, requestID string, req openai.CompletionsRequest) {
	irRequest, err := decode.OpenAICompletionsToIR(req)
	if err != nil {
		s.logRequestEvent("warn", requestID, "rejected", fmt.Sprintf("route=%q", "openai_completions"), fmt.Sprintf("reason=%q", err.Error()))
		s.writeOpenAIError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}

	irResponse, status, message, execErr := s.executeRequest(r.Context(), irRequest)
	if execErr != nil {
		s.logRequestEvent("warn", requestID, "failed", fmt.Sprintf("route=%q", "openai_completions"), fmt.Sprintf("status=%d", status), fmt.Sprintf("reason=%q", message))
		errorType := "server_error"
		if status == http.StatusBadRequest {
			errorType = "invalid_request_error"
		} else if status == http.StatusServiceUnavailable {
			errorType = "provider_unavailable"
		}
		s.writeOpenAIError(w, status, errorType, message)
		return
	}

	if req.Stream {
		s.logRequestEvent("info", requestID, "completed", fmt.Sprintf("route=%q", "openai_completions"), fmt.Sprintf("status=%q", "streaming"), fmt.Sprintf("prompt_tokens=%d", irResponse.Usage.PromptTokens), fmt.Sprintf("completion_tokens=%d", irResponse.Usage.CompletionTokens), fmt.Sprintf("total_tokens=%d", irResponse.Usage.TotalTokens))
		s.streamOpenAICompletions(w, req.Model, irResponse)
		return
	}

	response := encode.IRToOpenAICompletions(irResponse)
	response.Model = firstNonEmpty(strings.TrimSpace(req.Model), strings.TrimSpace(response.Model))
	s.logRequestEvent("info", requestID, "completed", fmt.Sprintf("route=%q", "openai_completions"), fmt.Sprintf("status=%q", "completed"), fmt.Sprintf("prompt_tokens=%d", irResponse.Usage.PromptTokens), fmt.Sprintf("completion_tokens=%d", irResponse.Usage.CompletionTokens), fmt.Sprintf("total_tokens=%d", irResponse.Usage.TotalTokens))
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(response)
}

func (s *Server) handleResponses(w http.ResponseWriter, r *http.Request) {
	r, requestID := s.prepareRequestContext(r)
	s.applyCommonHeaders(w)
	w.Header().Set("X-Request-ID", requestID)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if secErr := s.validateSecurityHeaders(r); secErr.Message != "" {
		s.logRequestEvent("warn", requestID, "rejected", fmt.Sprintf("route=%q", "openai_responses"), fmt.Sprintf("reason=%q", secErr.Message))
		s.writeOpenAIError(w, secErr.Status, secErr.Type, secErr.Message)
		return
	}
	if r.Method != http.MethodPost {
		s.writeOpenAIError(w, http.StatusMethodNotAllowed, "invalid_request_error", "method not allowed")
		return
	}

	req, err := openai.DecodeResponsesRequest(r.Body)
	if err != nil {
		s.logRequestEvent("warn", requestID, "rejected", fmt.Sprintf("route=%q", "openai_responses"), fmt.Sprintf("reason=%q", "invalid JSON"))
		s.writeOpenAIError(w, http.StatusBadRequest, "invalid_request_error", "invalid JSON")
		return
	}
	req.Stream = s.resolveStreamFlag(req.Model, string(ir.EndpointOpenAIResponses), req.Stream)
	model := strings.TrimSpace(req.Model)
	stream := req.Stream
	s.logRequestEvent("info", requestID, "accepted", fmt.Sprintf("route=%q", "openai_responses"), fmt.Sprintf("model=%q", model), fmt.Sprintf("stream=%t", stream))
	s.processOpenAIResponses(w, r, requestID, req)
}

func (s *Server) processOpenAIResponses(w http.ResponseWriter, r *http.Request, requestID string, req openai.ResponsesRequest) {
	irRequest, err := decode.OpenAIResponsesToIR(req)
	if err != nil {
		s.logRequestEvent("warn", requestID, "rejected", fmt.Sprintf("route=%q", "openai_responses"), fmt.Sprintf("reason=%q", err.Error()))
		s.writeOpenAIError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}

	irResponse, status, message, execErr := s.executeRequest(r.Context(), irRequest)
	if execErr != nil {
		s.logRequestEvent("warn", requestID, "failed", fmt.Sprintf("route=%q", "openai_responses"), fmt.Sprintf("status=%d", status), fmt.Sprintf("reason=%q", message))
		errorType := "server_error"
		if status == http.StatusBadRequest {
			errorType = "invalid_request_error"
		} else if status == http.StatusServiceUnavailable {
			errorType = "provider_unavailable"
		}
		s.writeOpenAIError(w, status, errorType, message)
		return
	}

	if req.Stream {
		s.logRequestEvent("info", requestID, "completed", fmt.Sprintf("route=%q", "openai_responses"), fmt.Sprintf("status=%q", "streaming"), fmt.Sprintf("prompt_tokens=%d", irResponse.Usage.PromptTokens), fmt.Sprintf("completion_tokens=%d", irResponse.Usage.CompletionTokens), fmt.Sprintf("total_tokens=%d", irResponse.Usage.TotalTokens))
		s.streamOpenAIResponses(w, req.Model, irResponse)
		return
	}

	response := encode.IRToOpenAIResponses(irResponse)
	response.Model = firstNonEmpty(strings.TrimSpace(req.Model), strings.TrimSpace(response.Model))
	s.logRequestEvent("info", requestID, "completed", fmt.Sprintf("route=%q", "openai_responses"), fmt.Sprintf("status=%q", "completed"), fmt.Sprintf("prompt_tokens=%d", irResponse.Usage.PromptTokens), fmt.Sprintf("completion_tokens=%d", irResponse.Usage.CompletionTokens), fmt.Sprintf("total_tokens=%d", irResponse.Usage.TotalTokens))
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(response)
}

func (s *Server) streamOpenAIResponses(w http.ResponseWriter, requestedModel string, response ir.Response) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		s.writeOpenAIError(w, http.StatusInternalServerError, "server_error", "streaming not supported")
		return
	}

	responseID := firstNonEmpty(strings.TrimSpace(response.ID), "resp_"+newSSEID())
	model := firstNonEmpty(strings.TrimSpace(requestedModel), strings.TrimSpace(response.Model))
	createdAt := nowUnix()
	itemID := "msg_" + newSSEID()

	writeOpenAIResponsesSSEEvent(w, "response.created", map[string]any{
		"type": "response.created",
		"response": map[string]any{
			"id":         responseID,
			"object":     "response",
			"created_at": createdAt,
			"status":     "in_progress",
			"model":      model,
			"output":     []any{},
		},
	})
	writeOpenAIResponsesSSEEvent(w, "response.in_progress", map[string]any{
		"type": "response.in_progress",
		"response": map[string]any{
			"id":         responseID,
			"object":     "response",
			"created_at": createdAt,
			"status":     "in_progress",
			"model":      model,
			"output":     []any{},
		},
	})
	writeOpenAIResponsesSSEEvent(w, "response.output_item.added", map[string]any{
		"type":         "response.output_item.added",
		"output_index": 0,
		"item": map[string]any{
			"id":     itemID,
			"type":   "message",
			"role":   "assistant",
			"status": "in_progress",
			"content": []any{map[string]any{
				"type":        "output_text",
				"text":        "",
				"annotations": []any{},
			}},
		},
	})
	flusher.Flush()

	for _, chunk := range chunkText(response.Text, 160) {
		writeOpenAIResponsesSSEEvent(w, "response.output_text.delta", map[string]any{
			"type":          "response.output_text.delta",
			"output_index":  0,
			"content_index": 0,
			"item_id":       itemID,
			"delta":         chunk,
		})
		flusher.Flush()
	}

	writeOpenAIResponsesSSEEvent(w, "response.output_text.done", map[string]any{
		"type":          "response.output_text.done",
		"output_index":  0,
		"content_index": 0,
		"item_id":       itemID,
		"text":          response.Text,
	})
	writeOpenAIResponsesSSEEvent(w, "response.output_item.done", map[string]any{
		"type":         "response.output_item.done",
		"output_index": 0,
		"item": map[string]any{
			"id":     itemID,
			"type":   "message",
			"role":   "assistant",
			"status": "completed",
			"content": []any{map[string]any{
				"type":        "output_text",
				"text":        response.Text,
				"annotations": []any{},
			}},
		},
	})
	writeOpenAIResponsesSSEEvent(w, "response.completed", map[string]any{
		"type":     "response.completed",
		"response": encode.IRToOpenAIResponses(response),
	})
	flusher.Flush()
}

func (s *Server) streamOpenAICompletions(w http.ResponseWriter, requestedModel string, response ir.Response) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		s.writeOpenAIError(w, http.StatusInternalServerError, "server_error", "streaming not supported")
		return
	}

	completionID := firstNonEmpty(strings.TrimSpace(response.ID), "cmpl-"+newSSEID())
	model := firstNonEmpty(strings.TrimSpace(requestedModel), strings.TrimSpace(response.Model))

	for _, chunk := range chunkText(response.Text, 160) {
		payload := openai.CompletionsStreamChunk{
			ID:      completionID,
			Object:  "text_completion",
			Created: time.Now().Unix(),
			Model:   model,
			Choices: []openai.CompletionsStreamChoice{{
				Index:        0,
				Text:         chunk,
				FinishReason: nil,
			}},
		}
		encoded, err := json.Marshal(payload)
		if err != nil {
			return
		}
		_, _ = fmt.Fprintf(w, "data: %s\n\n", encoded)
		flusher.Flush()
	}

	finishReason := firstNonEmpty(strings.TrimSpace(response.StopReason), "stop")
	finalChunk := openai.CompletionsStreamChunk{
		ID:      completionID,
		Object:  "text_completion",
		Created: time.Now().Unix(),
		Model:   model,
		Choices: []openai.CompletionsStreamChoice{{
			Index:        0,
			Text:         "",
			FinishReason: finishReason,
		}},
	}
	encoded, err := json.Marshal(finalChunk)
	if err != nil {
		return
	}
	_, _ = fmt.Fprintf(w, "data: %s\n\n", encoded)
	_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
	flusher.Flush()
}

type streamToolCall struct {
	ID       string
	Function map[string]any
}

func encodeIRToolCallsForStream(calls []ir.ToolCall) []streamToolCall {
	out := make([]streamToolCall, 0, len(calls))
	for _, call := range calls {
		name := strings.TrimSpace(call.Name)
		if name == "" {
			continue
		}
		id := strings.TrimSpace(call.ID)
		if id == "" {
			id = "toolu_" + newSSEID()
		}
		arguments := strings.TrimSpace(call.Arguments)
		if arguments == "" {
			arguments = "{}"
		}
		out = append(out, streamToolCall{
			ID: id,
			Function: map[string]any{
				"name":      name,
				"arguments": arguments,
			},
		})
	}
	return out
}

func writeOpenAISSEChunk(w http.ResponseWriter, chunk openai.ChatStreamChunk) error {
	encoded, err := json.Marshal(chunk)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "data: %s\n\n", encoded)
	return err
}

func writeOpenAIResponsesSSEEvent(w http.ResponseWriter, event string, payload any) {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return
	}
	_, _ = fmt.Fprintf(w, "event: %s\n", strings.TrimSpace(event))
	_, _ = fmt.Fprintf(w, "data: %s\n\n", encoded)
}
