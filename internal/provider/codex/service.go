package codex

import (
	"context"
	_ "embed"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"cliro/internal/account"
	"cliro/internal/config"
	models "cliro/internal/proxy/models"
	"cliro/internal/logger"
	"cliro/internal/provider"
)

//go:embed default_instructions.md
var embeddedDefaultInstructions string

const (
	codexBaseURL          = "https://chatgpt.com/backend-api/codex"
	codexVersion          = "0.118.0"
	quotaCooldown         = time.Hour
	defaultRequestTimeout = 5 * time.Minute
	codexUserAgent        = "codex-tui/0.118.0 (Mac OS 26.3.1; arm64) iTerm.app/3.6.9 (codex-tui; 0.118.0)"
)

// O15: reuse scanner read buffers across requests to avoid per-request 64 KiB heap allocation.
var scannerBufPool = sync.Pool{
	New: func() any {
		buf := make([]byte, 0, 64*1024)
		return &buf
	},
}

type Service struct {
	store      *config.Manager
	auth       accountAuth
	pool       *account.Pool
	log        *logger.Logger
	httpClient *http.Client
	retryPlan  RetryPlanner
	recovery   *AuthRecoveryCoordinator
}

type accountAuth interface {
	EnsureFreshAccount(accountID string) (config.Account, error)
	RefreshAccount(accountID string) (config.Account, error)
}

type contextKey string

const requestIDContextKey contextKey = "gateway_request_id"

func requestIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	value, _ := ctx.Value(requestIDContextKey).(string)
	return strings.TrimSpace(value)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func defaultCodexInstructions() string {
	return strings.TrimSpace(embeddedDefaultInstructions)
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
		retryPlan:  NewRetryPlanner(accountPool, "codex", nil),
		recovery:   NewAuthRecoveryCoordinator(func(accountID string) (config.Account, error) { return authManager.RefreshAccount(accountID) }, 2),
	}
}

func newHTTPClient(timeout time.Duration) *http.Client {
	if timeout <= 0 {
		timeout = defaultRequestTimeout
	}
	return &http.Client{Timeout: timeout}
}

func (s *Service) ExecuteFromIR(ctx context.Context, request models.Request) (CompletionOutcome, int, string, error) {
	return s.Complete(ctx, RequestFromIR(request))
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
	s.logProxyEvent("info", "request.success", requestID, logger.F("account", accountLabel), logger.F("prompt_tokens", usage.PromptTokens), logger.F("completion_tokens", usage.CompletionTokens), logger.F("total_tokens", usage.TotalTokens))
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
		a.Quota.Status = firstNonEmpty(a.Quota.Status, "degraded")
		a.Quota.Summary = "Request failed"
		a.Quota.Source = firstNonEmpty(a.Quota.Source, "runtime")
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
		s.logProxyEvent("warn", "request.attempt_failed", requestID, logger.F("account", accountLabel), logger.F("reason", detail), logger.F("failure_count", appliedFailures), logger.F("cooldown_seconds", int(appliedCooldown/time.Second)))
	}
}

func (s *Service) markBanned(requestID string, accountID string, accountLabel string, reason string) {
	_ = s.store.MarkAccountBanned(accountID, reason)
	s.logAuthEvent("warn", "account.banned", requestID, logger.F("account", accountLabel), logger.F("reason", reason))
}

func (s *Service) applyFailureDecision(requestID string, accountID string, accountLabel string, decision provider.FailureDecision) {
	switch decision.Class {
	case provider.FailureRequestShape:
		s.logProxyEvent("warn", "request.shape_invalid", requestID, logger.F("account", accountLabel), logger.F("reason", decision.Message))
	case provider.FailureDurableDisabled:
		if decision.BanAccount {
			s.markBanned(requestID, accountID, accountLabel, decision.Message)
			return
		}
		_ = s.store.MarkAccountDurablyDisabled(accountID, decision.Message)
		s.logAuthEvent("warn", "account.durable_disabled", requestID, logger.F("account", accountLabel), logger.F("reason", decision.Message))
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
		s.logAuthEvent("warn", "auth.relogin_required", requestID, logger.F("account", accountLabel), logger.F("reason", decision.Message))
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
		s.logQuotaEvent("warn", "quota.cooldown", requestID, logger.F("account", accountLabel), logger.F("reason", decision.Message), logger.F("cooldown_until", cooldownUntil))
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
	eventFields := append([]logger.Field{logger.F("request_id", requestID), logger.F("provider", "codex")}, fields...)
	switch level {
	case "warn":
		s.log.Warn(scope, event, eventFields...)
	case "error":
		s.log.Error(scope, event, eventFields...)
	case "debug":
		s.log.Debug(scope, event, eventFields...)
	default:
		s.log.Info(scope, event, eventFields...)
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
