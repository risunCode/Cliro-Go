package config

import (
	"os"
	"strings"
	"time"
)

func (m *Manager) UpsertAccount(account Account) error {
	now := time.Now().Unix()
	if account.CreatedAt == 0 {
		account.CreatedAt = now
	}
	account.UpdatedAt = now
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.accounts {
		if m.accounts[i].ID == account.ID {
			account.CreatedAt = m.accounts[i].CreatedAt
			m.accounts[i] = account
			return m.saveAccounts()
		}
	}
	m.accounts = append(m.accounts, account)
	return m.saveAccounts()
}

func (m *Manager) DeleteAccount(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.accounts {
		if m.accounts[i].ID == id {
			m.accounts = append(m.accounts[:i], m.accounts[i+1:]...)
			return m.saveAccounts()
		}
	}
	return nil
}

func (m *Manager) GetAccount(id string) (Account, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for i := range m.accounts {
		if m.accounts[i].ID == id {
			return m.accounts[i], true
		}
	}
	return Account{}, false
}

func (m *Manager) UpdateAccount(id string, fn func(*Account)) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.accounts {
		if m.accounts[i].ID == id {
			fn(&m.accounts[i])
			m.accounts[i].UpdatedAt = time.Now().Unix()
			return m.saveAccounts()
		}
	}
	return os.ErrNotExist
}

func (m *Manager) MarkAccountBanned(id string, reason string) error {
	trimmedReason := strings.TrimSpace(reason)
	return m.UpdateAccount(id, func(account *Account) {
		account.Enabled = false
		account.Banned = true
		account.BannedReason = trimmedReason
		account.HealthState = AccountHealthBanned
		account.HealthReason = trimmedReason
		account.CooldownUntil = 0
		account.ConsecutiveFailures = 0
		account.LastFailureAt = time.Now().Unix()
		if trimmedReason != "" {
			account.LastError = trimmedReason
		}
	})
}

func (m *Manager) MarkAccountHealthy(id string) error {
	return m.UpdateAccount(id, func(account *Account) {
		account.Banned = false
		account.BannedReason = ""
		account.CooldownUntil = 0
		account.ConsecutiveFailures = 0
		account.HealthState = AccountHealthReady
		account.HealthReason = ""
		if account.LastError != "" {
			account.LastError = ""
		}
	})
}

func (m *Manager) MarkAccountTransientCooldown(id string, reason string, cooldownUntil int64) error {
	trimmedReason := strings.TrimSpace(reason)
	return m.UpdateAccount(id, func(account *Account) {
		account.Banned = false
		account.BannedReason = ""
		account.HealthState = AccountHealthCooldownTransient
		account.HealthReason = trimmedReason
		account.CooldownUntil = cooldownUntil
		account.LastFailureAt = time.Now().Unix()
		if trimmedReason != "" {
			account.LastError = trimmedReason
		}
	})
}

func (m *Manager) MarkAccountQuotaCooldown(id string, reason string, cooldownUntil int64) error {
	trimmedReason := strings.TrimSpace(reason)
	return m.UpdateAccount(id, func(account *Account) {
		account.Banned = false
		account.BannedReason = ""
		account.HealthState = AccountHealthCooldownQuota
		account.HealthReason = trimmedReason
		account.CooldownUntil = cooldownUntil
		account.LastFailureAt = time.Now().Unix()
		if trimmedReason != "" {
			account.LastError = trimmedReason
		}
	})
}

func (m *Manager) MarkAccountDurablyDisabled(id string, reason string) error {
	trimmedReason := strings.TrimSpace(reason)
	return m.UpdateAccount(id, func(account *Account) {
		account.Enabled = false
		account.HealthState = AccountHealthDisabledDurable
		account.HealthReason = trimmedReason
		account.CooldownUntil = 0
		account.ConsecutiveFailures = 0
		account.LastFailureAt = time.Now().Unix()
		if trimmedReason != "" {
			account.LastError = trimmedReason
		}
	})
}

func (m *Manager) ClearTransientCooldown(id string) error {
	now := time.Now().Unix()
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.accounts {
		if m.accounts[i].ID != id {
			continue
		}
		if !shouldClearTransientCooldown(m.accounts[i], now) {
			return nil
		}
		clearTransientCooldown(&m.accounts[i])
		m.accounts[i].UpdatedAt = now
		return m.saveAccounts()
	}
	return os.ErrNotExist
}

func (m *Manager) UpdateStats(fn func(*ProxyStats)) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	fn(&m.stats)
	return m.saveStats()
}

func cloneAccounts(src []Account) []Account {
	if len(src) == 0 {
		return []Account{}
	}
	out := make([]Account, len(src))
	copy(out, src)
	for i := range out {
		if len(src[i].Quota.Buckets) > 0 {
			out[i].Quota.Buckets = append([]QuotaBucket(nil), src[i].Quota.Buckets...)
		}
	}
	return out
}

func cloneStartupWarnings(src []StartupWarning) []StartupWarning {
	if len(src) == 0 {
		return nil
	}
	out := make([]StartupWarning, len(src))
	copy(out, src)
	return out
}

func cloneModelAliases(src map[string]string) map[string]string {
	if len(src) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}

func clearTransientCooldown(account *Account) {
	if account == nil {
		return
	}
	account.CooldownUntil = 0
	account.ConsecutiveFailures = 0
	account.LastError = ""
	account.HealthState = AccountHealthReady
	account.HealthReason = ""
	account.LastFailureAt = 0
	if account.Quota.Status == "degraded" && strings.TrimSpace(account.Quota.Source) == "runtime" {
		account.Quota.Status = "healthy"
		account.Quota.Summary = "Transient backoff cleared manually."
		account.Quota.Error = ""
	}
}

func shouldClearTransientCooldown(account Account, now int64) bool {
	if account.Banned || account.HealthState == AccountHealthBanned {
		return false
	}
	if !account.Enabled || account.HealthState == AccountHealthDisabledDurable {
		return false
	}
	if isQuotaCooldownState(account, now) {
		return false
	}
	if account.HealthState == AccountHealthCooldownTransient {
		return true
	}
	return account.CooldownUntil > now
}

func isQuotaCooldownState(account Account, now int64) bool {
	if account.HealthState == AccountHealthCooldownQuota {
		return true
	}
	if strings.EqualFold(strings.TrimSpace(account.Quota.Status), "exhausted") && QuotaResetAt(account.Quota) > now {
		return true
	}
	return false
}
