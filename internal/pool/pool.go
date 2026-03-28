package pool

import (
	"fmt"
	"sync/atomic"
	"time"

	"cliro-go/internal/config"
)

type Pool struct {
	store   *config.Manager
	current uint64
}

func New(store *config.Manager) *Pool {
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
	accounts := p.store.Accounts()
	if len(accounts) == 0 {
		return nil
	}

	now := time.Now().Unix()
	start := int(atomic.AddUint64(&p.current, 1))
	available := make([]config.Account, 0, len(accounts))

	for i := 0; i < len(accounts); i++ {
		account := accounts[(start+i)%len(accounts)]
		if !account.Enabled {
			continue
		}
		if account.CooldownUntil > now {
			continue
		}
		if quotaResetAt(account.Quota) > now && account.Quota.Status == "exhausted" {
			continue
		}
		available = append(available, account)
	}

	return available
}

func (p *Pool) AvailableCount() int {
	accounts := p.store.Accounts()
	now := time.Now().Unix()
	count := 0
	for _, account := range accounts {
		if account.Enabled && account.CooldownUntil <= now && !(quotaResetAt(account.Quota) > now && account.Quota.Status == "exhausted") {
			count++
		}
	}
	return count
}

func quotaResetAt(quota config.QuotaInfo) int64 {
	var latest int64
	for _, bucket := range quota.Buckets {
		if bucket.ResetAt > latest {
			latest = bucket.ResetAt
		}
	}
	return latest
}
