package kiro

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"cliro/internal/account"
	accountstate "cliro/internal/account"
	"cliro/internal/config"
	"cliro/internal/logger"
	"cliro/internal/platform"
	"cliro/internal/provider"
	models "cliro/internal/proxy/models"
)

const (
	kiroRequestTimeout     = 5 * time.Minute
	kiroMachineIDHeaderVal = "kiro-desktop"
)

var kiroRuntimeURLs = []string{
	"https://q.us-east-1.amazonaws.com/generateAssistantResponse",
	"https://codewhisperer.us-east-1.amazonaws.com/generateAssistantResponse",
}

type CompletionOutcome struct {
	ID                string
	Model             string
	Text              string
	Thinking          string
	ThinkingSignature string
	ThinkingSource    string
	ToolUses          []ToolUse
	Usage             config.ProxyStats
	Provider          string
	AccountID         string
	AccountLabel      string
}

type ToolUse struct {
	ID    string
	Name  string
	Input map[string]any
}

type accountAuth interface {
	EnsureFreshAccount(accountID string) (config.Account, error)
	RefreshAccount(accountID string) (config.Account, error)
}

type Service struct {
	store      *config.Manager
	auth       accountAuth
	pool       *account.Pool
	log        *logger.Logger
	httpClient *http.Client
	quota      *QuotaFetcher
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func NewService(store *config.Manager, authManager accountAuth, accountPool *account.Pool, log *logger.Logger, httpClient *http.Client) *Service {
	client := httpClient
	if client == nil {
		client = &http.Client{Timeout: kiroRequestTimeout}
	}
	return &Service{
		store:      store,
		auth:       authManager,
		pool:       accountPool,
		log:        log,
		httpClient: client,
		quota:      NewQuotaFetcher(client),
	}
}

func (s *Service) ExecuteFromIR(ctx context.Context, request models.Request) (CompletionOutcome, int, string, error) {
	requestID := platform.RequestIDFromContext(ctx)
	if strings.TrimSpace(request.Model) == "" {
		s.recordRequestFailure()
		return CompletionOutcome{}, http.StatusBadRequest, "model is required", fmt.Errorf("model is required")
	}
	if s.pool.AvailabilitySnapshot("kiro").ReadyCount == 0 {
		s.recordRequestFailure()
		reason := s.pool.ProviderUnavailableReason("kiro")
		return CompletionOutcome{}, http.StatusServiceUnavailable, reason, fmt.Errorf(reason)
	}

	for _, candidate := range s.pool.AvailableAccountsForProvider("kiro") {
		accountLabel := accountstate.Label(candidate)
		account, err := s.auth.EnsureFreshAccount(candidate.ID)
		if err != nil {
			decision := provider.ClassifyHTTPFailure(http.StatusUnauthorized, err.Error())
			s.applyFailureDecision(requestID, candidate.ID, accountLabel, decision)
			continue
		}
		outcome, status, message, err := s.executeWithAccount(ctx, account, request)
		if err == nil {
			outcome.Provider = "kiro"
			outcome.AccountID = account.ID
			outcome.AccountLabel = accountstate.Label(account)
			s.markSuccess(requestID, account.ID, outcome.AccountLabel, outcome.Usage)
			return outcome, 0, "", nil
		}
		decision := provider.ClassifyHTTPFailure(status, message)
		if status == 0 {
			decision = provider.ClassifyTransportFailure(err)
		}
		if status > 0 {
			decision.Status = status
			decision.Message = firstNonEmpty(message, decision.Message)
		}
		s.applyFailureDecision(requestID, account.ID, accountstate.Label(account), decision)
	}

	s.recordRequestFailure()
	reason := s.pool.ProviderUnavailableReason("kiro")
	if strings.TrimSpace(reason) == "" {
		reason = "all kiro accounts failed"
	}
	return CompletionOutcome{}, http.StatusServiceUnavailable, reason, fmt.Errorf(reason)
}

func (s *Service) recordRequestFailure() {
	now := time.Now().Unix()
	_ = s.store.UpdateStats(func(stats *config.ProxyStats) {
		stats.TotalRequests++
		stats.FailedRequests++
		stats.LastRequestAt = now
	})
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
	})
	_ = s.store.UpdateStats(func(stats *config.ProxyStats) {
		stats.TotalRequests++
		stats.SuccessRequests++
		stats.PromptTokens += usage.PromptTokens
		stats.CompletionTokens += usage.CompletionTokens
		stats.TotalTokens += usage.TotalTokens
		stats.LastRequestAt = now
	})
	s.log.Info("proxy", "request.success", logger.F("request_id", requestID), logger.F("provider", "kiro"), logger.F("account", accountLabel))
}

func (s *Service) applyFailureDecision(requestID string, accountID string, accountLabel string, decision provider.FailureDecision) {
	switch decision.Class {
	case provider.FailureDurableDisabled:
		if decision.BanAccount {
			_ = s.store.MarkAccountBanned(accountID, decision.Message)
		} else {
			_ = s.store.MarkAccountDurablyDisabled(accountID, decision.Message)
		}
	case provider.FailureAuthRefreshable:
		_ = s.store.MarkAccountReloginRequired(accountID, decision.Message)
	case provider.FailureQuotaCooldown:
		cooldownUntil := time.Now().Add(decision.Cooldown).Unix()
		_ = s.store.MarkAccountQuotaCooldown(accountID, decision.Message, cooldownUntil)
	default:
		_ = s.store.MarkAccountTransientCooldown(accountID, decision.Message, time.Now().Add(provider.TransientCooldown(1)).Unix())
	}
	s.log.Warn("proxy", "request.failed", logger.F("request_id", requestID), logger.F("provider", "kiro"), logger.F("account", accountLabel), logger.F("reason", decision.Message))
}
