package codex

import (
	"bufio"
	"bytes"
	"cliro-go/internal/util"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"cliro-go/internal/account"
	"cliro-go/internal/config"
	contract "cliro-go/internal/contract"
	"cliro-go/internal/contract/rules"
	"cliro-go/internal/logger"
	"cliro-go/internal/platform"
	"cliro-go/internal/provider"

	"github.com/google/uuid"
)

const (
	codexBaseURL          = "https://chatgpt.com/backend-api/codex"
	codexVersion          = "0.117.0"
	quotaCooldown         = time.Hour
	defaultRequestTimeout = 5 * time.Minute
)

var codexUserAgent = platform.BuildOpencodeUserAgent()

type Service struct {
	store      *config.Manager
	auth       accountAuth
	pool       *account.Pool
	log        *logger.Logger
	httpClient *http.Client
}

type accountAuth interface {
	EnsureFreshAccount(accountID string) (config.Account, error)
	RefreshAccount(accountID string) (config.Account, error)
}

type responseEvent struct {
	Type     string       `json:"type"`
	Delta    string       `json:"delta"`
	Text     string       `json:"text"`
	Item     responseItem `json:"item"`
	Response struct {
		ID    string `json:"id"`
		Usage struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
			TotalTokens  int `json:"total_tokens"`
		} `json:"usage"`
		Output []responseItem `json:"output"`
	} `json:"response"`
	Error struct {
		Message         string `json:"message"`
		Type            string `json:"type"`
		ResetsInSeconds int64  `json:"resets_in_seconds"`
		ResetsAt        int64  `json:"resets_at"`
	} `json:"error"`
}

type responseItem struct {
	ID               string            `json:"id"`
	Type             string            `json:"type"`
	Role             string            `json:"role"`
	Status           string            `json:"status"`
	CallID           string            `json:"call_id"`
	Name             string            `json:"name"`
	Arguments        string            `json:"arguments"`
	EncryptedContent string            `json:"encrypted_content"`
	Content          []responseContent `json:"content"`
	Summary          []responseContent `json:"summary"`
}

type responseContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func NewService(store *config.Manager, authManager accountAuth, accountPool *account.Pool, log *logger.Logger, httpClient *http.Client) *Service {
	client := httpClient
	if client == nil {
		client = newHTTPClient(defaultRequestTimeout)
	}
	return &Service{
		store:      store,
		auth:       authManager,
		pool:       accountPool,
		log:        log,
		httpClient: client,
	}
}

func newHTTPClient(timeout time.Duration) *http.Client {
	if timeout <= 0 {
		timeout = defaultRequestTimeout
	}
	return &http.Client{Timeout: timeout}
}

func (s *Service) ExecuteFromIR(ctx context.Context, request contract.Request) (provider.CompletionOutcome, int, string, error) {
	return s.Complete(ctx, provider.RequestFromIR(request))
}

