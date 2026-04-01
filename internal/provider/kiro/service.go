package kiro

import (
	"cliro-go/internal/util"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"cliro-go/internal/account"
	"cliro-go/internal/config"
	contract "cliro-go/internal/contract"
	"cliro-go/internal/logger"
	"cliro-go/internal/platform"
	provider "cliro-go/internal/provider"

	"github.com/google/uuid"
)

var ErrFirstTokenTimeout = errors.New("kiro first token timeout")

const (
	kiroPrimaryURL          = "https://q.us-east-1.amazonaws.com/generateAssistantResponse"
	kiroFallbackURL         = "https://codewhisperer.us-east-1.amazonaws.com/generateAssistantResponse"
	kiroFallbackTarget      = "AmazonCodeWhispererStreamingService.GenerateAssistantResponse"
	kiroContentType         = "application/json"
	kiroAcceptStream        = "application/vnd.amazon.eventstream"
	kiroRuntimeUserAgent    = "aws-sdk-js/1.0.27 ua/2.1 os/linux lang/js md/nodejs#22.21.1 api/codewhispererstreaming#1.0.27 m/E KiroIDE-0.10.32"
	kiroRuntimeAmzUserAgent = "aws-sdk-js/1.0.27 KiroIDE 0.10.32"
	kiroAgentMode           = "vibe"
	kiroFirstTokenTimeout   = 10 * time.Second
	kiroFirstTokenRetries   = 5
	kiroMaxTimeout          = 30 * time.Second
	kiroDefaultOrigin       = "AI_EDITOR"
	kiroDefaultMaxTokens    = 32000
)

type endpointConfig struct {
	Name      string
	URL       string
	AmzTarget string
}

var endpointConfigs = []endpointConfig{
	{Name: "amazonq", URL: kiroPrimaryURL},
	{Name: "codewhisperer", URL: kiroFallbackURL, AmzTarget: kiroFallbackTarget},
}

type runtimeClient struct {
	httpClient        *http.Client
	firstTokenTimeout time.Duration
}

type Service struct {
	store             *config.Manager
	auth              accountAuth
	pool              *account.Pool
	log               *logger.Logger
	httpClient        *http.Client
	firstTokenTimeout time.Duration
}

type accountAuth interface {
	EnsureFreshAccount(accountID string) (config.Account, error)
	RefreshAccount(accountID string) (config.Account, error)
}

func NewService(store *config.Manager, authManager accountAuth, accountPool *account.Pool, log *logger.Logger, httpClient *http.Client) *Service {
	client := httpClient
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Minute}
	}
	return &Service{
		store:             store,
		auth:              authManager,
		pool:              accountPool,
		log:               log,
		httpClient:        client,
		firstTokenTimeout: kiroFirstTokenTimeout,
	}
}

func (s *Service) Complete(ctx context.Context, req provider.ChatRequest) (provider.CompletionOutcome, int, string, error) {
	return s.CompleteWithCallback(ctx, req, nil)
}

