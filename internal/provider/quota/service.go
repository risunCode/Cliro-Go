package quota

import (
	"cliro-go/internal/util"
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"cliro-go/internal/auth"
	"cliro-go/internal/config"
	"cliro-go/internal/logger"
	coreprovider "cliro-go/internal/provider"
	codexprovider "cliro-go/internal/provider/codex"
	kiroprovider "cliro-go/internal/provider/kiro"
)

const fetchTimeout = 25 * time.Second

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
		authMessage, refreshableAuth := config.RefreshableAuthReason(err.Error())
		if refreshableAuth {
			quota.Status = "unknown"
			quota.Summary = "Authentication required"
			quota.Source = util.FirstNonEmpty(strings.TrimSpace(quota.Source), "runtime")
			quota.Error = util.FirstNonEmpty(strings.TrimSpace(authMessage), strings.TrimSpace(quota.Error), strings.TrimSpace(err.Error()))
		}
		blockedMsg, blocked := blockedAccountMessageFromError(err)
		_ = s.store.UpdateAccount(accountID, func(a *config.Account) {
			a.Quota = normalizeQuotaInfo(quota)
			if refreshableAuth {
				a.HealthState = config.AccountHealthCooldownTransient
				a.HealthReason = "Need re-login"
				a.CooldownUntil = time.Now().Add(30 * time.Second).Unix()
				a.LastFailureAt = time.Now().Unix()
				a.LastError = util.FirstNonEmpty(strings.TrimSpace(authMessage), strings.TrimSpace(err.Error()))
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
			cooldownUntil := config.QuotaResetAt(quota)
			if cooldownUntil <= time.Now().Unix() {
				cooldownUntil = time.Now().Add(time.Hour).Unix()
			}
			a.CooldownUntil = cooldownUntil
			a.HealthState = config.AccountHealthCooldownQuota
			a.HealthReason = util.FirstNonEmpty(strings.TrimSpace(quota.Summary), "Quota exhausted")
			a.LastFailureAt = time.Now().Unix()
			if strings.TrimSpace(a.LastError) == "" {
				a.LastError = util.FirstNonEmpty(strings.TrimSpace(quota.Summary), "Quota exhausted")
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
					failures = append(failures, util.FirstNonEmpty(account.Email, account.ID)+": "+err.Error())
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

func (s *Service) logQuotaRefreshBatch(force bool, total int, eligible int, skipped map[string]int) {
	if s == nil || s.log == nil {
		return
	}
	mode := "smart"
	if force {
		mode = "force"
	}
	fields := []logger.Field{logger.String("mode", mode), logger.Int("total", total), logger.Int("eligible", eligible)}
	if skipped["quota_cooldown"] > 0 {
		fields = append(fields, logger.Int("skipped_quota_cooldown", skipped["quota_cooldown"]))
	}
	if skipped["disabled"] > 0 {
		fields = append(fields, logger.Int("skipped_disabled", skipped["disabled"]))
	}
	if skipped["banned"] > 0 {
		fields = append(fields, logger.Int("skipped_banned", skipped["banned"]))
	}
	s.log.InfoEvent("quota", "batch.refresh", fields...)
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
		message := util.FirstNonEmpty(strings.TrimSpace(quota.Error), strings.TrimSpace(quota.Summary), "Account deactivated")
		return message, true
	}

	sourceMessage := util.FirstNonEmpty(strings.TrimSpace(quota.Error), strings.TrimSpace(quota.Summary))
	if sourceMessage == "" {
		return "", false
	}
	return config.BlockedAccountReason(sourceMessage)
}

func blockedAccountMessageFromError(err error) (string, bool) {
	if err == nil {
		return "", false
	}
	return config.BlockedAccountReason(err.Error())
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
		if resetAt := config.QuotaResetAt(account.Quota); resetAt > now {
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
