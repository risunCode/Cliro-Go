package gateway

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	contract "cliro-go/internal/contract"
	"cliro-go/internal/contract/rules"
	"cliro-go/internal/logger"
	"cliro-go/internal/protocol/anthropic"
	"cliro-go/internal/provider"
	kiroprovider "cliro-go/internal/provider/kiro"
	"cliro-go/internal/route"
	"cliro-go/internal/util"
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
	s.logRequestEvent("info", requestID, "accepted", fmt.Sprintf("route=%q", "anthropic_messages"), fmt.Sprintf("model=%q", strings.TrimSpace(req.Model)), fmt.Sprintf("stream=%t", req.Stream))

	s.processAnthropicMessages(w, r, requestID, req)
}

func (s *Server) processAnthropicMessages(w http.ResponseWriter, r *http.Request, requestID string, req anthropic.MessagesRequest) {
	irRequest, err := anthropic.MessagesToIR(req)
	if err != nil {
		s.logRequestEvent("warn", requestID, "rejected", fmt.Sprintf("route=%q", "anthropic_messages"), fmt.Sprintf("reason=%q", err.Error()))
		s.writeAnthropicError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}

	// Check if we should use live streaming for Kiro provider
	resolution, resolveErr := s.resolveModelForStreaming(irRequest.Model)
	useLiveStreaming := req.Stream && resolveErr == nil && resolution.Provider == route.ProviderKiro

	if useLiveStreaming {
		s.processAnthropicMessagesLiveStream(w, r, requestID, req, irRequest)
		return
	}

	// Fallback to buffered streaming
	thinkingRequested := s.thinkingRequestedForModel(irRequest.Model, irRequest.Thinking.Requested)
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
		s.logAnthropicThinkingDecision(requestID, thinkingRequested, irResponse, strings.TrimSpace(anthropicThinkingSignature(irResponse.Thinking, irResponse.ThinkingSignature)) != "")
		s.streamAnthropicMessages(w, req.Model, irResponse)
		return
	}

	response := anthropic.IRToMessages(irResponse)
	response.Model = util.FirstNonEmpty(strings.TrimSpace(req.Model), strings.TrimSpace(response.Model))
	s.logAnthropicThinkingDecision(requestID, thinkingRequested, irResponse, strings.TrimSpace(anthropicThinkingSignature(irResponse.Thinking, irResponse.ThinkingSignature)) != "")
	s.logRequestEvent("info", requestID, "completed", fmt.Sprintf("route=%q", "anthropic_messages"), fmt.Sprintf("status=%q", "completed"), fmt.Sprintf("prompt_tokens=%d", irResponse.Usage.PromptTokens), fmt.Sprintf("completion_tokens=%d", irResponse.Usage.CompletionTokens), fmt.Sprintf("total_tokens=%d", irResponse.Usage.TotalTokens))
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(response)
}

func (s *Server) streamAnthropicMessages(w http.ResponseWriter, requestedModel string, response contract.Response) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		s.writeAnthropicError(w, http.StatusInternalServerError, "api_error", "streaming not supported")
		return
	}

	messageID := "msg_" + newSSEID()
	if strings.HasPrefix(strings.TrimSpace(response.ID), "msg_") {
		messageID = strings.TrimSpace(response.ID)
	}
	model := util.FirstNonEmpty(strings.TrimSpace(requestedModel), strings.TrimSpace(response.Model))

	thinkingPresent := strings.TrimSpace(response.Thinking) != ""
	textPresent := strings.TrimSpace(response.Text) != ""
	thinkingSignature := anthropicThinkingSignature(response.Thinking, response.ThinkingSignature)
	contentBlocks := make([]map[string]any, 0, 2+len(response.ToolCalls))
	if thinkingPresent {
		contentBlocks = append(contentBlocks, map[string]any{
			"type":      "thinking",
			"thinking":  "",
			"signature": "",
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
				"input_tokens":  anthropicStreamInputTokens(response.Usage),
				"output_tokens": 0,
			},
		},
	})
	flusher.Flush()

	emitStreamEvent := func(eventName string, payload map[string]any) {
		writeAnthropicSSEEvent(w, eventName, payload)
		flusher.Flush()
	}

	nextIndex := 0
	thinkingLifecycle := anthropic.NewThinkingBlockLifecycle(nextIndex, emitStreamEvent)
	if thinkingPresent {
		for _, chunk := range chunkText(response.Thinking, 160) {
			thinkingLifecycle.EmitThinkingDelta(chunk)
		}
		nextIndex = thinkingLifecycle.PrepareForNextBlock(thinkingSignature)
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
			event := anthropic.IRStreamToEvent(contract.Event{TextDelta: chunk})
			event["index"] = textIndex
			writeAnthropicSSEEvent(w, event["type"].(string), event)
			flusher.Flush()
		}

		writeAnthropicSSEEvent(w, "content_block_stop", map[string]any{"type": "content_block_stop", "index": textIndex})
		flusher.Flush()
	}

	for _, toolCall := range response.ToolCalls {
		nextIndex = emitAnthropicStreamToolCall(w, flusher, nextIndex, toolCall)
	}

	stopReason := anthropicStreamStopReason(response.StopReason, len(response.ToolCalls) > 0)

	writeAnthropicSSEEvent(w, "message_delta", map[string]any{
		"type": "message_delta",
		"delta": map[string]any{
			"stop_reason":   stopReason,
			"stop_sequence": nil,
		},
		"usage": map[string]int{
			"output_tokens": anthropicStreamOutputTokens(response.Usage),
		},
	})
	flusher.Flush()

	writeAnthropicSSEEvent(w, "message_stop", map[string]any{"type": "message_stop"})
	flusher.Flush()
}