func (s *Service) CompleteWithCallback(ctx context.Context, req provider.ChatRequest, eventCallback func(StreamEvent)) (provider.CompletionOutcome, int, string, error) {
	requestID := platform.RequestIDFromContext(ctx)
	if strings.TrimSpace(req.Model) == "" {
		s.recordRequestFailure()
		s.logProxyEvent("warn", "request.rejected", requestID, logger.String("route", strings.TrimSpace(req.RouteFamily)), logger.String("reason", "model is required"))
		return provider.CompletionOutcome{}, http.StatusBadRequest, "model is required", fmt.Errorf("model is required")
	}

	upstreamCandidates := s.pool.AvailableAccountsForProvider("kiro")
	if len(upstreamCandidates) == 0 {
		s.recordRequestFailure()
		reason := s.pool.ProviderUnavailableReason("kiro")
		s.logProxyEvent("warn", "request.rejected", requestID, logger.String("route", strings.TrimSpace(req.RouteFamily)), logger.String("reason", reason))
		return provider.CompletionOutcome{}, http.StatusServiceUnavailable, reason, fmt.Errorf(reason)
	}

	runtimeClient := newRuntimeClient(s.httpClient, s.firstTokenTimeout)
	thinkingSettings := s.store.Snapshot().Thinking
	var lastStatus int
	var lastMessage string

	for _, candidate := range upstreamCandidates {
		accountLabel := config.AccountLabel(candidate)
		s.logProxyEvent("info", "request.attempt", requestID, logger.String("route", strings.TrimSpace(req.RouteFamily)), logger.String("account", accountLabel), logger.String("model", strings.TrimSpace(req.Model)))
		account, err := s.auth.EnsureFreshAccount(candidate.ID)
		if err != nil {
			decision := classifyHTTPFailure(http.StatusUnauthorized, []byte(err.Error()))
			s.applyFailureDecision(requestID, candidate.ID, accountLabel, decision)
			lastStatus = decision.Status
			lastMessage = decision.Message
			continue
		}
		accountLabel = config.AccountLabel(account)
		refreshedAfterFailure := false

		for {
			payload, err := buildRequestPayload(req, account, thinkingSettings)
			if err != nil {
				s.recordRequestFailure()
				return provider.CompletionOutcome{}, http.StatusBadRequest, err.Error(), err
			}
			body, err := json.Marshal(payload)
			if err != nil {
				s.recordRequestFailure()
				return provider.CompletionOutcome{}, http.StatusBadRequest, err.Error(), err
			}

			resp, _, err := runtimeClient.Do(ctx, account, body)
			if err != nil {
				decision := classifyTransportFailure(err)
				s.applyFailureDecision(requestID, account.ID, accountLabel, decision)
				lastStatus = decision.Status
				lastMessage = decision.Message
				break
			}

			if resp.StatusCode < 200 || resp.StatusCode >= 300 {
				data, _ := io.ReadAll(resp.Body)
				_ = resp.Body.Close()
				decision := classifyHTTPFailure(resp.StatusCode, data)
				if decision.Class == provider.FailureAuthRefreshable && !refreshedAfterFailure {
					refreshedAccount, refreshErr := s.auth.RefreshAccount(account.ID)
					if refreshErr == nil {
						account = refreshedAccount
						accountLabel = config.AccountLabel(account)
						refreshedAfterFailure = true
						s.logAuthEvent("info", "auth.token_refreshed_retry", requestID, logger.String("account", accountLabel))
						continue
					}
					decision = classifyHTTPFailure(http.StatusUnauthorized, []byte(refreshErr.Error()))
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

			outcome, err := collectCompletionWithTagsAndCallback(resp.Body, req, thinkingSettings.FallbackTags, eventCallback)
			_ = resp.Body.Close()
			if err != nil {
				decision := classifyTransportFailure(err)
				s.applyFailureDecision(requestID, account.ID, accountLabel, decision)
				lastStatus = decision.Status
				lastMessage = decision.Message
				break
			}
			outcome.Provider = "kiro"
			outcome.AccountID = account.ID
			outcome.AccountLabel = accountLabel

			s.markSuccess(requestID, account.ID, accountLabel, outcome.Usage)
			return outcome, 0, "", nil
		}
	}

	snapshot := s.pool.AvailabilitySnapshot("kiro")
	if snapshot.ReadyCount == 0 {
		lastStatus = http.StatusServiceUnavailable
		lastMessage = s.pool.ProviderUnavailableReason("kiro")
	}
	if lastStatus == 0 {
		lastStatus = http.StatusServiceUnavailable
	}
	if strings.TrimSpace(lastMessage) == "" {
		lastMessage = "all kiro accounts failed"
	}
	s.recordRequestFailure()
	s.logProxyEvent("warn", "request.failed", requestID, logger.String("route", strings.TrimSpace(req.RouteFamily)), logger.String("reason", lastMessage))
	return provider.CompletionOutcome{}, lastStatus, lastMessage, fmt.Errorf(lastMessage)
}

func (s *Service) markSuccess(requestID string, accountID string, accountLabel string, usage config.ProxyStats) {
	now := time.Now().Unix()
	_ = s.store.UpdateAccount(accountID, func(account *config.Account) {
		account.RequestCount++
		account.PromptTokens += usage.PromptTokens
		account.CompletionTokens += usage.CompletionTokens
		account.TotalTokens += usage.TotalTokens
		account.LastUsed = now
		account.CooldownUntil = 0
		account.ConsecutiveFailures = 0
		account.Banned = false
		account.BannedReason = ""
		account.HealthState = config.AccountHealthReady
		account.HealthReason = ""
		account.LastError = ""
		if account.Quota.Status == "exhausted" || account.Quota.Status == "unknown" || account.Quota.Status == "degraded" {
			account.Quota.Status = "healthy"
			account.Quota.Summary = "Recent request succeeded."
			account.Quota.Source = util.FirstNonEmpty(account.Quota.Source, "runtime")
			account.Quota.Error = ""
			account.Quota.LastCheckedAt = now
			for index := range account.Quota.Buckets {
				if account.Quota.Buckets[index].Status == "exhausted" || account.Quota.Buckets[index].Status == "unknown" {
					account.Quota.Buckets[index].Status = "healthy"
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
	appliedCooldown := time.Duration(0)
	appliedFailures := 0
	_ = s.store.UpdateAccount(accountID, func(account *config.Account) {
		account.ErrorCount++
		account.LastError = err.Error()
		account.LastFailureAt = now
		account.Quota.Status = util.FirstNonEmpty(account.Quota.Status, "degraded")
		account.Quota.Summary = err.Error()
		account.Quota.Source = util.FirstNonEmpty(account.Quota.Source, "runtime")
		account.Quota.Error = err.Error()
		account.Quota.LastCheckedAt = now
		nextFailures := account.ConsecutiveFailures + 1
		appliedCooldown = provider.TransientCooldown(nextFailures)
		appliedFailures = nextFailures
		account.ConsecutiveFailures = nextFailures
		account.CooldownUntil = now + int64(appliedCooldown/time.Second)
		account.HealthState = config.AccountHealthCooldownTransient
		account.HealthReason = err.Error()
	})
	if appliedCooldown > 0 {
		s.logProxyEvent("warn", "request.attempt_failed", requestID, logger.String("account", accountLabel), logger.String("reason", err.Error()), logger.Int("failure_count", appliedFailures), logger.Int("cooldown_seconds", int(appliedCooldown/time.Second)))
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
	case provider.FailureQuotaCooldown:
		cooldownUntil := time.Now().Add(decision.Cooldown).Unix()
		_ = s.store.UpdateAccount(accountID, func(account *config.Account) {
			account.ErrorCount++
			account.CooldownUntil = cooldownUntil
			account.HealthState = config.AccountHealthCooldownQuota
			account.HealthReason = decision.Message
			account.LastFailureAt = time.Now().Unix()
			account.LastError = decision.Message
			account.Quota = config.QuotaInfo{
				Status:        "exhausted",
				Summary:       decision.Message,
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
	eventFields := append([]logger.Field{logger.String("request_id", requestID), logger.String("provider", "kiro")}, fields...)
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

func newRuntimeClient(httpClient *http.Client, firstTokenTimeout time.Duration) *runtimeClient {
	client := httpClient
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Minute}
	}
	if firstTokenTimeout <= 0 {
		firstTokenTimeout = kiroFirstTokenTimeout
	}
	return &runtimeClient{httpClient: client, firstTokenTimeout: firstTokenTimeout}
}

func (c *runtimeClient) Do(ctx context.Context, account config.Account, body []byte) (*http.Response, endpointConfig, error) {
	var lastErr error
	for attempt := 1; attempt <= kiroFirstTokenRetries; attempt++ {
		timeout := time.Duration(attempt) * c.firstTokenTimeout
		if timeout > kiroMaxTimeout {
			timeout = kiroMaxTimeout
		}

		for _, endpoint := range endpointConfigs {
			updatedBody := updateOriginInPayload(body, endpoint)

			resp, err := c.doOnceWithTimeout(ctx, account, updatedBody, endpoint, attempt, timeout)
			if err == nil {
				return resp, endpoint, nil
			}
			lastErr = err
			if err != ErrFirstTokenTimeout {
				return nil, endpoint, err
			}
		}
	}
	return nil, endpointConfig{}, lastErr
}

func updateOriginInPayload(body []byte, endpoint endpointConfig) []byte {
	origin := "CLI"
	if endpoint.Name == "codewhisperer" {
		origin = "AI_EDITOR"
	}

	oldOrigin := "AI_EDITOR"
	newOrigin := origin
	if origin == "AI_EDITOR" {
		oldOrigin = "CLI"
	}

	return bytes.ReplaceAll(body, []byte(`"origin":"`+oldOrigin+`"`), []byte(`"origin":"`+newOrigin+`"`))
}

func (c *runtimeClient) doOnce(ctx context.Context, account config.Account, body []byte, endpoint endpointConfig, attempt int) (*http.Response, error) {
	return c.doOnceWithTimeout(ctx, account, body, endpoint, attempt, c.firstTokenTimeout)
}

func (c *runtimeClient) doOnceWithTimeout(ctx context.Context, account config.Account, body []byte, endpoint endpointConfig, attempt int, timeout time.Duration) (*http.Response, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.URL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	applyRuntimeHeaders(httpReq, account, endpoint, attempt)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return resp, nil
	}

	wrappedBody, err := waitForFirstToken(resp.Body, timeout)
	if err != nil {
		_ = resp.Body.Close()
		return nil, err
	}
	resp.Body = wrappedBody
	return resp, nil
}

func waitForFirstToken(body io.ReadCloser, timeout time.Duration) (io.ReadCloser, error) {
	if timeout <= 0 {
		return body, nil
	}

	type readResult struct {
		data []byte
		err  error
	}

	buf := make([]byte, 4096)
	resultCh := make(chan readResult, 1)
	go func() {
		n, err := body.Read(buf)
		resultCh <- readResult{data: append([]byte(nil), buf[:n]...), err: err}
	}()

	select {
	case result := <-resultCh:
		if len(result.data) > 0 {
			reader := io.MultiReader(bytes.NewReader(result.data), body)
			return readCloser{Reader: reader, Closer: body}, nil
		}
		if result.err == nil {
			return body, nil
		}
		return nil, result.err
	case <-time.After(timeout):
		_ = body.Close()
		return nil, ErrFirstTokenTimeout
	}
}

type readCloser struct {
	io.Reader
	io.Closer
}

func applyRuntimeHeaders(req *http.Request, account config.Account, endpoint endpointConfig, attempt int) {
	req.Header.Set("Content-Type", kiroContentType)
	req.Header.Set("Accept", kiroAcceptStream)
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(account.AccessToken))

	if isIDCAuth(account) {
		req.Header.Set("User-Agent", "aws-sdk-rust/1.3.9 os/macos lang/rust/1.87.0")
		req.Header.Set("x-amz-user-agent", "aws-sdk-rust/1.3.9 ua/2.1 api/ssooidc/1.88.0 os/macos lang/rust/1.87.0 m/E app/AmazonQ-For-CLI")
		req.Header.Set("x-amzn-kiro-agent-mode", "vibe")
	} else {
		machineID := generateMachineID()
		req.Header.Set("User-Agent", fmt.Sprintf("aws-sdk-js/1.0.27 ua/2.1 os/linux lang/js md/nodejs#22.21.1 api/codewhispererstreaming#1.0.27 m/E KiroIDE-0.10.32-%s", machineID))
		req.Header.Set("x-amz-user-agent", "aws-sdk-js/1.0.27 KiroIDE 0.10.32")
		req.Header.Set("x-amzn-kiro-agent-mode", "spec")
	}

	req.Header.Set("x-amzn-codewhisperer-optout", "true")
	req.Header.Set("amz-sdk-invocation-id", uuid.NewString())
	req.Header.Set("amz-sdk-request", "attempt="+strconv.Itoa(maxInt(attempt, 1))+"; max="+strconv.Itoa(kiroFirstTokenRetries*len(endpointConfigs)))

	if endpoint.AmzTarget != "" {
		req.Header.Set("X-Amz-Target", endpoint.AmzTarget)
	}
}

func isIDCAuth(account config.Account) bool {
	if strings.TrimSpace(account.ClientID) != "" && strings.TrimSpace(account.ClientSecret) != "" {
		return true
	}
	return strings.ToLower(strings.TrimSpace(account.AuthMethod)) == "idc"
}

func generateMachineID() string {
	return strings.ReplaceAll(uuid.NewString(), "-", "")
}

func classifyHTTPFailure(statusCode int, body []byte) provider.FailureDecision {
	message := upstreamErrorMessage(statusCode, body)
	trimmed := strings.TrimSpace(message)
	decision := provider.ClassifyHTTPFailure(statusCode, trimmed)
	if decision.Class == provider.FailureDurableDisabled || decision.Class == provider.FailureAuthRefreshable {
		return decision
	}

	lowerMessage := strings.ToLower(trimmed)
	if statusCode == http.StatusTooManyRequests || strings.Contains(lowerMessage, "usage limit") || strings.Contains(lowerMessage, "quota") || strings.Contains(lowerMessage, "credit") || strings.Contains(lowerMessage, "rate limit") {
		return provider.FailureDecision{Class: provider.FailureQuotaCooldown, Message: trimmed, Cooldown: time.Hour, Status: http.StatusTooManyRequests}
	}

	if decision.Class == provider.FailureRequestShape || statusCode == http.StatusBadRequest || statusCode == http.StatusUnprocessableEntity || strings.Contains(lowerMessage, "improperly formed request") || strings.Contains(lowerMessage, "validationexception") || strings.Contains(lowerMessage, "validation exception") || strings.Contains(lowerMessage, "invalid schema") || strings.Contains(lowerMessage, "invalid tool") || strings.Contains(lowerMessage, "malformed") {
		return provider.FailureDecision{Class: provider.FailureRequestShape, Message: trimmed, Status: http.StatusBadRequest}
	}

	return decision
}

func classifyTransportFailure(err error) provider.FailureDecision {
	if errors.Is(err, ErrFirstTokenTimeout) {
		return provider.FailureDecision{Class: provider.FailureRetryableTransport, Message: ErrFirstTokenTimeout.Error(), Cooldown: 5 * time.Second, RetryAllowed: true, Status: http.StatusGatewayTimeout}
	}
	decision := provider.ClassifyTransportFailure(err)
	if err != nil {
		message := strings.TrimSpace(err.Error())
		if strings.Contains(strings.ToLower(message), "timeout") || errors.Is(err, context.DeadlineExceeded) {
			decision.Status = http.StatusGatewayTimeout
		}
	}
	return decision
}

func upstreamErrorMessage(statusCode int, body []byte) string {
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		return http.StatusText(statusCode)
	}
	for _, key := range []string{"message", "Message", "errorMessage"} {
		var object map[string]any
		if err := json.Unmarshal(body, &object); err == nil {
			if value, ok := object[key].(string); ok && strings.TrimSpace(value) != "" {
				return strings.TrimSpace(value)
			}
			if nested, ok := object["error"].(map[string]any); ok {
				if value, ok := nested[key].(string); ok && strings.TrimSpace(value) != "" {
					return strings.TrimSpace(value)
				}
			}
		}
	}
	return trimmed
}


func maxInt(left int, right int) int {
	if left > right {
		return left
	}
	return right
}

func (s *Service) ExecuteFromIR(ctx context.Context, request contract.Request) (provider.CompletionOutcome, int, string, error) {
	return s.Complete(ctx, provider.RequestFromIR(request))
}
