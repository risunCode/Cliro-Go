package proxy

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"cliro-go/internal/auth"
	"cliro-go/internal/config"
	"cliro-go/internal/logger"
	"cliro-go/internal/pool"

	"github.com/google/uuid"
)

const (
	codexBaseURL      = "https://chatgpt.com/backend-api/codex"
	codexVersion      = "0.101.0"
	codexUserAgent    = "codex_cli_rs/0.101.0 (Windows NT 10.0; Win64; x64)"
	requestTimeout    = 5 * time.Minute
	quotaCooldown     = time.Hour
	transientCooldown = time.Minute
)

type Service struct {
	store      *config.Manager
	auth       *auth.Manager
	pool       *pool.Pool
	log        *logger.Logger
	httpClient *http.Client

	mu       sync.Mutex
	server   *http.Server
	running  bool
	started  time.Time
	bindAddr string
}

type openAIChatRequest struct {
	Model       string          `json:"model"`
	Messages    []openAIMessage `json:"messages"`
	Stream      bool            `json:"stream"`
	Temperature *float64        `json:"temperature,omitempty"`
	MaxTokens   *int            `json:"max_tokens,omitempty"`
	User        string          `json:"user,omitempty"`
}

type openAIMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

type codexResponseEvent struct {
	Type     string `json:"type"`
	Delta    string `json:"delta"`
	Text     string `json:"text"`
	Response struct {
		ID    string `json:"id"`
		Usage struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
			TotalTokens  int `json:"total_tokens"`
		} `json:"usage"`
	} `json:"response"`
	Error struct {
		Message         string `json:"message"`
		Type            string `json:"type"`
		ResetsInSeconds int64  `json:"resets_in_seconds"`
		ResetsAt        int64  `json:"resets_at"`
	} `json:"error"`
}

type completionOutcome struct {
	Text  string
	Usage config.ProxyStats
	ID    string
	Model string
}

func NewService(store *config.Manager, authManager *auth.Manager, accountPool *pool.Pool, log *logger.Logger) *Service {
	return &Service{
		store: store,
		auth:  authManager,
		pool:  accountPool,
		log:   log,
		httpClient: &http.Client{
			Timeout: requestTimeout,
		},
	}
}

func (s *Service) Running() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running
}

func (s *Service) BindAddress() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.bindAddr
}

func (s *Service) Start(port int, allowLAN bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running {
		return nil
	}
	host := "127.0.0.1"
	if allowLAN {
		host = "0.0.0.0"
	}
	bindAddr := fmt.Sprintf("%s:%d", host, port)
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleRoot)
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/v1/stats", s.handleStats)
	mux.HandleFunc("/v1/models", s.handleModels)
	mux.HandleFunc("/v1/chat/completions", s.handleChatCompletions)
	mux.HandleFunc("/v1/completions", s.handleCompletions)
	mux.HandleFunc("/api/event_logging/batch", s.handleEventLogging)
	server := &http.Server{Addr: bindAddr, Handler: mux}
	s.server = server
	s.running = true
	s.started = time.Now()
	s.bindAddr = bindAddr
	go func() {
		err := server.ListenAndServe()
		if err != nil && err != http.ErrServerClosed {
			s.log.Error("proxy", "proxy server stopped unexpectedly: "+err.Error())
		}
		s.mu.Lock()
		s.running = false
		s.mu.Unlock()
	}()
	s.log.Info("proxy", fmt.Sprintf("proxy server listening on http://%s", bindAddr))
	return nil
}

func (s *Service) Stop(ctx context.Context) error {
	s.mu.Lock()
	server := s.server
	s.server = nil
	s.running = false
	s.mu.Unlock()
	if server == nil {
		return nil
	}
	s.log.Info("proxy", "stopping proxy server")
	return server.Shutdown(ctx)
}

func (s *Service) handleHealth(w http.ResponseWriter, _ *http.Request) {
	s.applyCommonHeaders(w)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status":     "ok",
		"running":    s.Running(),
		"started_at": s.started.Unix(),
	})
}

