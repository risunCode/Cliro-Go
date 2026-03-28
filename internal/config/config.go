package config

import (
	"os"
	"sort"
	"sync"
	"time"
)

type Account struct {
	ID               string    `json:"id"`
	Provider         string    `json:"provider,omitempty"`
	Email            string    `json:"email"`
	AccountID        string    `json:"accountId,omitempty"`
	PlanType         string    `json:"planType,omitempty"`
	Quota            QuotaInfo `json:"quota,omitempty"`
	AccessToken      string    `json:"accessToken"`
	RefreshToken     string    `json:"refreshToken"`
	IDToken          string    `json:"idToken,omitempty"`
	ExpiresAt        int64     `json:"expiresAt,omitempty"`
	Enabled          bool      `json:"enabled"`
	CooldownUntil    int64     `json:"cooldownUntil,omitempty"`
	LastError        string    `json:"lastError,omitempty"`
	RequestCount     int       `json:"requestCount,omitempty"`
	ErrorCount       int       `json:"errorCount,omitempty"`
	PromptTokens     int       `json:"promptTokens,omitempty"`
	CompletionTokens int       `json:"completionTokens,omitempty"`
	TotalTokens      int       `json:"totalTokens,omitempty"`
	LastUsed         int64     `json:"lastUsed,omitempty"`
	LastRefresh      int64     `json:"lastRefresh,omitempty"`
	CreatedAt        int64     `json:"createdAt"`
	UpdatedAt        int64     `json:"updatedAt"`
}

type QuotaInfo struct {
	Status        string        `json:"status,omitempty"`
	Summary       string        `json:"summary,omitempty"`
	Source        string        `json:"source,omitempty"`
	Error         string        `json:"error,omitempty"`
	LastCheckedAt int64         `json:"lastCheckedAt,omitempty"`
	Buckets       []QuotaBucket `json:"buckets,omitempty"`
}

type QuotaBucket struct {
	Name      string `json:"name"`
	Used      int    `json:"used,omitempty"`
	Total     int    `json:"total,omitempty"`
	Remaining int    `json:"remaining,omitempty"`
	Percent   int    `json:"percent,omitempty"`
	ResetAt   int64  `json:"resetAt,omitempty"`
	Status    string `json:"status,omitempty"`
}

type ProxyStats struct {
	TotalRequests    int   `json:"totalRequests"`
	SuccessRequests  int   `json:"successRequests"`
	FailedRequests   int   `json:"failedRequests"`
	PromptTokens     int   `json:"promptTokens"`
	CompletionTokens int   `json:"completionTokens"`
	TotalTokens      int   `json:"totalTokens"`
	LastRequestAt    int64 `json:"lastRequestAt,omitempty"`
}

type AppConfig struct {
	ProxyPort      int        `json:"proxyPort"`
	AllowLAN       bool       `json:"allowLan"`
	AutoStartProxy bool       `json:"autoStartProxy"`
	Accounts       []Account  `json:"accounts"`
	Stats          ProxyStats `json:"stats"`
}

const defaultProxyPort = 8095

type Manager struct {
	mu      sync.RWMutex
	storage *Storage

	// In-memory cache
	settings AppSettings
	accounts []Account
	stats    ProxyStats
}

func NewManager(dataDir string) (*Manager, error) {
	storage, err := NewStorage(dataDir)
	if err != nil {
		return nil, err
	}

	m := &Manager{storage: storage}

	if err := m.load(); err != nil {
		return nil, err
	}
	return m, nil
}

func (m *Manager) load() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Load settings
	settings, err := m.storage.LoadSettings()
	if err != nil {
		return err
	}

	// Apply defaults
	if settings.ProxyPort == 0 {
		settings.ProxyPort = defaultProxyPort
	}
	m.settings = settings

	// Load accounts
	accounts, err := m.storage.LoadAccounts()
	if err != nil {
		return err
	}
	if accounts == nil {
		accounts = []Account{}
	}
	sort.Slice(accounts, func(i, j int) bool { return accounts[i].CreatedAt > accounts[j].CreatedAt })
	m.accounts = accounts

	// Load stats
	stats, err := m.storage.LoadStats()
	if err != nil {
		return err
	}
	m.stats = stats

	return nil
}

func (m *Manager) saveSettings() error {
	return m.storage.SaveSettings(m.settings)
}

func (m *Manager) saveAccounts() error {
	return m.storage.SaveAccounts(m.accounts)
}

func (m *Manager) saveStats() error {
	return m.storage.SaveStats(m.stats)
}

func (m *Manager) Snapshot() AppConfig {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return AppConfig{
		ProxyPort:      m.settings.ProxyPort,
		AllowLAN:       m.settings.AllowLAN,
		AutoStartProxy: m.settings.AutoStartProxy,
		Accounts:       cloneAccounts(m.accounts),
		Stats:          m.stats,
	}
}

func (m *Manager) Accounts() []Account {
	m.mu.RLock()
	defer m.mu.RUnlock()
	accounts := cloneAccounts(m.accounts)
	sort.Slice(accounts, func(i, j int) bool { return accounts[i].CreatedAt > accounts[j].CreatedAt })
	return accounts
}

func (m *Manager) ProxyPort() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.settings.ProxyPort == 0 {
		return defaultProxyPort
	}
	return m.settings.ProxyPort
}

func (m *Manager) SetProxyPort(port int) error {
	if port <= 0 {
		port = defaultProxyPort
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.settings.ProxyPort = port
	return m.saveSettings()
}

func (m *Manager) AllowLAN() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.settings.AllowLAN
}

func (m *Manager) SetAllowLAN(enabled bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.settings.AllowLAN = enabled
	return m.saveSettings()
}

func (m *Manager) AutoStartProxy() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.settings.AutoStartProxy
}

func (m *Manager) SetAutoStartProxy(enabled bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.settings.AutoStartProxy = enabled
	return m.saveSettings()
}

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