func anthropicThinkingSignature(thinking string, providedSignature string) string {
	if strings.TrimSpace(thinking) == "" {
		return ""
	}
	if strings.TrimSpace(providedSignature) != "" {
		return strings.TrimSpace(providedSignature)
	}
	return contract.StableThinkingSignature(thinking)
}

func (s *Server) resolveModelForStreaming(model string) (route.ModelResolution, error) {
	aliases := s.store.ModelAliases()
	resolution, err := route.ResolveModel(model, route.DefaultThinkingSuffix, aliases)
	if err != nil {
		return route.ModelResolution{}, err
	}
	return resolution, nil
}

func (s *Server) thinkingRequestedForModel(model string, alreadyRequested bool) bool {
	if alreadyRequested {
		return true
	}
	resolution, err := s.resolveModelForStreaming(model)
	if err != nil {
		return false
	}
	return resolution.ThinkingRequested
}

func (s *Server) processAnthropicMessagesLiveStream(w http.ResponseWriter, r *http.Request, requestID string, req anthropic.MessagesRequest, irRequest contract.Request) {
	preparedRequest, _, status, message, err := s.prepareExecutionRequest(irRequest)
	if err != nil {
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

	kiroExecutor, ok := s.kiro.(liveCompletionExecutor)
	if !ok {
		s.logRequestEvent("warn", requestID, "failed", fmt.Sprintf("route=%q", "anthropic_messages"), fmt.Sprintf("reason=%q", "live streaming executor unavailable"))
		s.writeAnthropicError(w, http.StatusInternalServerError, "api_error", "live streaming executor unavailable")
		return
	}

	// Setup SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		s.writeAnthropicError(w, http.StatusInternalServerError, "api_error", "streaming not supported")
		return
	}

	// Prepare message metadata
	messageID := "msg_" + newSSEID()
	model := util.FirstNonEmpty(strings.TrimSpace(req.Model), strings.TrimSpace(preparedRequest.Model))

	// State tracking for live streaming
	var textStarted bool
	var textIndex int
	var promptTokens, completionTokens int
	var streamedThinking strings.Builder

	// Send message_start
	writeAnthropicSSEEvent(w, "message_start", map[string]any{
		"type": "message_start",
		"message": map[string]any{
			"id":            messageID,
			"type":          "message",
			"role":          "assistant",
			"model":         model,
			"content":       []any{},
			"stop_reason":   nil,
			"stop_sequence": nil,
			"usage": map[string]int{
				"input_tokens":  0,
				"output_tokens": 0,
			},
		},
	})
	flusher.Flush()
	emitStreamEvent := func(eventName string, payload map[string]any) {
		writeAnthropicSSEEvent(w, eventName, payload)
		flusher.Flush()
	}
	thinkingLifecycle := anthropic.NewThinkingBlockLifecycle(0, emitStreamEvent)

	// Execute with streaming callback
	chatReq := provider.RequestFromIR(preparedRequest)

	outcome, status, message, execErr := kiroExecutor.CompleteWithCallback(r.Context(), chatReq, func(event kiroprovider.StreamEvent) {
		// Handle thinking delta
		if event.Thinking != "" {
			if !textStarted {
				streamedThinking.WriteString(event.Thinking)
				thinkingLifecycle.EmitThinkingDelta(event.Thinking)
			}
		}

		// Handle text delta
		if event.Text != "" {
			if !textStarted {
				textIndex = thinkingLifecycle.PrepareForNextBlock(anthropicThinkingSignature(streamedThinking.String(), ""))
				textStarted = true

				writeAnthropicSSEEvent(w, "content_block_start", map[string]any{
					"type":  "content_block_start",
					"index": textIndex,
					"content_block": map[string]any{
						"type": "text",
						"text": "",
					},
				})
				flusher.Flush()
			}

			writeAnthropicSSEEvent(w, "content_block_delta", map[string]any{
				"type":  "content_block_delta",
				"index": textIndex,
				"delta": map[string]any{
					"type": "text_delta",
					"text": event.Text,
				},
			})
			flusher.Flush()
		}

		// Track usage
		if event.Usage.PromptTokens > 0 {
			promptTokens = event.Usage.PromptTokens
		}
		if event.Usage.CompletionTokens > 0 {
			completionTokens = event.Usage.CompletionTokens
		}
	})

	if execErr != nil {
		s.logRequestEvent("warn", requestID, "failed", fmt.Sprintf("route=%q", "anthropic_messages"), fmt.Sprintf("status=%d", status), fmt.Sprintf("reason=%q", message))
		// Can't send error after SSE started, just close stream
		return
	}
	irResponse := outcomeToIRResponse(outcome, preparedRequest.Model)

	// Close any open content blocks
	nextIndex := 0
	if textStarted {
		writeAnthropicSSEEvent(w, "content_block_stop", map[string]any{
			"type":  "content_block_stop",
			"index": textIndex,
		})
		flusher.Flush()
		nextIndex = textIndex + 1
	} else {
		nextIndex = thinkingLifecycle.PrepareForNextBlock(anthropicThinkingSignature(irResponse.Thinking, irResponse.ThinkingSignature))
	}

	for _, toolCall := range irResponse.ToolCalls {
		nextIndex = emitAnthropicStreamToolCall(w, flusher, nextIndex, toolCall)
	}

	// Send message_delta with stop reason
	stopReason := anthropicStreamStopReason(irResponse.StopReason, len(irResponse.ToolCalls) > 0)

	writeAnthropicSSEEvent(w, "message_delta", map[string]any{
		"type": "message_delta",
		"delta": map[string]any{
			"stop_reason":   stopReason,
			"stop_sequence": nil,
		},
		"usage": map[string]int{
			"output_tokens": completionTokens,
		},
	})
	flusher.Flush()

	// Send message_stop
	writeAnthropicSSEEvent(w, "message_stop", map[string]any{
		"type": "message_stop",
	})
	flusher.Flush()

	s.logAnthropicThinkingDecision(requestID, preparedRequest.Thinking.Requested, irResponse, strings.TrimSpace(anthropicThinkingSignature(util.FirstNonEmpty(streamedThinking.String(), outcome.Thinking), outcome.ThinkingSignature)) != "")
	s.logRequestEvent("info", requestID, "completed", fmt.Sprintf("route=%q", "anthropic_messages"), fmt.Sprintf("status=%q", "live_streaming"), fmt.Sprintf("prompt_tokens=%d", promptTokens), fmt.Sprintf("completion_tokens=%d", completionTokens))
}

