package proxyhttp

import (
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"cliro/internal/logger"
	"cliro/internal/proxy/anthropic"
	proxycodex "cliro/internal/proxy/codex"
	models "cliro/internal/proxy/models"
	sharedproxy "cliro/internal/proxy/shared"
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

	req, err := proxycodex.DecodeChatRequest(r.Body)
	if err != nil {
		s.logRequestEvent("warn", requestID, "rejected", fmt.Sprintf("route=%q", "openai_chat"), fmt.Sprintf("reason=%q", "invalid JSON"))
		s.writeOpenAIError(w, http.StatusBadRequest, "invalid_request_error", "invalid JSON")
		return
	}
	s.logRequestEvent("info", requestID, "accepted", fmt.Sprintf("route=%q", "openai_chat"), fmt.Sprintf("model=%q", strings.TrimSpace(req.Model)), fmt.Sprintf("stream=%t", req.Stream))

	s.processOpenAIChat(w, r, requestID, req)
}

func (s *Server) processOpenAIChat(w http.ResponseWriter, r *http.Request, requestID string, req proxycodex.ChatRequest) {
	irRequest, err := proxycodex.ChatToIR(req)
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
		if err := proxycodex.StreamOpenAIChat(w, req.Model, irResponse, newSSEID, nowUnix); err != nil {
			s.writeOpenAIError(w, http.StatusInternalServerError, "server_error", err.Error())
		}
		return
	}

	response := proxycodex.IRToChat(irResponse)
	response.Model = sharedproxy.FirstNonEmpty(strings.TrimSpace(req.Model), strings.TrimSpace(response.Model))
	s.logRequestEvent("info", requestID, "completed", fmt.Sprintf("route=%q", "openai_chat"), fmt.Sprintf("status=%q", "completed"), fmt.Sprintf("prompt_tokens=%d", irResponse.Usage.PromptTokens), fmt.Sprintf("completion_tokens=%d", irResponse.Usage.CompletionTokens), fmt.Sprintf("total_tokens=%d", irResponse.Usage.TotalTokens))
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(response)
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

	req, err := proxycodex.DecodeCompletionsRequest(r.Body)
	if err != nil {
		s.logRequestEvent("warn", requestID, "rejected", fmt.Sprintf("route=%q", "openai_completions"), fmt.Sprintf("reason=%q", "invalid JSON"))
		s.writeOpenAIError(w, http.StatusBadRequest, "invalid_request_error", "invalid JSON")
		return
	}
	s.logRequestEvent("info", requestID, "accepted", fmt.Sprintf("route=%q", "openai_completions"), fmt.Sprintf("model=%q", strings.TrimSpace(req.Model)), fmt.Sprintf("stream=%t", req.Stream))

	s.processOpenAICompletions(w, r, requestID, req)
}

func (s *Server) processOpenAICompletions(w http.ResponseWriter, r *http.Request, requestID string, req proxycodex.CompletionsRequest) {
	irRequest, err := proxycodex.CompletionsToIR(req)
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
		if err := proxycodex.StreamOpenAICompletions(w, req.Model, irResponse, newSSEID); err != nil {
			s.writeOpenAIError(w, http.StatusInternalServerError, "server_error", err.Error())
		}
		return
	}

	response := proxycodex.IRToCompletions(irResponse)
	response.Model = sharedproxy.FirstNonEmpty(strings.TrimSpace(req.Model), strings.TrimSpace(response.Model))
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

	req, err := proxycodex.DecodeResponsesRequest(r.Body)
	if err != nil {
		s.logRequestEvent("warn", requestID, "rejected", fmt.Sprintf("route=%q", "openai_responses"), fmt.Sprintf("reason=%q", "invalid JSON"))
		s.writeOpenAIError(w, http.StatusBadRequest, "invalid_request_error", "invalid JSON")
		return
	}
	model := strings.TrimSpace(req.Model)
	stream := req.Stream
	s.logRequestEvent("info", requestID, "accepted", fmt.Sprintf("route=%q", "openai_responses"), fmt.Sprintf("model=%q", model), fmt.Sprintf("stream=%t", stream))
	s.processOpenAIResponses(w, r, requestID, req)
}