func (s *Service) handleRoot(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	s.applyCommonHeaders(w)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"name":    "CLIro-Go Codex Proxy",
		"status":  "ok",
		"running": s.Running(),
		"routes": []string{
			"GET /health",
			"GET /v1/models",
			"GET /v1/stats",
			"POST /v1/chat/completions",
			"POST /v1/completions",
		},
	})
}

func (s *Service) handleStats(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	s.applyCommonHeaders(w)
	w.Header().Set("Content-Type", "application/json")
	snapshot := s.store.Snapshot()
	accounts := s.store.Accounts()
	now := time.Now().Unix()
	enabled := 0
	available := 0
	for _, account := range accounts {
		if account.Enabled {
			enabled++
			if account.CooldownUntil <= now {
				available++
			}
		}
	}
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status":          "ok",
		"accounts":        len(accounts),
		"enabledAccounts": enabled,
		"available":       available,
		"stats":           snapshot.Stats,
	})
}

func (s *Service) handleEventLogging(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	s.applyCommonHeaders(w)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok"})
}

func (s *Service) handleModels(w http.ResponseWriter, _ *http.Request) {
	s.applyCommonHeaders(w)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"object": "list",
		"data": []map[string]any{
			{"id": "gpt-5.1-codex-max", "object": "model", "owned_by": "codex"},
			{"id": "gpt-5.1-codex-mini", "object": "model", "owned_by": "codex"},
			{"id": "gpt-5.2", "object": "model", "owned_by": "codex"},
			{"id": "gpt-5.4", "object": "model", "owned_by": "codex"},
			{"id": "gpt-5.2-codex", "object": "model", "owned_by": "codex"},
			{"id": "gpt-5.3-codex", "object": "model", "owned_by": "codex"},
			{"id": "gpt-5.1-codex", "object": "model", "owned_by": "codex"},
		},
	})
}

