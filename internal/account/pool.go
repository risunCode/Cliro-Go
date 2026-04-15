package account

import (
	"fmt"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"cliro/internal/config"
)

// ValidateProvider returns an error if the provider string is not a supported value.
func ValidateProvider(provider string) error {
	normalized := strings.ToLower(strings.TrimSpace(provider))
	if normalized == "" {
		return fmt.Errorf("account provider is required")
	}
	if normalized != "codex" && normalized != "kiro" {
		return fmt.Errorf("unsupported account provider: %s", provider)
	}
	return nil
}

// ValidateAccount returns an error if the account is missing required fields.
func ValidateAccount(account config.Account) error {
	if strings.TrimSpace(account.ID) == "" {
		return fmt.Errorf("account id is required")
	}
	return ValidateProvider(account.Provider)
}

type Pool struct {
	store   *config.Manager
	current uint64
}

type AvailabilitySnapshot struct {
	Provider               string `json:"provider,omitempty"`
	TotalCount             int    `json:"totalCount"`
	ReadyCount             int    `json:"readyCount"`
	CooldownQuotaCount     int    `json:"cooldownQuotaCount"`
	CooldownTransientCount int    `json:"cooldownTransientCount"`
	DisabledDurableCount   int    `json:"disabledDurableCount"`
	BannedCount            int    `json:"bannedCount"`
}

func NewPool(store *config.Manager) *Pool {
	return &Pool{store: store}
}

func (p *Pool) NextAccount() (config.Account, error) {
	accounts := p.AvailableAccounts()
	if len(accounts) == 0 {
		allAccounts := p.store.Accounts()
		if len(allAccounts) == 0 {
			return config.Account{}, fmt.Errorf("no accounts configured")
		}
		return config.Account{}, fmt.Errorf("all accounts are cooling down or disabled")
	}
	return accounts[0], nil
}

func (p *Pool) AvailableAccounts() []config.Account {
	return p.availableAccountsByProvider("")
}

func (p *Pool) AvailableAccountsForProvider(provider string) []config.Account {
	return p.availableAccountsByProvider(provider)
}

func (p *Pool) availableAccountsByProvider(provider string) []config.Account {
	accounts := p.store.Accounts()
	if len(accounts) == 0 {
		return nil
	}

	targetProvider := strings.TrimSpace(provider)
	if targetProvider != "" {
		targetProvider = strings.ToLower(targetProvider)
	}

	now := time.Now().Unix()
	start := int(atomic.AddUint64(&p.current, 1) % uint64(len(accounts)))
	available := make([]config.Account, 0, len(accounts))

	for i := 0; i < len(accounts); i++ {
		account := accounts[(start+i)%len(accounts)]
		if targetProvider != "" && account.Provider != targetProvider {
			continue
		}
		if accountAvailabilityState(account, now) != config.AccountHealthReady {
			continue
		}
		available = append(available, account)
	}

	switch p.store.SchedulingMode() {
	case config.SchedulingModeCacheFirst:
		sort.SliceStable(available, func(i, j int) bool {
			return cacheFirstLess(available[i], available[j])
		})
	case config.SchedulingModeBalance:
		sort.SliceStable(available, func(i, j int) bool {
			return balanceLess(available[i], available[j])
		})
	}

	return available
}

func (p *Pool) AvailableCount() int {
	return p.AvailabilitySnapshot("").ReadyCount
}

func (p *Pool) AvailabilitySnapshot(provider string) AvailabilitySnapshot {
	accounts := p.store.Accounts()
	targetProvider := strings.ToLower(strings.TrimSpace(provider))
	now := time.Now().Unix()
	snapshot := AvailabilitySnapshot{Provider: targetProvider}

	for _, account := range accounts {
		if targetProvider != "" && account.Provider != targetProvider {
			continue
		}
		snapshot.TotalCount++
		switch accountAvailabilityState(account, now) {
		case config.AccountHealthReady:
			snapshot.ReadyCount++
		case config.AccountHealthCooldownQuota:
			snapshot.CooldownQuotaCount++
		case config.AccountHealthCooldownTransient:
			snapshot.CooldownTransientCount++
		case config.AccountHealthDisabledDurable:
			snapshot.DisabledDurableCount++
		case config.AccountHealthBanned:
			snapshot.BannedCount++
		default:
			snapshot.CooldownTransientCount++
		}
	}

	return snapshot
}

func (p *Pool) ProviderUnavailableReason(provider string) string {
	targetProvider := strings.ToLower(strings.TrimSpace(provider))
	if targetProvider == "" {
		targetProvider = "provider"
	}
	snapshot := p.AvailabilitySnapshot(targetProvider)
	return fmt.Sprintf("no available %s accounts (ready=%d cooldown_quota=%d cooldown_transient=%d durable_disabled=%d banned=%d)", targetProvider, snapshot.ReadyCount, snapshot.CooldownQuotaCount, snapshot.CooldownTransientCount, snapshot.DisabledDurableCount, snapshot.BannedCount)
}

func accountAvailabilityState(acc config.Account, now int64) config.AccountHealthState {
	if acc.Banned || acc.HealthState == config.AccountHealthBanned {
		return config.AccountHealthBanned
	}
	if !acc.Enabled || acc.HealthState == config.AccountHealthDisabledDurable {
		return config.AccountHealthDisabledDurable
	}
	if acc.CooldownUntil > now {
		if acc.HealthState == config.AccountHealthCooldownQuota {
			return config.AccountHealthCooldownQuota
		}
		return config.AccountHealthCooldownTransient
	}
	if QuotaResetAt(acc.Quota) > now && acc.Quota.Status == "exhausted" {
		return config.AccountHealthCooldownQuota
	}
	return config.AccountHealthReady
}

func cacheFirstLess(left config.Account, right config.Account) bool {
	if left.LastUsed != right.LastUsed {
		return left.LastUsed > right.LastUsed
	}
	if left.ErrorCount != right.ErrorCount {
		return left.ErrorCount < right.ErrorCount
	}
	if left.RequestCount != right.RequestCount {
		return left.RequestCount > right.RequestCount
	}
	return false
}

func balanceLess(left config.Account, right config.Account) bool {
	if left.RequestCount != right.RequestCount {
		return left.RequestCount < right.RequestCount
	}
	if left.ErrorCount != right.ErrorCount {
		return left.ErrorCount < right.ErrorCount
	}
	if left.LastUsed != right.LastUsed {
		return left.LastUsed < right.LastUsed
	}
	return false
}
