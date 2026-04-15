package quota

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	accountstate "cliro/internal/account"
	"cliro/internal/auth"
	sharedauth "cliro/internal/auth/shared"
	"cliro/internal/config"
	"cliro/internal/logger"
	coreprovider "cliro/internal/provider"
	codexprovider "cliro/internal/provider/codex"
	kiroprovider "cliro/internal/provider/kiro"
)

const (
	fetchTimeout = 25 * time.Second

	// autoRefreshInterval is how often the quota auto-refresh loop wakes up.
	autoRefreshInterval = 5 * time.Minute

	// autoRefreshConcurrency caps concurrent quota refreshes in the auto loop.
	autoRefreshConcurrency = 4
)

type Service struct {
	store        *config.Manager
	auth         *auth.Manager
	log          *logger.Logger
	codexFetcher *codexprovider.QuotaFetcher
	kiroFetcher  *kiroprovider.QuotaFetcher
}

func NewService(store *config.Manager, authManager *auth.Manager, log *logger.Logger, httpClient *http.Client) *Service {
	client := httpClient
	if client == nil {
		client = &http.Client{Timeout: fetchTimeout}
	}
	return &Service{
		store:        store,
		auth:         authManager,
		log:          log,
		codexFetcher: codexprovider.NewQuotaFetcher(client),
		kiroFetcher:  kiroprovider.NewQuotaFetcher(client),
	}
}

func (s *Service) RefreshAccountWithQuota(accountID string) (config.Account, error) {
	account, ok := s.store.GetAccount(accountID)
	if !ok {
		return config.Account{}, fmt.Errorf("account not found")
	}
	if err := validateQuotaProvider(account); err != nil {
		return account, err
	}

	refreshed, err := s.auth.RefreshAccount(accountID)
	if err != nil {
		if updated, ok := s.store.GetAccount(accountID); ok {
			return updated, err
		}
		return refreshed, err
	}
	account = refreshed

	ctx, cancel := context.WithTimeout(context.Background(), fetchTimeout)
	defer cancel()

	quota, resolvedEmail, quotaErr := s.fetchQuotaForAccount(ctx, account)
	if err := s.applyQuotaSnapshot(accountID, quota, resolvedEmail); err != nil {
		return account, err
	}

	updated, _ := s.store.GetAccount(accountID)
	return updated, quotaErr
}

func (s *Service) RefreshQuota(accountID string) (config.Account, error) {
	account, ok := s.store.GetAccount(accountID)
	if !ok {
		return config.Account{}, fmt.Errorf("account not found")
	}
	if err := validateQuotaProvider(account); err != nil {
		return account, err
	}

	fresh, err := s.auth.EnsureFreshAccount(accountID)
	if err != nil {
		quota := coreprovider.SynthesizeQuota(account, err)
		authMessage, refreshableAuth := sharedauth.RefreshableAuthReason(err.Error())
		if refreshableAuth {
			quota.Status = "unknown"
			quota.Summary = "Authentication required"
			quota.Source = firstNonEmpty(strings.TrimSpace(quota.Source), "runtime")
			quota.Error = firstNonEmpty(strings.TrimSpace(authMessage), strings.TrimSpace(quota.Error), strings.TrimSpace(err.Error()))
		}
		blockedMsg, blocked := blockedAccountMessageFromError(err)
		_ = s.store.UpdateAccount(accountID, func(a *config.Account) {
			a.Quota = normalizeQuotaInfo(quota)
			if refreshableAuth {
				a.HealthState = config.AccountHealthCooldownTransient
				a.HealthReason = "Need re-login"
				a.CooldownUntil = time.Now().Add(30 * time.Second).Unix()
				a.LastFailureAt = time.Now().Unix()
				a.LastError = firstNonEmpty(strings.TrimSpace(authMessage), strings.TrimSpace(err.Error()))
			}
			if blocked {
				a.Enabled = false
				a.Banned = true
				a.BannedReason = blockedMsg
				a.LastError = blockedMsg
			}
		})
		return account, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), fetchTimeout)
	defer cancel()

	quota, resolvedEmail, quotaErr := s.fetchQuotaForAccount(ctx, fresh)
	if err := s.applyQuotaSnapshot(accountID, quota, resolvedEmail); err != nil {
		return fresh, err
	}
	updated, _ := s.store.GetAccount(accountID)
	return updated, quotaErr
}