func (s *Service) handleCompletions(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		s.applyCommonHeaders(w)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Model  string `json:"model"`
		Prompt string `json:"prompt"`
		Stream bool   `json:"stream"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeOpenAIError(w, http.StatusBadRequest, "invalid_request_error", "invalid JSON")
		return
	}
	chatReq := openAIChatRequest{
		Model:    req.Model,
		Stream:   req.Stream,
		Messages: []openAIMessage{{Role: "user", Content: req.Prompt}},
	}
	s.processChatCompletion(w, r, chatReq)
}

func (s *Service) handleChatCompletions(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodOptions {
		s.applyCommonHeaders(w)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req openAIChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeOpenAIError(w, http.StatusBadRequest, "invalid_request_error", "invalid JSON")
		return
	}
	s.processChatCompletion(w, r, req)
}

func (s *Service) processChatCompletion(w http.ResponseWriter, r *http.Request, req openAIChatRequest) {
	s.applyCommonHeaders(w)
	if strings.TrimSpace(req.Model) == "" {
		req.Model = "gpt-5.2-codex"
	}
	upstreamCandidates := s.pool.AvailableAccounts()
	if len(upstreamCandidates) == 0 {
		s.recordRequestFailure()
		s.writeOpenAIError(w, http.StatusServiceUnavailable, "server_error", "no available accounts")
		return
	}

	var lastStatus int
	var lastMessage string

	for _, candidate := range upstreamCandidates {
		account, err := s.auth.EnsureFreshAccount(candidate.ID)
		if err != nil {
			s.markTransientFailure(candidate.ID, candidate.Email, err)
			lastStatus = http.StatusUnauthorized
			lastMessage = "failed to refresh account token"
			continue
		}

		s.log.Info("proxy", fmt.Sprintf("dispatching model=%s stream=%t via %s", req.Model, req.Stream, firstNonEmpty(account.Email, account.ID)))

		upstreamReq, err := s.buildCodexRequest(r.Context(), account, req)
		if err != nil {
			s.recordRequestFailure()
			s.writeOpenAIError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
			return
		}

		resp, err := s.httpClient.Do(upstreamReq)
		if err != nil {
			s.markTransientFailure(account.ID, account.Email, err)
			lastStatus = http.StatusBadGateway
			lastMessage = err.Error()
			continue
		}

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			data, _ := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			status, message := s.handleUpstreamFailure(account, resp.StatusCode, data)
			lastStatus = status
			lastMessage = message
			if resp.StatusCode >= 400 && resp.StatusCode < 500 && resp.StatusCode != http.StatusUnauthorized && resp.StatusCode != http.StatusForbidden && resp.StatusCode != http.StatusTooManyRequests {
				break
			}
			continue
		}

		if req.Stream {
			s.streamOpenAIResponse(w, resp.Body, req.Model, account)
			_ = resp.Body.Close()
			return
		}

		outcome, err := s.collectCompletion(resp.Body, req.Model)
		_ = resp.Body.Close()
		if err != nil {
			s.markTransientFailure(account.ID, account.Email, err)
			lastStatus = http.StatusBadGateway
			lastMessage = err.Error()
			continue
		}

		s.markSuccess(account.ID, outcome.Usage)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":      outcome.ID,
			"object":  "chat.completion",
			"created": time.Now().Unix(),
			"model":   req.Model,
			"choices": []map[string]any{{
				"index": 0,
				"message": map[string]any{
					"role":    "assistant",
					"content": outcome.Text,
				},
				"finish_reason": "stop",
			}},
			"usage": map[string]int{
				"prompt_tokens":     outcome.Usage.PromptTokens,
				"completion_tokens": outcome.Usage.CompletionTokens,
				"total_tokens":      outcome.Usage.TotalTokens,
			},
		})
		return
	}

	if lastStatus == 0 {
		lastStatus = http.StatusServiceUnavailable
	}
	if strings.TrimSpace(lastMessage) == "" {
		lastMessage = "all accounts failed"
	}
	s.recordRequestFailure()
	s.writeOpenAIError(w, lastStatus, "server_error", lastMessage)
}

func (s *Service) buildCodexRequest(ctx context.Context, account config.Account, req openAIChatRequest) (*http.Request, error) {
	input := make([]map[string]any, 0, len(req.Messages))
	for _, msg := range req.Messages {
		text := messageToText(msg.Content)
		if text == "" {
			continue
		}
		role := strings.ToLower(strings.TrimSpace(msg.Role))
		if role == "system" {
			role = "developer"
		}
		if role == "" {
			role = "user"
		}
		input = append(input, map[string]any{
			"type": "message",
			"role": role,
			"content": []map[string]any{{
				"type": "input_text",
				"text": text,
			}},
		})
	}
	if len(input) == 0 {
		return nil, fmt.Errorf("messages are empty")
	}
	payload := map[string]any{
		"model":               req.Model,
		"input":               input,
		"stream":              true,
		"store":               false,
		"include":             []string{"reasoning.encrypted_content"},
		"parallel_tool_calls": true,
		"instructions":        "",
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, codexBaseURL+"/responses", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	httpReq.Header.Set("Authorization", "Bearer "+account.AccessToken)
	httpReq.Header.Set("Version", codexVersion)
	httpReq.Header.Set("Session_id", uuid.NewString())
	httpReq.Header.Set("User-Agent", codexUserAgent)
	httpReq.Header.Set("Connection", "Keep-Alive")
	httpReq.Header.Set("Originator", "codex_cli_rs")
	if strings.TrimSpace(account.AccountID) != "" {
		httpReq.Header.Set("Chatgpt-Account-Id", account.AccountID)
	}
	return httpReq, nil
}

func (s *Service) collectCompletion(body io.Reader, model string) (completionOutcome, error) {
	var out completionOutcome
	out.ID = "chatcmpl-" + uuid.NewString()
	out.Model = model
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)
	var builder strings.Builder
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" || payload == "[DONE]" {
			continue
		}
		var event codexResponseEvent
		if err := json.Unmarshal([]byte(payload), &event); err != nil {
			continue
		}
		switch event.Type {
		case "response.output_text.delta":
			if event.Delta != "" {
				builder.WriteString(event.Delta)
			} else if event.Text != "" {
				builder.WriteString(event.Text)
			}
		case "response.completed":
			out.ID = firstNonEmpty(event.Response.ID, out.ID)
			out.Usage.PromptTokens = event.Response.Usage.InputTokens
			out.Usage.CompletionTokens = event.Response.Usage.OutputTokens
			out.Usage.TotalTokens = event.Response.Usage.TotalTokens
		case "error":
			return out, fmt.Errorf(firstNonEmpty(event.Error.Message, "upstream error"))
		}
	}
	if err := scanner.Err(); err != nil {
		return out, err
	}
	out.Text = builder.String()
	if out.Usage.TotalTokens == 0 {
		out.Usage.TotalTokens = out.Usage.PromptTokens + out.Usage.CompletionTokens
	}
	return out, nil
}

func (s *Service) streamOpenAIResponse(w http.ResponseWriter, body io.Reader, model string, account config.Account) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	flusher, ok := w.(http.Flusher)
	if !ok {
		s.writeOpenAIError(w, http.StatusInternalServerError, "server_error", "streaming not supported")
		return
	}
	chatID := "chatcmpl-" + uuid.NewString()
	initial, _ := json.Marshal(map[string]any{
		"id":      chatID,
		"object":  "chat.completion.chunk",
		"created": time.Now().Unix(),
		"model":   model,
		"choices": []map[string]any{{"index": 0, "delta": map[string]any{"role": "assistant"}, "finish_reason": nil}},
	})
	_, _ = fmt.Fprintf(w, "data: %s\n\n", initial)
	flusher.Flush()

	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)
	usage := config.ProxyStats{}
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" || payload == "[DONE]" {
			continue
		}
		var event codexResponseEvent
		if err := json.Unmarshal([]byte(payload), &event); err != nil {
			continue
		}
		switch event.Type {
		case "response.output_text.delta":
			text := firstNonEmpty(event.Delta, event.Text)
			if text == "" {
				continue
			}
			chunk, _ := json.Marshal(map[string]any{
				"id":      chatID,
				"object":  "chat.completion.chunk",
				"created": time.Now().Unix(),
				"model":   model,
				"choices": []map[string]any{{"index": 0, "delta": map[string]any{"content": text}, "finish_reason": nil}},
			})
			_, _ = fmt.Fprintf(w, "data: %s\n\n", chunk)
			flusher.Flush()
		case "response.completed":
			usage.PromptTokens = event.Response.Usage.InputTokens
			usage.CompletionTokens = event.Response.Usage.OutputTokens
			usage.TotalTokens = event.Response.Usage.TotalTokens
		case "error":
			s.markTransientFailure(account.ID, account.Email, fmt.Errorf(firstNonEmpty(event.Error.Message, "upstream error")))
			s.recordRequestFailure()
			return
		}
	}
	if err := scanner.Err(); err != nil {
		s.markTransientFailure(account.ID, account.Email, err)
		s.recordRequestFailure()
		return
	}
	s.markSuccess(account.ID, usage)
	finalChunk, _ := json.Marshal(map[string]any{
		"id":      chatID,
		"object":  "chat.completion.chunk",
		"created": time.Now().Unix(),
		"model":   model,
		"choices": []map[string]any{{"index": 0, "delta": map[string]any{}, "finish_reason": "stop"}},
	})
	_, _ = fmt.Fprintf(w, "data: %s\n\n", finalChunk)
	_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
	flusher.Flush()
}

func (s *Service) handleUpstreamFailure(account config.Account, statusCode int, body []byte) (int, string) {
	message := strings.TrimSpace(string(body))
	if message == "" {
		message = fmt.Sprintf("upstream returned %d", statusCode)
	}
	var event codexResponseEvent
	if err := json.Unmarshal(body, &event); err == nil && event.Error.Message != "" {
		message = event.Error.Message
	}
	if statusCode == http.StatusUnauthorized || statusCode == http.StatusForbidden {
		s.markTransientFailure(account.ID, account.Email, fmt.Errorf(message))
		s.log.Warn("auth", firstNonEmpty(account.Email, account.ID)+" upstream auth rejected request")
		return http.StatusUnauthorized, message
	}
	if statusCode == http.StatusTooManyRequests || strings.Contains(strings.ToLower(message), "usage_limit_reached") {
		cooldownUntil := time.Now().Add(quotaCooldown).Unix()
		if err := json.Unmarshal(body, &event); err == nil {
			if event.Error.ResetsAt > time.Now().Unix() {
				cooldownUntil = event.Error.ResetsAt
			} else if event.Error.ResetsInSeconds > 0 {
				cooldownUntil = time.Now().Add(time.Duration(event.Error.ResetsInSeconds) * time.Second).Unix()
			}
		}
		_ = s.store.UpdateAccount(account.ID, func(a *config.Account) {
			a.CooldownUntil = cooldownUntil
			a.LastError = message
			a.ErrorCount++
			a.Quota = config.QuotaInfo{
				Status:        "exhausted",
				Summary:       message,
				Source:        "runtime",
				Error:         message,
				LastCheckedAt: time.Now().Unix(),
				Buckets: []config.QuotaBucket{{
					Name:    "session",
					ResetAt: cooldownUntil,
					Status:  "exhausted",
				}},
			}
		})
		s.log.Warn("quota", account.Email+" entered cooldown: "+message)
		return http.StatusTooManyRequests, message
	}
	s.markTransientFailure(account.ID, account.Email, fmt.Errorf(message))
	return http.StatusBadGateway, message
}

func (s *Service) markSuccess(accountID string, usage config.ProxyStats) {
	now := time.Now().Unix()
	_ = s.store.UpdateAccount(accountID, func(a *config.Account) {
		a.RequestCount++
		a.PromptTokens += usage.PromptTokens
		a.CompletionTokens += usage.CompletionTokens
		a.TotalTokens += usage.TotalTokens
		a.LastUsed = now
		a.CooldownUntil = 0
		a.LastError = ""
		if a.Quota.Status == "exhausted" || a.Quota.Status == "unknown" || a.Quota.Status == "degraded" {
			a.Quota.Status = "healthy"
			a.Quota.Summary = "Recent request succeeded."
			a.Quota.Source = firstNonEmpty(a.Quota.Source, "runtime")
			a.Quota.Error = ""
			a.Quota.LastCheckedAt = now
			for i := range a.Quota.Buckets {
				if a.Quota.Buckets[i].Status == "exhausted" || a.Quota.Buckets[i].Status == "unknown" {
					a.Quota.Buckets[i].Status = "healthy"
				}
			}
		}
	})
	_ = s.store.UpdateStats(func(stats *config.ProxyStats) {
		stats.TotalRequests++
		stats.SuccessRequests++
		stats.PromptTokens += usage.PromptTokens
		stats.CompletionTokens += usage.CompletionTokens
		stats.TotalTokens += usage.TotalTokens
		stats.LastRequestAt = now
	})
	s.log.Info("proxy", fmt.Sprintf("completion request completed successfully (%d tokens)", usage.TotalTokens))
}

func (s *Service) markTransientFailure(accountID, email string, err error) {
	_ = s.store.UpdateAccount(accountID, func(a *config.Account) {
		a.ErrorCount++
		a.LastError = err.Error()
		a.CooldownUntil = time.Now().Add(transientCooldown).Unix()
		a.Quota.Status = firstNonEmpty(a.Quota.Status, "degraded")
		a.Quota.Summary = err.Error()
		a.Quota.Source = firstNonEmpty(a.Quota.Source, "runtime")
		a.Quota.Error = err.Error()
		a.Quota.LastCheckedAt = time.Now().Unix()
	})
	s.log.Warn("proxy", email+" request failed: "+err.Error())
}

func (s *Service) recordRequestFailure() {
	now := time.Now().Unix()
	_ = s.store.UpdateStats(func(stats *config.ProxyStats) {
		stats.TotalRequests++
		stats.FailedRequests++
		stats.LastRequestAt = now
	})
}

func (s *Service) applyCommonHeaders(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
}

func (s *Service) writeOpenAIError(w http.ResponseWriter, status int, errType, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]any{
			"type":    errType,
			"message": message,
		},
	})
}

func messageToText(content any) string {
	switch v := content.(type) {
	case string:
		return v
	case []any:
		parts := make([]string, 0, len(v))
		for _, item := range v {
			if m, ok := item.(map[string]any); ok {
				text, _ := m["text"].(string)
				if strings.TrimSpace(text) != "" {
					parts = append(parts, text)
				}
			}
		}
		return strings.Join(parts, "\n")
	default:
		data, _ := json.Marshal(v)
		return strings.TrimSpace(string(data))
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func ParseRetryAfterSeconds(value string) int64 {
	seconds, _ := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	return seconds
}