func (s *Server) processOpenAIResponses(w http.ResponseWriter, r *http.Request, requestID string, req proxycodex.ResponsesRequest) {
	irRequest, err := proxycodex.ResponsesToIR(req)
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
		if err := proxycodex.StreamOpenAIResponses(w, req.Model, irResponse, newSSEID, nowUnix); err != nil {
			s.writeOpenAIError(w, http.StatusInternalServerError, "server_error", err.Error())
		}
		return
	}

	response := proxycodex.IRToResponses(irResponse)
	response.Model = sharedproxy.FirstNonEmpty(strings.TrimSpace(req.Model), strings.TrimSpace(response.Model))
	s.logRequestEvent("info", requestID, "completed", fmt.Sprintf("route=%q", "openai_responses"), fmt.Sprintf("status=%q", "completed"), fmt.Sprintf("prompt_tokens=%d", irResponse.Usage.PromptTokens), fmt.Sprintf("completion_tokens=%d", irResponse.Usage.CompletionTokens), fmt.Sprintf("total_tokens=%d", irResponse.Usage.TotalTokens))
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(response)
}

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

	thinkingRequested := s.thinkingRequestedForModel(irRequest.Thinking.Requested)
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
		if err := anthropic.StreamAnthropicMessages(w, req.Model, irResponse, newSSEID); err != nil {
			s.writeAnthropicError(w, http.StatusInternalServerError, "api_error", err.Error())
		}
		return
	}

	response := anthropic.IRToMessages(irResponse)
	response.Model = sharedproxy.FirstNonEmpty(strings.TrimSpace(req.Model), strings.TrimSpace(response.Model))
	s.logAnthropicThinkingDecision(requestID, thinkingRequested, irResponse, strings.TrimSpace(anthropicThinkingSignature(irResponse.Thinking, irResponse.ThinkingSignature)) != "")
	s.logRequestEvent("info", requestID, "completed", fmt.Sprintf("route=%q", "anthropic_messages"), fmt.Sprintf("status=%q", "completed"), fmt.Sprintf("prompt_tokens=%d", irResponse.Usage.PromptTokens), fmt.Sprintf("completion_tokens=%d", irResponse.Usage.CompletionTokens), fmt.Sprintf("total_tokens=%d", irResponse.Usage.TotalTokens))
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(response)
}

func anthropicThinkingSignature(thinking string, providedSignature string) string {
	if strings.TrimSpace(thinking) == "" {
		return ""
	}
	if strings.TrimSpace(providedSignature) != "" {
		return strings.TrimSpace(providedSignature)
	}
	return models.StableThinkingSignature(thinking)
}

func (s *Server) thinkingRequestedForModel(alreadyRequested bool) bool {
	return alreadyRequested
}