func (s *Service) RefreshQuotaOnly(accountID string) error {
	_, err := s.RefreshQuota(accountID)
	return err
}

func (s *Service) RefreshAllQuotas() error {
	return s.refreshAllQuotas(false)
}

func (s *Service) ForceRefreshAllQuotas() error {
	return s.refreshAllQuotas(true)
}

func (s *Service) applyQuotaSnapshot(accountID string, quota config.QuotaInfo, resolvedEmail string) error {
	quota = normalizeQuotaInfo(quota)
	return s.store.UpdateAccount(accountID, func(a *config.Account) {
		a.Quota = quota
		if strings.TrimSpace(resolvedEmail) != "" {
			a.Email = strings.TrimSpace(resolvedEmail)
		}
		if blockedMsg, blocked := blockedAccountMessageFromQuota(quota); blocked {
			a.Enabled = false
			a.Banned = true
			a.BannedReason = blockedMsg
			a.HealthState = config.AccountHealthBanned
			a.HealthReason = blockedMsg
			a.LastError = blockedMsg
			return
		}
		if shouldApplyQuotaCooldown(quota) {
			if !a.Enabled && a.HealthState == config.AccountHealthDisabledDurable {
				return
			}
			cooldownUntil := accountstate.QuotaResetAt(quota)
			if cooldownUntil <= time.Now().Unix() {
				cooldownUntil = time.Now().Add(time.Hour).Unix()
			}
			a.CooldownUntil = cooldownUntil
			a.HealthState = config.AccountHealthCooldownQuota
			a.HealthReason = firstNonEmpty(strings.TrimSpace(quota.Summary), "Quota exhausted")
			a.LastFailureAt = time.Now().Unix()
			if strings.TrimSpace(a.LastError) == "" {
				a.LastError = firstNonEmpty(strings.TrimSpace(quota.Summary), "Quota exhausted")
			}
		} else if a.HealthState == config.AccountHealthCooldownQuota {
			a.HealthState = config.AccountHealthReady
			a.HealthReason = ""
			a.CooldownUntil = 0
			a.ConsecutiveFailures = 0
		}
	})
}

func (s *Service) fetchQuotaForAccount(ctx context.Context, account config.Account) (config.QuotaInfo, string, error) {
	if isKiroAccount(account) {
		return s.kiroFetcher.FetchQuota(ctx, account, func(accountID string) (config.Account, error) {
			return s.auth.RefreshAccount(accountID)
		})
	}
	if !isCodexAccount(account) {
		return config.QuotaInfo{}, "", fmt.Errorf("unsupported provider for quota refresh: %s", strings.TrimSpace(account.Provider))
	}
	quota, err := s.codexFetcher.FetchQuota(ctx, account)
	return quota, "", err
}

func (s *Service) refreshAllQuotas(force bool) error {
	accounts := s.store.Accounts()
	if len(accounts) == 0 {
		return nil
	}

	now := time.Now().Unix()
	eligible := make([]config.Account, 0, len(accounts))
	skipped := map[string]int{}
	for _, account := range accounts {
		if !force {
			if skip, reason := shouldSkipBatchQuotaRefresh(account, now); skip {
				skipped[reason]++
				continue
			}
		}
		eligible = append(eligible, account)
	}

	if len(eligible) == 0 {
		s.logQuotaRefreshBatch(force, len(accounts), 0, skipped)
		return nil
	}

	workerCount := 4
	if workerCount > len(eligible) {
		workerCount = len(eligible)
	}
	if workerCount <= 0 {
		workerCount = 1
	}

	jobs := make(chan config.Account)
	failures := make([]string, 0)
	var failuresMu sync.Mutex
	var wg sync.WaitGroup

	for worker := 0; worker < workerCount; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for account := range jobs {
				if _, err := s.RefreshQuota(account.ID); err != nil {
					failuresMu.Lock()
					failures = append(failures, firstNonEmpty(account.Email, account.ID)+": "+err.Error())
					failuresMu.Unlock()
				}
			}
		}()
	}

	for _, account := range eligible {
		jobs <- account
	}
	close(jobs)
	wg.Wait()

	s.logQuotaRefreshBatch(force, len(accounts), len(eligible), skipped)

	if len(failures) > 0 {
		return fmt.Errorf(strings.Join(failures, "; "))
	}
	return nil
}

