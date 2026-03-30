package gateway

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"cliro-go/internal/adapter/decode"
	"cliro-go/internal/adapter/encode"
	"cliro-go/internal/adapter/ir"
	"cliro-go/internal/protocol/anthropic"
)

func (s *Server) handleAnthropicMessages(w http.ResponseWriter, r *http.Request) {
	r, requestID := s.prepareRequestContext(r)
	s.applyCommonHeaders(w)
	w.Header().Set("X-Request-ID", requestID)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if secErr := s.validateSecurityHeaders(r); secErr.Message != "" {
		s.logRequestEvent("warn", requestID, "rejected", fmt.Sprintf("route=%q", "anthropic_messages"), fmt.Sprintf("reason=%q", secErr.Message))
		s.writeAnthropicError(w, secErr.Status, secErr.Type, secErr.Message)
		return
	}
	if r.Method != http.MethodPost {
		s.writeAnthropicError(w, http.StatusMethodNotAllowed, "invalid_request_error", "method not allowed")
		return
	}

	req, err := anthropic.DecodeMessagesRequest(r.Body)
	if err != nil {
		s.logRequestEvent("warn", requestID, "rejected", fmt.Sprintf("route=%q", "anthropic_messages"), fmt.Sprintf("reason=%q", "invalid JSON"))
		s.writeAnthropicError(w, http.StatusBadRequest, "invalid_request_error", "invalid JSON")
		return
	}
	req.Stream = s.resolveStreamFlag(req.Model, string(ir.EndpointAnthropicMessages), req.Stream)
	s.logRequestEvent("info", requestID, "accepted", fmt.Sprintf("route=%q", "anthropic_messages"), fmt.Sprintf("model=%q", strings.TrimSpace(req.Model)), fmt.Sprintf("stream=%t", req.Stream))

	s.processAnthropicMessages(w, r, requestID, req)
}

func (s *Server) processAnthropicMessages(w http.ResponseWriter, r *http.Request, requestID string, req anthropic.MessagesRequest) {
	irRequest, err := decode.AnthropicMessagesToIR(req)
	if err != nil {
		s.logRequestEvent("warn", requestID, "rejected", fmt.Sprintf("route=%q", "anthropic_messages"), fmt.Sprintf("reason=%q", err.Error()))
		s.writeAnthropicError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}

	irResponse, status, message, execErr := s.executeRequest(r.Context(), irRequest)
	if execErr != nil {
		s.logRequestEvent("warn", requestID, "failed", fmt.Sprintf("route=%q", "anthropic_messages"), fmt.Sprintf("status=%d", status), fmt.Sprintf("reason=%q", message))
		errorType := "api_error"
		if status == http.StatusBadRequest {
			errorType = "invalid_request_error"
		} else if status == http.StatusServiceUnavailable {
			errorType = "provider_unavailable"
		}
		s.writeAnthropicError(w, status, errorType, message)
		return
	}

	if req.Stream {
		s.logRequestEvent("info", requestID, "completed", fmt.Sprintf("route=%q", "anthropic_messages"), fmt.Sprintf("status=%q", "streaming"), fmt.Sprintf("prompt_tokens=%d", irResponse.Usage.PromptTokens), fmt.Sprintf("completion_tokens=%d", irResponse.Usage.CompletionTokens), fmt.Sprintf("total_tokens=%d", irResponse.Usage.TotalTokens))
		s.streamAnthropicMessages(w, req.Model, irResponse)
		return
	}

	response := encode.IRToAnthropicMessages(irResponse)
	response.Model = firstNonEmpty(strings.TrimSpace(req.Model), strings.TrimSpace(response.Model))
	s.logRequestEvent("info", requestID, "completed", fmt.Sprintf("route=%q", "anthropic_messages"), fmt.Sprintf("status=%q", "completed"), fmt.Sprintf("prompt_tokens=%d", irResponse.Usage.PromptTokens), fmt.Sprintf("completion_tokens=%d", irResponse.Usage.CompletionTokens), fmt.Sprintf("total_tokens=%d", irResponse.Usage.TotalTokens))
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(response)
}

