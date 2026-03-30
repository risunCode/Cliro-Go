package codex

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"cliro-go/internal/account"
	"cliro-go/internal/auth"
	"cliro-go/internal/config"
	"cliro-go/internal/logger"
	"cliro-go/internal/platform"
	"cliro-go/internal/provider"

	"github.com/google/uuid"
)

const (
	codexBaseURL   = "https://chatgpt.com/backend-api/codex"
	codexVersion   = "0.101.0"
	codexUserAgent = "codex_cli_rs/0.101.0 (Windows NT 10.0; Win64; x64)"
	quotaCooldown  = time.Hour
)

type Service struct {
	store      *config.Manager
	auth       *auth.Manager
	pool       *account.Pool
	log        *logger.Logger
	httpClient *http.Client
}

type responseEvent struct {
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

func NewService(store *config.Manager, authManager *auth.Manager, accountPool *account.Pool, log *logger.Logger, httpClient *http.Client) *Service {
	client := httpClient
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Minute}
	}
	return &Service{
		store:      store,
		auth:       authManager,
		pool:       accountPool,
		log:        log,
		httpClient: client,
	}
}

func (s *Service) Complete(ctx context.Context, req provider.ChatRequest) (provider.CompletionOutcome, int, string, error) {
	requestID := platform.RequestIDFromContext(ctx)
	if strings.TrimSpace(req.Model) == "" {
		s.recordRequestFailure()
		s.log.Warn("proxy", fmt.Sprintf("request_id=%q provider=%q route=%q phase=%q reason=%q", requestID, "codex", strings.TrimSpace(req.RouteFamily), "rejected", "model is required"))
		return provider.CompletionOutcome{}, http.StatusBadRequest, "model is required", fmt.Errorf("model is required")
	}

	upstreamCandidates := s.pool.AvailableAccountsForProvider("codex")
	if len(upstreamCandidates) == 0 {
		s.recordRequestFailure()
		reason := s.pool.ProviderUnavailableReason("codex")
		s.log.Warn("proxy", fmt.Sprintf("request_id=%q provider=%q route=%q phase=%q reason=%q", requestID, "codex", strings.TrimSpace(req.RouteFamily), "rejected", reason))
		return provider.CompletionOutcome{}, http.StatusServiceUnavailable, reason, fmt.Errorf(reason)
	}

	var lastStatus int
	var lastMessage string

	for _, candidate := range upstreamCandidates {
		accountLabel := config.AccountLabel(candidate)
		s.log.Info("proxy", fmt.Sprintf("request_id=%q provider=%q route=%q phase=%q account=%q model=%q", requestID, "codex", strings.TrimSpace(req.RouteFamily), "attempt", accountLabel, strings.TrimSpace(req.Model)))
		account, err := s.auth.EnsureFreshAccount(candidate.ID)
		if err != nil {
			decision := provider.ClassifyHTTPFailure(http.StatusUnauthorized, err.Error())
			s.applyFailureDecision(requestID, candidate.ID, accountLabel, decision)
			lastStatus = decision.Status
			lastMessage = decision.Message
			continue
		}
		accountLabel = config.AccountLabel(account)
		refreshedAfterFailure := false

		for {
			upstreamReq, err := s.buildRequest(ctx, account, req)
			if err != nil {
				s.recordRequestFailure()
				return provider.CompletionOutcome{}, http.StatusBadRequest, err.Error(), err
			}

			resp, err := s.httpClient.Do(upstreamReq)
			if err != nil {
				decision := provider.ClassifyTransportFailure(err)
				s.applyFailureDecision(requestID, account.ID, accountLabel, decision)
				lastStatus = decision.Status
				lastMessage = decision.Message
				break
			}

			if resp.StatusCode < 200 || resp.StatusCode >= 300 {
				data, _ := io.ReadAll(resp.Body)
				_ = resp.Body.Close()
				decision := s.handleUpstreamFailure(account, resp.StatusCode, data)
				if decision.Class == provider.FailureAuthRefreshable && !refreshedAfterFailure {
					refreshedAccount, refreshErr := s.auth.RefreshAccount(account.ID)
					if refreshErr == nil {
						account = refreshedAccount
						accountLabel = config.AccountLabel(account)
						refreshedAfterFailure = true
						s.log.Info("auth", fmt.Sprintf("request_id=%q provider=%q phase=%q account=%q", requestID, "codex", "token_refreshed_retry", accountLabel))
						continue
					}
					decision = provider.ClassifyHTTPFailure(http.StatusUnauthorized, refreshErr.Error())
				}

				s.applyFailureDecision(requestID, account.ID, accountLabel, decision)
				lastStatus = decision.Status
				lastMessage = decision.Message
				if decision.Class == provider.FailureRequestShape {
					s.recordRequestFailure()
					return provider.CompletionOutcome{}, decision.Status, decision.Message, fmt.Errorf(decision.Message)
				}
				break
			}

			outcome, err := s.collectCompletion(resp.Body, req.Model)
			_ = resp.Body.Close()
			if err != nil {
				decision := provider.ClassifyTransportFailure(err)
				s.applyFailureDecision(requestID, account.ID, accountLabel, decision)
				lastStatus = decision.Status
				lastMessage = decision.Message
				break
			}
			outcome.Provider = "codex"
			outcome.AccountID = account.ID
			outcome.AccountLabel = accountLabel

			s.markSuccess(requestID, account.ID, accountLabel, outcome.Usage)
			return outcome, 0, "", nil
		}
	}

	snapshot := s.pool.AvailabilitySnapshot("codex")
	if snapshot.ReadyCount == 0 {
		lastStatus = http.StatusServiceUnavailable
		lastMessage = s.pool.ProviderUnavailableReason("codex")
	}
	if lastStatus == 0 {
		lastStatus = http.StatusServiceUnavailable
	}
	if strings.TrimSpace(lastMessage) == "" {
		lastMessage = "all codex accounts failed"
	}
	s.recordRequestFailure()
	s.log.Warn("proxy", fmt.Sprintf("request_id=%q provider=%q route=%q phase=%q reason=%q", requestID, "codex", strings.TrimSpace(req.RouteFamily), "failed", lastMessage))
	return provider.CompletionOutcome{}, lastStatus, lastMessage, fmt.Errorf(lastMessage)
}