// StartAutoQuotaRefreshLoop starts a background loop that wakes every
// autoRefreshInterval and refreshes Codex accounts whose quota is
// exhausted and whose reset timestamp has already passed, or whose
// quota message indicates "windows reached their limit".
// It returns immediately; the loop runs until ctx is cancelled.
func (s *Service) StartAutoQuotaRefreshLoop(ctx context.Context) {
	go func() {
		s.runAutoQuotaRefreshOnce()
		ticker := time.NewTicker(autoRefreshInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.runAutoQuotaRefreshOnce()
			}
		}
	}()
}

func (s *Service) runAutoQuotaRefreshOnce() {
	accounts := s.store.Accounts()
	if len(accounts) == 0 {
		return
	}

	now := time.Now().Unix()
	var candidates []config.Account
	for _, acc := range accounts {
		if !shouldAutoRefreshQuota(acc, now) {
			continue
		}
		candidates = append(candidates, acc)
	}
	if len(candidates) == 0 {
		return
	}

	s.log.Info("quota", "auto_refresh.start", logger.F("candidates", len(candidates)))

	sem := make(chan struct{}, autoRefreshConcurrency)
	var wg sync.WaitGroup
	for _, acc := range candidates {
		acc := acc
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			label := accountstate.Label(acc)
			if _, err := s.RefreshQuota(acc.ID); err != nil {
				s.log.Warn("quota", "auto_refresh.failed",
					logger.F("account", label),
					logger.F("error", err.Error()))
			} else {
				s.log.Info("quota", "auto_refresh.ok",
					logger.F("account", label))
			}
		}()
	}
	wg.Wait()
}

// shouldAutoRefreshQuota returns true when an account needs a background
// quota refresh: it must be an enabled Codex account whose quota is
// exhausted but the reset window has already passed, or whose quota
// message contains "windows reached their limit".
func shouldAutoRefreshQuota(acc config.Account, now int64) bool {
	if !isCodexAccount(acc) {
		return false
	}
	if !acc.Enabled || acc.Banned {
		return false
	}
	if acc.HealthState == config.AccountHealthDisabledDurable || acc.HealthState == config.AccountHealthBanned {
		return false
	}

	status := strings.ToLower(strings.TrimSpace(acc.Quota.Status))
	isExhausted := status == "exhausted" || status == "empty"
	windowsLimit := isWindowsLimitMessage(acc)

	if !isExhausted && !windowsLimit {
		return false
	}

	// For exhausted quota: only refresh once the reset timestamp has passed.
	// For the windows-limit signal: refresh immediately.
	if isExhausted && !windowsLimit {
		resetAt := accountstate.QuotaResetAt(acc.Quota)
		if resetAt <= 0 || resetAt > now {
			return false
		}
	}

	return true
}

func isWindowsLimitMessage(acc config.Account) bool {
	needle := "windows reached their limit"
	if strings.Contains(strings.ToLower(acc.Quota.Summary), needle) {
		return true
	}
	if strings.Contains(strings.ToLower(acc.Quota.Error), needle) {
		return true
	}
	if strings.Contains(strings.ToLower(acc.LastError), needle) {
		return true
	}
	return false
}