func (s *Server) logAnthropicThinkingDecision(requestID string, thinkingRequested bool, response models.Response, anthropicSignatureEmitted bool) {
	s.logThinkingDecision("info", requestID,
		logger.F("route", string(models.EndpointAnthropicMessages)),
		logger.F("thinking_requested", thinkingRequested),
		logger.F("thinking_source", thinkingSourceValue(response.ThinkingSource, response.Thinking)),
		logger.F("thinking_emitted", strings.TrimSpace(response.Thinking) != ""),
		logger.F("anthropic_signature_emitted", anthropicSignatureEmitted),
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

func irMessagesToText(messages []models.Message) string {
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

func anthropicToolArgumentsJSON(toolCall models.ToolCall) string {
	input := map[string]any{}
	arguments := strings.TrimSpace(toolCall.Arguments)
	if arguments != "" {
		_ = json.Unmarshal([]byte(arguments), &input)
	}
	input = models.RemapToolCallArgs(toolCall.Name, input)
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

func anthropicStreamInputTokens(usage models.Usage) int {
	if usage.InputTokens > 0 {
		return usage.InputTokens
	}
	return usage.PromptTokens
}

func anthropicStreamOutputTokens(usage models.Usage) int {
	if usage.OutputTokens > 0 {
		return usage.OutputTokens
	}
	return usage.CompletionTokens
}

func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	r, requestID := s.prepareRequestContext(r)
	s.applyCommonHeaders(w)
	w.Header().Set("X-Request-ID", requestID)
	if !isRootPathAlias(r.URL.Path) {
		http.NotFound(w, r)
		return
	}
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if secErr := s.validateSecurityHeaders(r); secErr.Message != "" {
		s.logRequestEvent("warn", requestID, "rejected", fmt.Sprintf("route=%q", "root"), fmt.Sprintf("reason=%q", secErr.Message))
		s.writeGenericError(w, secErr.Status, secErr.Type, secErr.Message)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"name":    "CLIRO Gateway",
		"status":  "ok",
		"running": s.Running(),
		"routes": []string{
			"GET /health",
			"GET /v1/health",
			"GET /v1/models",
			"GET /v1/stats",
			"POST /v1/responses",
			"POST /v1/chat/completions",
			"POST /v1/completions",
			"POST /v1/messages",
			"POST /v1/messages/count_tokens",
		},
	})
}

func isRootPathAlias(path string) bool {
	trimmed := strings.TrimSpace(path)
	return trimmed == "/" || trimmed == compatV1Prefix || trimmed == compatV1Path(compatV1Prefix)
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	r, requestID := s.prepareRequestContext(r)
	s.applyCommonHeaders(w)
	w.Header().Set("X-Request-ID", requestID)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if secErr := s.validateSecurityHeaders(r); secErr.Message != "" {
		s.logRequestEvent("warn", requestID, "rejected", fmt.Sprintf("route=%q", "stats"), fmt.Sprintf("reason=%q", secErr.Message))
		s.writeGenericError(w, secErr.Status, secErr.Type, secErr.Message)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	snapshot := s.store.Snapshot()
	accounts := s.store.Accounts()
	enabled := 0
	availableSnapshot := s.pool.AvailabilitySnapshot("")
	for _, account := range accounts {
		if account.Enabled {
			enabled++
		}
	}

	_ = json.NewEncoder(w).Encode(map[string]any{
		"status":          "ok",
		"accounts":        len(accounts),
		"enabledAccounts": enabled,
		"available":       availableSnapshot.ReadyCount,
		"availability":    availableSnapshot,
		"stats":           snapshot.Stats,
	})
}

func (s *Server) handleEventLogging(w http.ResponseWriter, r *http.Request) {
	r, requestID := s.prepareRequestContext(r)
	s.applyCommonHeaders(w)
	w.Header().Set("X-Request-ID", requestID)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if secErr := s.validateSecurityHeaders(r); secErr.Message != "" {
		s.logRequestEvent("warn", requestID, "rejected", fmt.Sprintf("route=%q", "event_logging"), fmt.Sprintf("reason=%q", secErr.Message))
		s.writeGenericError(w, secErr.Status, secErr.Type, secErr.Message)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok"})
}

func (s *Server) handleModels(w http.ResponseWriter, r *http.Request) {
	r, requestID := s.prepareRequestContext(r)
	s.applyCommonHeaders(w)
	w.Header().Set("X-Request-ID", requestID)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if secErr := s.validateSecurityHeaders(r); secErr.Message != "" {
		s.logRequestEvent("warn", requestID, "rejected", fmt.Sprintf("route=%q", "models"), fmt.Sprintf("reason=%q", secErr.Message))
		s.writeGenericError(w, secErr.Status, secErr.Type, secErr.Message)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	models := models.CatalogModels()
	data := make([]map[string]any, 0, len(models))
	for _, model := range models {
		data = append(data, map[string]any{"id": model.ID, "object": "model", "owned_by": model.OwnedBy})
	}
	_ = json.NewEncoder(w).Encode(map[string]any{
		"object": "list",
		"data":   data,
	})
}

type APIError struct {
	Status  int
	Type    string
	Message string
}

func (e APIError) Error() string {
	return e.Message
}

func InvalidRequest(message string) APIError {
	return APIError{Status: http.StatusBadRequest, Type: "invalid_request_error", Message: message}
}

func ServerError(message string) APIError {
	return APIError{Status: http.StatusInternalServerError, Type: "server_error", Message: message}
}

func Unauthorized(message string) APIError {
	return APIError{Status: http.StatusUnauthorized, Type: "authentication_error", Message: message}
}

func Forbidden(message string) APIError {
	return APIError{Status: http.StatusForbidden, Type: "permission_error", Message: message}
}

func (s *Server) validateSecurityHeaders(r *http.Request) APIError {
	if r == nil {
		return InvalidRequest("request is required")
	}
	if s.store == nil {
		return ServerError("store unavailable")
	}

	configuredKey := strings.TrimSpace(s.store.ProxyAPIKey())
	providedKey, err := resolveProxyCredential(r)
	if err != nil {
		return InvalidRequest(err.Error())
	}

	if !s.store.AuthorizationMode() {
		return APIError{}
	}
	if configuredKey == "" {
		return Forbidden("authorization mode enabled but proxy API key is not configured")
	}
	if providedKey == "" {
		return Unauthorized("missing proxy API key")
	}
	if subtle.ConstantTimeCompare([]byte(providedKey), []byte(configuredKey)) != 1 {
		return Unauthorized("invalid proxy API key")
	}
	return APIError{}
}

func resolveProxyCredential(r *http.Request) (string, error) {
	if r == nil {
		return "", nil
	}
	authorizationHeader := strings.TrimSpace(r.Header.Get("Authorization"))
	xAPIKey := strings.TrimSpace(r.Header.Get("X-API-Key"))

	resolvedBearer := ""
	if authorizationHeader != "" {
		parts := strings.Fields(authorizationHeader)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || strings.TrimSpace(parts[1]) == "" {
			return "", fmt.Errorf("malformed Authorization header")
		}
		resolvedBearer = strings.TrimSpace(parts[1])
	}

	if resolvedBearer != "" && xAPIKey != "" && subtle.ConstantTimeCompare([]byte(resolvedBearer), []byte(xAPIKey)) != 1 {
		return "", fmt.Errorf("conflicting Authorization and X-API-Key headers")
	}
	if resolvedBearer != "" {
		return resolvedBearer, nil
	}
	return xAPIKey, nil
}