func (s *Service) Complete(ctx context.Context, req provider.ChatRequest) (provider.CompletionOutcome, int, string, error) {
	requestID := platform.RequestIDFromContext(ctx)
	if strings.TrimSpace(req.Model) == "" {
		s.recordRequestFailure()
		s.logProxyEvent("warn", "request.rejected", requestID, logger.String("route", strings.TrimSpace(req.RouteFamily)), logger.String("reason", "model is required"))
		return provider.CompletionOutcome{}, http.StatusBadRequest, "model is required", fmt.Errorf("model is required")
	}

	upstreamCandidates := s.pool.AvailableAccountsForProvider("codex")
	if len(upstreamCandidates) == 0 {
		s.recordRequestFailure()
		reason := s.pool.ProviderUnavailableReason("codex")
		s.logProxyEvent("warn", "request.rejected", requestID, logger.String("route", strings.TrimSpace(req.RouteFamily)), logger.String("reason", reason))
		return provider.CompletionOutcome{}, http.StatusServiceUnavailable, reason, fmt.Errorf(reason)
	}

	var lastStatus int
	var lastMessage string

	for _, candidate := range upstreamCandidates {
		accountLabel := config.AccountLabel(candidate)
		s.logProxyEvent("info", "request.attempt", requestID, logger.String("route", strings.TrimSpace(req.RouteFamily)), logger.String("account", accountLabel), logger.String("model", strings.TrimSpace(req.Model)))
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
						s.logAuthEvent("info", "auth.token_refreshed_retry", requestID, logger.String("account", accountLabel))
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
	s.logProxyEvent("warn", "request.failed", requestID, logger.String("route", strings.TrimSpace(req.RouteFamily)), logger.String("reason", lastMessage))
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
	httpReq.Header.Set("Session_id", uuid.NewString())
	httpReq.Header.Set("User-Agent", codexUserAgent)
	httpReq.Header.Set("Connection", "Keep-Alive")
	httpReq.Header.Set("Originator", "opencode")
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
	var textBuilder strings.Builder
	var thinkingBuilder strings.Builder
	toolUses := make([]provider.ToolUse, 0)
	seenToolUseIDs := make(map[string]struct{})
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
				textBuilder.WriteString(event.Delta)
			} else if event.Text != "" {
				textBuilder.WriteString(event.Text)
			}
		case "response.reasoning.delta", "response.reasoning_text.delta", "response.reasoning_summary_text.delta", "response.output_reasoning.delta":
			if event.Delta != "" {
				thinkingBuilder.WriteString(event.Delta)
			} else if event.Text != "" {
				thinkingBuilder.WriteString(event.Text)
			}
		case "response.output_item.done":
			collectResponseItem(&textBuilder, &thinkingBuilder, &toolUses, seenToolUseIDs, event.Item)
		case "response.completed":
			out.ID = util.FirstNonEmpty(event.Response.ID, out.ID)
			out.Usage.PromptTokens = event.Response.Usage.InputTokens
			out.Usage.CompletionTokens = event.Response.Usage.OutputTokens
			out.Usage.TotalTokens = event.Response.Usage.TotalTokens
			for _, item := range event.Response.Output {
				collectResponseItem(&textBuilder, &thinkingBuilder, &toolUses, seenToolUseIDs, item)
			}
		case "error":
			return out, fmt.Errorf(util.FirstNonEmpty(event.Error.Message, "upstream error"))
		}
	}
	if err := scanner.Err(); err != nil {
		return out, err
	}

	out.Text = textBuilder.String()
	out.Thinking = thinkingBuilder.String()
	out.ThinkingSignature = contract.StableThinkingSignature(out.Thinking)
	if strings.TrimSpace(out.Thinking) != "" {
		out.ThinkingSource = "native"
	} else {
		out.ThinkingSource = "none"
	}
	out.ToolUses = toolUses
	if out.Usage.TotalTokens == 0 {
		out.Usage.TotalTokens = out.Usage.PromptTokens + out.Usage.CompletionTokens
	}
	return out, nil
}

func collectResponseItem(textBuilder *strings.Builder, thinkingBuilder *strings.Builder, toolUses *[]provider.ToolUse, seenToolUseIDs map[string]struct{}, item responseItem) {
	switch strings.ToLower(strings.TrimSpace(item.Type)) {
	case "function_call":
		toolUseID := util.FirstNonEmpty(strings.TrimSpace(item.CallID), strings.TrimSpace(item.ID))
		toolName := strings.TrimSpace(item.Name)
		if toolUseID == "" || toolName == "" {
			return
		}
		if _, exists := seenToolUseIDs[toolUseID]; exists {
			return
		}
		seenToolUseIDs[toolUseID] = struct{}{}
		*toolUses = append(*toolUses, provider.ToolUse{
			ID:    toolUseID,
			Name:  toolName,
			Input: remappedCodexToolArgs(toolName, item.Arguments),
		})
	case "message":
		if textBuilder.Len() == 0 {
			if text := responseItemText(item.Content); text != "" {
				textBuilder.WriteString(text)
			}
		}
	case "reasoning":
		if thinkingBuilder.Len() == 0 {
			if text := util.FirstNonEmpty(responseItemText(item.Summary), responseItemText(item.Content)); text != "" {
				thinkingBuilder.WriteString(text)
			}
		}
	}
}

func responseItemText(content []responseContent) string {
	if len(content) == 0 {
		return ""
	}
	parts := make([]string, 0, len(content))
	for _, part := range content {
		if strings.TrimSpace(part.Text) != "" {
			parts = append(parts, part.Text)
		}
	}
	return strings.TrimSpace(strings.Join(parts, ""))
}

func remappedCodexToolArgs(name string, arguments string) map[string]any {
	input := map[string]any{}
	if strings.TrimSpace(arguments) != "" {
		_ = json.Unmarshal([]byte(arguments), &input)
	}
	return rules.RemapToolCallArgs(name, input)
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
			a.Quota.Source = util.FirstNonEmpty(a.Quota.Source, "runtime")
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
	s.logProxyEvent("info", "request.success", requestID, logger.String("account", accountLabel), logger.Int("prompt_tokens", usage.PromptTokens), logger.Int("completion_tokens", usage.CompletionTokens), logger.Int("total_tokens", usage.TotalTokens))
}