func (s *Server) streamAnthropicMessages(w http.ResponseWriter, requestedModel string, response ir.Response) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		s.writeAnthropicError(w, http.StatusInternalServerError, "api_error", "streaming not supported")
		return
	}

	messageID := "msg_" + newSSEID()
	model := firstNonEmpty(strings.TrimSpace(requestedModel), strings.TrimSpace(response.Model))

	thinkingPresent := strings.TrimSpace(response.Thinking) != ""
	textPresent := strings.TrimSpace(response.Text) != ""
	thinkingSignature := ""
	if thinkingPresent {
		thinkingSignature = generateThinkingSignature()
	}
	contentBlocks := make([]map[string]any, 0, 2+len(response.ToolCalls))
	if thinkingPresent {
		contentBlocks = append(contentBlocks, map[string]any{
			"type":      "thinking",
			"thinking":  "",
			"signature": thinkingSignature,
		})
	}
	if textPresent {
		contentBlocks = append(contentBlocks, map[string]any{
			"type": "text",
			"text": "",
		})
	}
	for _, toolCall := range response.ToolCalls {
		name := strings.TrimSpace(toolCall.Name)
		if name == "" {
			continue
		}
		id := strings.TrimSpace(toolCall.ID)
		if id == "" {
			id = "toolu_" + newSSEID()
		}
		contentBlocks = append(contentBlocks, map[string]any{
			"type":  "tool_use",
			"id":    id,
			"name":  name,
			"input": map[string]any{},
		})
	}
	if len(contentBlocks) == 0 {
		contentBlocks = append(contentBlocks, map[string]any{
			"type": "text",
			"text": "",
		})
		textPresent = true
	}

	writeAnthropicSSEEvent(w, "message_start", map[string]any{
		"type": "message_start",
		"message": map[string]any{
			"id":            messageID,
			"type":          "message",
			"role":          "assistant",
			"model":         model,
			"content":       contentBlocks,
			"stop_reason":   nil,
			"stop_sequence": nil,
			"usage": map[string]int{
				"input_tokens":  response.Usage.PromptTokens,
				"output_tokens": 0,
			},
		},
	})
	flusher.Flush()

	nextIndex := 0
	if thinkingPresent {
		thinkingIndex := nextIndex
		nextIndex++

		writeAnthropicSSEEvent(w, "content_block_start", map[string]any{
			"type":  "content_block_start",
			"index": thinkingIndex,
			"content_block": map[string]any{
				"type":      "thinking",
				"thinking":  "",
				"signature": thinkingSignature,
			},
		})
		flusher.Flush()

		for _, chunk := range chunkText(response.Thinking, 160) {
			event := encode.IRStreamToAnthropicEvent(ir.Event{ThinkDelta: chunk})
			event["index"] = thinkingIndex
			writeAnthropicSSEEvent(w, event["type"].(string), event)
			flusher.Flush()
		}

		writeAnthropicSSEEvent(w, "content_block_stop", map[string]any{"type": "content_block_stop", "index": thinkingIndex})
		flusher.Flush()
	}

	if textPresent {
		textIndex := nextIndex
		nextIndex++

		writeAnthropicSSEEvent(w, "content_block_start", map[string]any{
			"type":  "content_block_start",
			"index": textIndex,
			"content_block": map[string]any{
				"type": "text",
				"text": "",
			},
		})
		flusher.Flush()

		for _, chunk := range chunkText(response.Text, 160) {
			event := encode.IRStreamToAnthropicEvent(ir.Event{TextDelta: chunk})
			event["index"] = textIndex
			writeAnthropicSSEEvent(w, event["type"].(string), event)
			flusher.Flush()
		}

		writeAnthropicSSEEvent(w, "content_block_stop", map[string]any{"type": "content_block_stop", "index": textIndex})
		flusher.Flush()
	}

	for _, toolCall := range response.ToolCalls {
		name := strings.TrimSpace(toolCall.Name)
		if name == "" {
			continue
		}
		id := strings.TrimSpace(toolCall.ID)
		if id == "" {
			id = "toolu_" + newSSEID()
		}

		toolIndex := nextIndex
		nextIndex++

		writeAnthropicSSEEvent(w, "content_block_start", map[string]any{
			"type":  "content_block_start",
			"index": toolIndex,
			"content_block": map[string]any{
				"type":  "tool_use",
				"id":    id,
				"name":  name,
				"input": map[string]any{},
			},
		})
		flusher.Flush()

		arguments := strings.TrimSpace(toolCall.Arguments)
		if arguments == "" {
			arguments = "{}"
		}
		writeAnthropicSSEEvent(w, "content_block_delta", map[string]any{
			"type":  "content_block_delta",
			"index": toolIndex,
			"delta": map[string]any{
				"type":         "input_json_delta",
				"partial_json": arguments,
			},
		})
		flusher.Flush()

		writeAnthropicSSEEvent(w, "content_block_stop", map[string]any{"type": "content_block_stop", "index": toolIndex})
		flusher.Flush()
	}

	stopReason := "end_turn"
	if len(response.ToolCalls) > 0 {
		stopReason = "tool_use"
	}

	writeAnthropicSSEEvent(w, "message_delta", map[string]any{
		"type": "message_delta",
		"delta": map[string]any{
			"stop_reason":   stopReason,
			"stop_sequence": nil,
		},
		"usage": map[string]int{
			"output_tokens": response.Usage.CompletionTokens,
		},
	})
	flusher.Flush()

	writeAnthropicSSEEvent(w, "message_stop", map[string]any{"type": "message_stop"})
	flusher.Flush()
}