func (s *Server) logAnthropicThinkingDecision(requestID string, thinkingRequested bool, response contract.Response, anthropicSignatureEmitted bool) {
	s.logThinkingDecision("info", requestID,
		logger.String("route", string(contract.EndpointAnthropicMessages)),
		logger.Bool("thinking_requested", thinkingRequested),
		logger.String("thinking_source", thinkingSourceValue(response.ThinkingSource, response.Thinking)),
		logger.Bool("thinking_emitted", strings.TrimSpace(response.Thinking) != ""),
		logger.Bool("anthropic_signature_emitted", anthropicSignatureEmitted),
	)
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

	irRequest, err := anthropic.MessagesToIR(req)
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

func irMessagesToText(messages []contract.Message) string {
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

func emitAnthropicStreamToolCall(w http.ResponseWriter, flusher http.Flusher, nextIndex int, toolCall contract.ToolCall) int {
	name := strings.TrimSpace(toolCall.Name)
	if name == "" {
		return nextIndex
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

	writeAnthropicSSEEvent(w, "content_block_delta", map[string]any{
		"type":  "content_block_delta",
		"index": toolIndex,
		"delta": map[string]any{
			"type":         "input_json_delta",
			"partial_json": anthropicToolArgumentsJSON(toolCall),
		},
	})
	flusher.Flush()

	writeAnthropicSSEEvent(w, "content_block_stop", map[string]any{"type": "content_block_stop", "index": toolIndex})
	flusher.Flush()
	return nextIndex
}

func anthropicToolArgumentsJSON(toolCall contract.ToolCall) string {
	input := map[string]any{}
	arguments := strings.TrimSpace(toolCall.Arguments)
	if arguments != "" {
		_ = json.Unmarshal([]byte(arguments), &input)
	}
	input = rules.RemapToolCallArgs(toolCall.Name, input)
	encoded, err := json.Marshal(input)
	if err != nil {
		return "{}"
	}
	return string(encoded)
}

func anthropicStreamStopReason(stopReason string, hasToolCalls bool) string {
	if hasToolCalls {
		return "tool_use"
	}
	switch strings.TrimSpace(stopReason) {
	case "", "stop", "end_turn":
		return "end_turn"
	case "tool_calls", "tool_use":
		return "tool_use"
	default:
		return strings.TrimSpace(stopReason)
	}
}

func anthropicStreamInputTokens(usage contract.Usage) int {
	if usage.InputTokens > 0 {
		return usage.InputTokens
	}
	return usage.PromptTokens
}

func anthropicStreamOutputTokens(usage contract.Usage) int {
	if usage.OutputTokens > 0 {
		return usage.OutputTokens
	}
	return usage.CompletionTokens
}