func (s *Service) buildRequest(ctx context.Context, account config.Account, req provider.ChatRequest) (*http.Request, error) {
	payload, err := s.buildRequestPayload(req)
	if err != nil {
		return nil, fmt.Errorf("messages are empty")
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

func (s *Service) collectCompletion(body io.Reader, model string) (provider.CompletionOutcome, error) {
	var out provider.CompletionOutcome
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
		var event responseEvent
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

func (s *Service) handleUpstreamFailure(account config.Account, statusCode int, body []byte) provider.FailureDecision {
	message := strings.TrimSpace(string(body))
	if message == "" {
		message = fmt.Sprintf("upstream returned %d", statusCode)
	}

	var event responseEvent
	if err := json.Unmarshal(body, &event); err == nil && event.Error.Message != "" {
		message = event.Error.Message
	}
	decision := provider.ClassifyHTTPFailure(statusCode, message)
	if decision.Class == provider.FailureQuotaCooldown {
		cooldownUntil := time.Now().Add(decision.Cooldown).Unix()
		if err := json.Unmarshal(body, &event); err == nil {
			if event.Error.ResetsAt > time.Now().Unix() {
				cooldownUntil = event.Error.ResetsAt
			} else if event.Error.ResetsInSeconds > 0 {
				cooldownUntil = time.Now().Add(time.Duration(event.Error.ResetsInSeconds) * time.Second).Unix()
			}
		}
		decision.Cooldown = time.Until(time.Unix(cooldownUntil, 0))
		if decision.Cooldown < 0 {
			decision.Cooldown = quotaCooldown
		}
	}
	return decision
}

func (s *Service) markSuccess(requestID string, accountID string, accountLabel string, usage config.ProxyStats) {
	now := time.Now().Unix()
	_ = s.store.UpdateAccount(accountID, func(a *config.Account) {
		a.RequestCount++
		a.PromptTokens += usage.PromptTokens
		a.CompletionTokens += usage.CompletionTokens
		a.TotalTokens += usage.TotalTokens
		a.LastUsed = now
		a.CooldownUntil = 0
		a.ConsecutiveFailures = 0
		a.Banned = false
		a.BannedReason = ""
		a.HealthState = config.AccountHealthReady
		a.HealthReason = ""
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
	s.log.Info("proxy", fmt.Sprintf("request_id=%q provider=%q phase=%q account=%q prompt_tokens=%d completion_tokens=%d total_tokens=%d", requestID, "codex", "success", accountLabel, usage.PromptTokens, usage.CompletionTokens, usage.TotalTokens))
}

func (s *Service) markTransientFailure(requestID string, accountID string, accountLabel string, err error) {
	breakerEnabled := s.store.CircuitBreaker()
	steps := s.store.CircuitSteps()
	now := time.Now().Unix()
	appliedCooldown := time.Duration(0)
	appliedStep := 0
	_ = s.store.UpdateAccount(accountID, func(a *config.Account) {
		a.ErrorCount++
		a.LastError = err.Error()
		a.LastFailureAt = now
		a.Quota.Status = firstNonEmpty(a.Quota.Status, "degraded")
		a.Quota.Summary = err.Error()
		a.Quota.Source = firstNonEmpty(a.Quota.Source, "runtime")
		a.Quota.Error = err.Error()
		a.Quota.LastCheckedAt = now
		if !breakerEnabled {
			a.ConsecutiveFailures = 0
			a.CooldownUntil = 0
			a.HealthState = config.AccountHealthReady
			a.HealthReason = ""
			return
		}
		nextFailures := a.ConsecutiveFailures + 1
		appliedCooldown = provider.CircuitCooldown(steps, nextFailures)
		appliedStep = nextFailures
		a.ConsecutiveFailures = nextFailures
		a.CooldownUntil = now + int64(appliedCooldown/time.Second)
		a.HealthState = config.AccountHealthCooldownTransient
		a.HealthReason = err.Error()
	})
	if breakerEnabled && appliedCooldown > 0 {
		cappedStep := appliedStep
		if cappedStep > len(steps) {
			cappedStep = len(steps)
		}
		s.log.Warn("proxy", fmt.Sprintf("request_id=%q provider=%q phase=%q account=%q reason=%q circuit_step=%d cooldown_seconds=%d", requestID, "codex", "attempt_failed", accountLabel, err.Error(), cappedStep, int(appliedCooldown/time.Second)))
		return
	}
	s.log.Warn("proxy", fmt.Sprintf("request_id=%q provider=%q phase=%q account=%q reason=%q circuit_breaker=%t", requestID, "codex", "attempt_failed", accountLabel, err.Error(), breakerEnabled))
}

func (s *Service) markBanned(requestID string, accountID string, accountLabel string, reason string) {
	_ = s.store.MarkAccountBanned(accountID, reason)
	s.log.Warn("auth", fmt.Sprintf("request_id=%q provider=%q phase=%q account=%q reason=%q", requestID, "codex", "banned", accountLabel, reason))
}

func (s *Service) applyFailureDecision(requestID string, accountID string, accountLabel string, decision provider.FailureDecision) {
	switch decision.Class {
	case provider.FailureRequestShape:
		s.log.Warn("proxy", fmt.Sprintf("request_id=%q provider=%q phase=%q account=%q reason=%q", requestID, "codex", "request_shape", accountLabel, decision.Message))
	case provider.FailureDurableDisabled:
		if decision.BanAccount {
			s.markBanned(requestID, accountID, accountLabel, decision.Message)
			return
		}
		_ = s.store.MarkAccountDurablyDisabled(accountID, decision.Message)
		s.log.Warn("auth", fmt.Sprintf("request_id=%q provider=%q phase=%q account=%q reason=%q", requestID, "codex", "durable_disabled", accountLabel, decision.Message))
	case provider.FailureQuotaCooldown:
		cooldownUntil := time.Now().Add(decision.Cooldown).Unix()
		_ = s.store.UpdateAccount(accountID, func(a *config.Account) {
			a.ErrorCount++
			a.CooldownUntil = cooldownUntil
			a.HealthState = config.AccountHealthCooldownQuota
			a.HealthReason = decision.Message
			a.LastFailureAt = time.Now().Unix()
			a.LastError = decision.Message
			a.Quota = config.QuotaInfo{
				Status:        "exhausted",
				Summary:       decision.Message,
				Source:        "runtime",
				Error:         decision.Message,
				LastCheckedAt: time.Now().Unix(),
				Buckets:       []config.QuotaBucket{{Name: "session", ResetAt: cooldownUntil, Status: "exhausted"}},
			}
		})
		s.log.Warn("quota", fmt.Sprintf("request_id=%q provider=%q account=%q phase=%q reason=%q cooldown_until=%d", requestID, "codex", accountLabel, "cooldown", decision.Message, cooldownUntil))
	default:
		s.markTransientFailure(requestID, accountID, accountLabel, fmt.Errorf(decision.Message))
	}
}

func (s *Service) buildRequestPayload(req provider.ChatRequest) (map[string]any, error) {
	input := make([]any, 0, len(req.Messages))
	for _, msg := range req.Messages {
		items := s.codexMessageItems(msg)
		input = append(input, items...)
	}
	if len(input) == 0 {
		return nil, fmt.Errorf("messages are empty")
	}
	payload := map[string]any{
		"model":               req.Model,
		"input":               input,
		"instructions":        defaultCodexInstructions(),
		"stream":              true,
		"store":               false,
		"include":             []string{"reasoning.encrypted_content"},
		"parallel_tool_calls": true,
	}
	if req.Metadata != nil {
		if previousResponseID, ok := req.Metadata["previousResponseID"].(string); ok && strings.TrimSpace(previousResponseID) != "" {
			payload["previous_response_id"] = strings.TrimSpace(previousResponseID)
		}
		if parallelToolCalls, ok := req.Metadata["parallelToolCalls"].(bool); ok {
			payload["parallel_tool_calls"] = parallelToolCalls
		}
		if instructions, ok := req.Metadata["instructions"].(string); ok && strings.TrimSpace(instructions) != "" {
			payload["instructions"] = defaultCodexInstructions() + "\n\n## Request Context\n\n" + strings.TrimSpace(instructions)
		}
	}
	if len(req.Tools) > 0 {
		payload["tools"] = s.codexTools(req.Tools)
	}
	if req.ToolChoice != nil && req.ToolChoice != "" {
		payload["tool_choice"] = req.ToolChoice
	}
	return payload, nil
}

func (s *Service) codexMessageItems(msg provider.Message) []any {
	role := strings.ToLower(strings.TrimSpace(msg.Role))
	switch role {
	case "system", "developer":
		text := strings.TrimSpace(messageToText(msg.Content))
		if text == "" {
			return nil
		}
		return []any{map[string]any{"type": "message", "role": "developer", "content": []any{map[string]any{"type": "input_text", "text": text}}}}
	case "assistant":
		items := make([]any, 0, 1+len(msg.ToolCalls))
		if text := strings.TrimSpace(messageToText(msg.Content)); text != "" {
			items = append(items, map[string]any{"type": "message", "role": "assistant", "content": []any{map[string]any{"type": "output_text", "text": text}}})
		}
		for _, toolCall := range msg.ToolCalls {
			name := strings.TrimSpace(toolCall.Function.Name)
			if name == "" {
				continue
			}
			arguments := strings.TrimSpace(toolCall.Function.Arguments)
			if arguments == "" {
				arguments = "{}"
			}
			items = append(items, map[string]any{"type": "function_call", "call_id": firstNonEmpty(toolCall.ID, "toolu_"+uuid.NewString()[:8]), "name": name, "arguments": arguments})
		}
		return items
	case "tool":
		toolCallID := strings.TrimSpace(msg.ToolCallID)
		if toolCallID == "" {
			return nil
		}
		return []any{map[string]any{"type": "function_call_output", "call_id": toolCallID, "output": messageToText(msg.Content)}}
	default:
		text := strings.TrimSpace(messageToText(msg.Content))
		if text == "" {
			return nil
		}
		return []any{map[string]any{"type": "message", "role": "user", "content": []any{map[string]any{"type": "input_text", "text": text}}}}
	}
}

func (s *Service) codexTools(tools []provider.Tool) []any {
	converted := make([]any, 0, len(tools))
	for _, tool := range tools {
		name := strings.TrimSpace(tool.Function.Name)
		if name == "" {
			continue
		}
		converted = append(converted, map[string]any{
			"type":        "function",
			"name":        name,
			"description": strings.TrimSpace(tool.Function.Description),
			"parameters":  tool.Function.Parameters,
		})
	}
	if len(converted) == 0 {
		return nil
	}
	return converted
}

func (s *Service) recordRequestFailure() {
	now := time.Now().Unix()
	_ = s.store.UpdateStats(func(stats *config.ProxyStats) {
		stats.TotalRequests++
		stats.FailedRequests++
		stats.LastRequestAt = now
	})
}

func messageToText(content any) string {
	switch typed := content.(type) {
	case nil:
		return ""
	case string:
		return typed
	case []any:
		parts := make([]string, 0, len(typed))
		for _, item := range typed {
			if object, ok := item.(map[string]any); ok {
				text, _ := object["text"].(string)
				if strings.TrimSpace(text) != "" {
					parts = append(parts, text)
				}
			}
		}
		return strings.Join(parts, "\n")
	default:
		data, _ := json.Marshal(typed)
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