func (s *Service) logQuotaRefreshBatch(force bool, total int, eligible int, skipped map[string]int) {
	if s == nil || s.log == nil {
		return
	}
	mode := "smart"
	if force {
		mode = "force"
	}
	fields := []logger.Field{logger.F("mode", mode), logger.F("total", total), logger.F("eligible", eligible)}
	if skipped["quota_cooldown"] > 0 {
		fields = append(fields, logger.F("skipped_quota_cooldown", skipped["quota_cooldown"]))
	}
	if skipped["disabled"] > 0 {
		fields = append(fields, logger.F("skipped_disabled", skipped["disabled"]))
	}
	if skipped["banned"] > 0 {
		fields = append(fields, logger.F("skipped_banned", skipped["banned"]))
	}
	s.log.Info("quota", "batch.refresh", fields...)
}

func isKiroAccount(account config.Account) bool {
	return strings.EqualFold(strings.TrimSpace(account.Provider), "kiro")
}

func isCodexAccount(account config.Account) bool {
	return strings.EqualFold(strings.TrimSpace(account.Provider), "codex")
}

func validateQuotaProvider(account config.Account) error {
	if isKiroAccount(account) || isCodexAccount(account) {
		return nil
	}
	provider := strings.TrimSpace(account.Provider)
	if provider == "" {
		return fmt.Errorf("account provider is required")
	}
	return fmt.Errorf("unsupported provider for quota refresh: %s", provider)
}

func blockedAccountMessageFromQuota(quota config.QuotaInfo) (string, bool) {
	status := strings.ToLower(strings.TrimSpace(quota.Status))
	switch status {
	case "deactivated", "banned", "suspended", "disabled", "account_deactivated":
		message := firstNonEmpty(strings.TrimSpace(quota.Error), strings.TrimSpace(quota.Summary), "Account deactivated")
		return message, true
	}

	sourceMessage := firstNonEmpty(strings.TrimSpace(quota.Error), strings.TrimSpace(quota.Summary))
	if sourceMessage == "" {
		return "", false
	}
	return sharedauth.BlockedAccountReason(sourceMessage)
}

func blockedAccountMessageFromError(err error) (string, bool) {
	if err == nil {
		return "", false
	}
	return sharedauth.BlockedAccountReason(err.Error())
}

func shouldApplyQuotaCooldown(quota config.QuotaInfo) bool {
	status := strings.ToLower(strings.TrimSpace(quota.Status))
	if status == "exhausted" || status == "empty" {
		return true
	}
	for _, bucket := range quota.Buckets {
		bucketStatus := strings.ToLower(strings.TrimSpace(bucket.Status))
		if bucketStatus == "exhausted" || bucketStatus == "empty" {
			return true
		}
		if bucket.Total > 0 {
			remaining := bucket.Remaining
			if remaining == 0 && bucket.Used > 0 && bucket.Used <= bucket.Total {
				remaining = maxInt(bucket.Total-bucket.Used, 0)
			}
			if remaining <= 0 {
				return true
			}
		}
	}
	return false
}

func shouldSkipBatchQuotaRefresh(account config.Account, now int64) (bool, string) {
	if account.Banned || account.HealthState == config.AccountHealthBanned {
		return true, "banned"
	}
	if !account.Enabled || account.HealthState == config.AccountHealthDisabledDurable {
		return true, "disabled"
	}
	if shouldApplyQuotaCooldown(account.Quota) {
		if resetAt := accountstate.QuotaResetAt(account.Quota); resetAt > now {
			return true, "quota_cooldown"
		}
	}
	return false, ""
}

func normalizeQuotaInfo(quota config.QuotaInfo) config.QuotaInfo {
	quota.Summary = strings.TrimSpace(quota.Summary)
	quota.Error = strings.TrimSpace(quota.Error)

	if quota.Summary == "" && quota.Error != "" {
		quota.Summary = quota.Error
	}

	if quota.Summary != "" && strings.EqualFold(quota.Summary, quota.Error) {
		quota.Error = ""
	}

	return quota
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