func (s *Server) handleAnthropicCountTokens(w http.ResponseWriter, r *http.Request) {
	r, requestID := s.prepareRequestContext(r)
	s.applyCommonHeaders(w)
	w.Header().Set("X-Request-ID", requestID)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if secErr := s.validateSecurityHeaders(r); secErr.Message != "" {
		s.logRequestEvent("warn", requestID, "rejected", fmt.Sprintf("route=%q", "anthropic_count_tokens"), fmt.Sprintf("reason=%q", secErr.Message))
		s.writeAnthropicError(w, secErr.Status, secErr.Type, secErr.Message)
		return
	}
	if r.Method != http.MethodPost {
		s.writeAnthropicError(w, http.StatusMethodNotAllowed, "invalid_request_error", "method not allowed")
		return
	}

	req, err := anthropic.DecodeCountTokensRequest(r.Body)
	if err != nil {
		s.logRequestEvent("warn", requestID, "rejected", fmt.Sprintf("route=%q", "anthropic_count_tokens"), fmt.Sprintf("reason=%q", "invalid JSON"))
		s.writeAnthropicError(w, http.StatusBadRequest, "invalid_request_error", "invalid JSON")
		return
	}

	irRequest, err := decode.AnthropicMessagesToIR(req)
	if err != nil {
		s.logRequestEvent("warn", requestID, "rejected", fmt.Sprintf("route=%q", "anthropic_count_tokens"), fmt.Sprintf("reason=%q", err.Error()))
		s.writeAnthropicError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}

	inputText := irMessagesToText(irRequest.Messages)
	estimated := estimateTokens(inputText)

	s.logRequestEvent("info", requestID, "completed", fmt.Sprintf("route=%q", "anthropic_count_tokens"), fmt.Sprintf("status=%q", "completed"), fmt.Sprintf("input_tokens=%d", estimated))
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"input_tokens": estimated})
}

func irMessagesToText(messages []ir.Message) string {
	parts := make([]string, 0, len(messages))
	for _, message := range messages {
		content := strings.TrimSpace(anyToText(message.Content))
		if content != "" {
			parts = append(parts, content)
		}
	}
	return strings.TrimSpace(strings.Join(parts, "\n"))
}

func anyToText(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(typed)
	case []any:
		parts := make([]string, 0, len(typed))
		for _, item := range typed {
			text := strings.TrimSpace(anyToText(item))
			if text != "" {
				parts = append(parts, text)
			}
		}
		return strings.TrimSpace(strings.Join(parts, "\n"))
	case map[string]any:
		if text, ok := typed["text"].(string); ok && strings.TrimSpace(text) != "" {
			return strings.TrimSpace(text)
		}
		if thinking, ok := typed["thinking"].(string); ok && strings.TrimSpace(thinking) != "" {
			return strings.TrimSpace(thinking)
		}
		if content, ok := typed["content"]; ok {
			return anyToText(content)
		}
		encoded, _ := json.Marshal(typed)
		return strings.TrimSpace(string(encoded))
	default:
		encoded, _ := json.Marshal(typed)
		return strings.TrimSpace(string(encoded))
	}
}

func writeAnthropicSSEEvent(w http.ResponseWriter, event string, payload any) {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return
	}
	_, _ = fmt.Fprintf(w, "event: %s\n", strings.TrimSpace(event))
	_, _ = fmt.Fprintf(w, "data: %s\n\n", encoded)
}

func generateThinkingSignature() string {
	raw := strings.ReplaceAll(newSSEID(), "-", "")
	if len(raw) > 32 {
		raw = raw[:32]
	}
	return "sig_" + raw
}