func (s *Service) markTransientFailure(requestID string, accountID string, accountLabel string, err error) {
	now := time.Now().Unix()
	detail := strings.TrimSpace(err.Error())
	if detail == "" {
		detail = "request failed"
	}
	appliedCooldown := time.Duration(0)
	appliedFailures := 0
	_ = s.store.UpdateAccount(accountID, func(a *config.Account) {
		a.ErrorCount++
		a.LastError = detail
		a.LastFailureAt = now
		a.Quota.Status = util.FirstNonEmpty(a.Quota.Status, "degraded")
		a.Quota.Summary = "Request failed"
		a.Quota.Source = util.FirstNonEmpty(a.Quota.Source, "runtime")
		a.Quota.Error = detail
		a.Quota.LastCheckedAt = now
		nextFailures := a.ConsecutiveFailures + 1
		appliedCooldown = provider.TransientCooldown(nextFailures)
		appliedFailures = nextFailures
		a.ConsecutiveFailures = nextFailures
		a.CooldownUntil = now + int64(appliedCooldown/time.Second)
		a.HealthState = config.AccountHealthCooldownTransient
		a.HealthReason = detail
	})
	if appliedCooldown > 0 {
		s.logProxyEvent("warn", "request.attempt_failed", requestID, logger.String("account", accountLabel), logger.String("reason", detail), logger.Int("failure_count", appliedFailures), logger.Int("cooldown_seconds", int(appliedCooldown/time.Second)))
	}
}

func (s *Service) markBanned(requestID string, accountID string, accountLabel string, reason string) {
	_ = s.store.MarkAccountBanned(accountID, reason)
	s.logAuthEvent("warn", "account.banned", requestID, logger.String("account", accountLabel), logger.String("reason", reason))
}

func (s *Service) applyFailureDecision(requestID string, accountID string, accountLabel string, decision provider.FailureDecision) {
	switch decision.Class {
	case provider.FailureRequestShape:
		s.logProxyEvent("warn", "request.shape_invalid", requestID, logger.String("account", accountLabel), logger.String("reason", decision.Message))
	case provider.FailureDurableDisabled:
		if decision.BanAccount {
			s.markBanned(requestID, accountID, accountLabel, decision.Message)
			return
		}
		_ = s.store.MarkAccountDurablyDisabled(accountID, decision.Message)
		s.logAuthEvent("warn", "account.durable_disabled", requestID, logger.String("account", accountLabel), logger.String("reason", decision.Message))
	case provider.FailureAuthRefreshable:
		cooldownUntil := time.Now().Add(maxDuration(decision.Cooldown, 30*time.Second)).Unix()
		_ = s.store.UpdateAccount(accountID, func(a *config.Account) {
			a.ErrorCount++
			a.CooldownUntil = cooldownUntil
			a.HealthState = config.AccountHealthCooldownTransient
			a.HealthReason = "Need re-login"
			a.LastFailureAt = time.Now().Unix()
			a.LastError = decision.Message
			a.Quota = config.QuotaInfo{
				Status:        "unknown",
				Summary:       "Authentication required",
				Source:        "runtime",
				Error:         decision.Message,
				LastCheckedAt: time.Now().Unix(),
			}
		})
		s.logAuthEvent("warn", "auth.relogin_required", requestID, logger.String("account", accountLabel), logger.String("reason", decision.Message))
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
				Summary:       "Quota exhausted",
				Source:        "runtime",
				Error:         decision.Message,
				LastCheckedAt: time.Now().Unix(),
				Buckets:       []config.QuotaBucket{{Name: "session", ResetAt: cooldownUntil, Status: "exhausted"}},
			}
		})
		s.logQuotaEvent("warn", "quota.cooldown", requestID, logger.String("account", accountLabel), logger.String("reason", decision.Message), logger.Int64("cooldown_until", cooldownUntil))
	default:
		s.markTransientFailure(requestID, accountID, accountLabel, fmt.Errorf(decision.Message))
	}
}

func maxDuration(current time.Duration, fallback time.Duration) time.Duration {
	if current > 0 {
		return current
	}
	if fallback > 0 {
		return fallback
	}
	return 0
}

func (s *Service) logProxyEvent(level string, event string, requestID string, fields ...logger.Field) {
	s.logEvent(level, "proxy", event, requestID, fields...)
}

func (s *Service) logAuthEvent(level string, event string, requestID string, fields ...logger.Field) {
	s.logEvent(level, "auth", event, requestID, fields...)
}

func (s *Service) logQuotaEvent(level string, event string, requestID string, fields ...logger.Field) {
	s.logEvent(level, "quota", event, requestID, fields...)
}

func (s *Service) logEvent(level string, scope string, event string, requestID string, fields ...logger.Field) {
	eventFields := append([]logger.Field{logger.String("request_id", requestID), logger.String("provider", "codex")}, fields...)
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "warn":
		s.log.WarnEvent(scope, event, eventFields...)
	case "error":
		s.log.ErrorEvent(scope, event, eventFields...)
	case "debug":
		s.log.DebugEvent(scope, event, eventFields...)
	default:
		s.log.InfoEvent(scope, event, eventFields...)
	}
}

func (s *Service) recordRequestFailure() {
	now := time.Now().Unix()
	_ = s.store.UpdateStats(func(stats *config.ProxyStats) {
		stats.TotalRequests++
		stats.FailedRequests++
		stats.LastRequestAt = now
	})
}
