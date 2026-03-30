package config

import (
	"encoding/json"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

type StartupWarning struct {
	Code       string `json:"code"`
	FilePath   string `json:"filePath"`
	BackupPath string `json:"backupPath,omitempty"`
	Message    string `json:"message"`
}

type StreamMode string

const (
	StreamModeDefault  StreamMode = "default"
	StreamModeDisabled StreamMode = "false"
	StreamModeEnabled  StreamMode = "true"
)

type SchedulingMode string

const (
	SchedulingModeCacheFirst  SchedulingMode = "cache_first"
	SchedulingModeBalance     SchedulingMode = "balance"
	SchedulingModePerformance SchedulingMode = "performance"
)

type CloudflaredMode string

const (
	CloudflaredModeQuick CloudflaredMode = "quick"
	CloudflaredModeAuth  CloudflaredMode = "auth"
)

type CloudflaredSettings struct {
	Enabled  bool            `json:"enabled,omitempty"`
	Mode     CloudflaredMode `json:"mode,omitempty"`
	Token    string          `json:"token,omitempty"`
	UseHTTP2 bool            `json:"useHttp2,omitempty"`
}

var defaultCircuitStepValues = []int{10, 30, 60}

type AccountHealthState string

const (
	AccountHealthReady             AccountHealthState = "ready"
	AccountHealthCooldownQuota     AccountHealthState = "cooldown_quota"
	AccountHealthCooldownTransient AccountHealthState = "cooldown_transient"
	AccountHealthDisabledDurable   AccountHealthState = "disabled_durable"
	AccountHealthBanned            AccountHealthState = "banned"
)

type Account struct {
	ID                  string             `json:"id"`
	Provider            string             `json:"provider,omitempty"`
	Email               string             `json:"email"`
	AccountID           string             `json:"accountId,omitempty"`
	PlanType            string             `json:"planType,omitempty"`
	Quota               QuotaInfo          `json:"quota,omitempty"`
	AccessToken         string             `json:"accessToken"`
	RefreshToken        string             `json:"refreshToken"`
	IDToken             string             `json:"idToken,omitempty"`
	ClientID            string             `json:"clientId,omitempty"`
	ClientSecret        string             `json:"clientSecret,omitempty"`
	ExpiresAt           int64              `json:"expiresAt,omitempty"`
	Enabled             bool               `json:"enabled"`
	Banned              bool               `json:"banned,omitempty"`
	BannedReason        string             `json:"bannedReason,omitempty"`
	HealthState         AccountHealthState `json:"healthState,omitempty"`
	HealthReason        string             `json:"healthReason,omitempty"`
	CooldownUntil       int64              `json:"cooldownUntil,omitempty"`
	LastFailureAt       int64              `json:"lastFailureAt,omitempty"`
	ConsecutiveFailures int                `json:"consecutiveFailures,omitempty"`
	LastError           string             `json:"lastError,omitempty"`
	RequestCount        int                `json:"requestCount,omitempty"`
	ErrorCount          int                `json:"errorCount,omitempty"`
	PromptTokens        int                `json:"promptTokens,omitempty"`
	CompletionTokens    int                `json:"completionTokens,omitempty"`
	TotalTokens         int                `json:"totalTokens,omitempty"`
	LastUsed            int64              `json:"lastUsed,omitempty"`
	LastRefresh         int64              `json:"lastRefresh,omitempty"`
	CreatedAt           int64              `json:"createdAt"`
	UpdatedAt           int64              `json:"updatedAt"`
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
	ProxyPort         int                 `json:"proxyPort"`
	AllowLAN          bool                `json:"allowLan"`
	AutoStartProxy    bool                `json:"autoStartProxy"`
	ProxyAPIKey       string              `json:"proxyApiKey,omitempty"`
	AuthorizationMode bool                `json:"authorizationMode,omitempty"`
	StreamMode        StreamMode          `json:"streamMode,omitempty"`
	SchedulingMode    SchedulingMode      `json:"schedulingMode,omitempty"`
	CircuitBreaker    bool                `json:"circuitBreaker,omitempty"`
	CircuitSteps      []int               `json:"circuitSteps,omitempty"`
	Cloudflared       CloudflaredSettings `json:"cloudflared,omitempty"`
	Accounts          []Account           `json:"accounts"`
	Stats             ProxyStats          `json:"stats"`
	StartupWarnings   []StartupWarning    `json:"startupWarnings,omitempty"`
}

const defaultProxyPort = 8095

type Manager struct {
	mu      sync.RWMutex
	storage *Storage

	// In-memory cache
	settings        AppSettings
	accounts        []Account
	stats           ProxyStats
	startupWarnings []StartupWarning
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
	settings, warnings, err := m.storage.LoadSettings()
	if err != nil {
		return err
	}
	m.startupWarnings = append(m.startupWarnings, warnings...)

	// Apply defaults
	if settings.ProxyPort == 0 {
		settings.ProxyPort = defaultProxyPort
	}
	settings.StreamMode = string(normalizeStreamMode(StreamMode(settings.StreamMode)))
	settings.SchedulingMode = string(normalizeSchedulingMode(SchedulingMode(settings.SchedulingMode)))
	settings.CircuitSteps = normalizeCircuitSteps(settings.CircuitSteps)
	settings.Cloudflared = normalizeCloudflaredSettings(settings.Cloudflared)
	m.settings = settings

	// Load accounts
	accounts, warnings, err := m.storage.LoadAccounts()
	if err != nil {
		return err
	}
	m.startupWarnings = append(m.startupWarnings, warnings...)
	if accounts == nil {
		accounts = []Account{}
	}
	sort.Slice(accounts, func(i, j int) bool { return accounts[i].CreatedAt > accounts[j].CreatedAt })
	m.accounts = accounts

	// Load stats
	stats, warnings, err := m.storage.LoadStats()
	if err != nil {
		return err
	}
	m.startupWarnings = append(m.startupWarnings, warnings...)
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
		ProxyPort:         m.settings.ProxyPort,
		AllowLAN:          m.settings.AllowLAN,
		AutoStartProxy:    m.settings.AutoStartProxy,
		ProxyAPIKey:       m.settings.ProxyAPIKey,
		AuthorizationMode: m.settings.AuthorizationMode,
		StreamMode:        normalizeStreamMode(StreamMode(m.settings.StreamMode)),
		SchedulingMode:    normalizeSchedulingMode(SchedulingMode(m.settings.SchedulingMode)),
		CircuitBreaker:    m.settings.CircuitBreaker,
		CircuitSteps:      cloneIntSlice(m.settings.CircuitSteps),
		Cloudflared:       normalizeCloudflaredSettings(m.settings.Cloudflared),
		Accounts:          cloneAccounts(m.accounts),
		Stats:             m.stats,
		StartupWarnings:   cloneStartupWarnings(m.startupWarnings),
	}
}

func (m *Manager) StartupWarnings() []StartupWarning {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return cloneStartupWarnings(m.startupWarnings)
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

func (m *Manager) ProxyAPIKey() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return strings.TrimSpace(m.settings.ProxyAPIKey)
}

func (m *Manager) SetProxyAPIKey(apiKey string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.settings.ProxyAPIKey = strings.TrimSpace(apiKey)
	return m.saveSettings()
}

func (m *Manager) AuthorizationMode() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.settings.AuthorizationMode
}

func (m *Manager) SetAuthorizationMode(enabled bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.settings.AuthorizationMode = enabled
	return m.saveSettings()
}

func (m *Manager) StreamMode() StreamMode {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return normalizeStreamMode(StreamMode(m.settings.StreamMode))
}

func (m *Manager) SetStreamMode(mode StreamMode) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.settings.StreamMode = string(normalizeStreamMode(mode))
	return m.saveSettings()
}

func (m *Manager) SchedulingMode() SchedulingMode {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return normalizeSchedulingMode(SchedulingMode(m.settings.SchedulingMode))
}

func (m *Manager) SetSchedulingMode(mode SchedulingMode) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.settings.SchedulingMode = string(normalizeSchedulingMode(mode))
	return m.saveSettings()
}

func (m *Manager) CircuitBreaker() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.settings.CircuitBreaker
}

func (m *Manager) SetCircuitBreaker(enabled bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.settings.CircuitBreaker = enabled
	return m.saveSettings()
}

func (m *Manager) CircuitSteps() []int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return normalizeCircuitSteps(m.settings.CircuitSteps)
}

func (m *Manager) SetCircuitSteps(steps []int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.settings.CircuitSteps = normalizeCircuitSteps(steps)
	return m.saveSettings()
}

func (m *Manager) Cloudflared() CloudflaredSettings {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return normalizeCloudflaredSettings(m.settings.Cloudflared)
}

func (m *Manager) SetCloudflaredConfig(mode CloudflaredMode, token string, useHTTP2 bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	next := normalizeCloudflaredSettings(m.settings.Cloudflared)
	next.Mode = normalizeCloudflaredMode(mode)
	next.Token = strings.TrimSpace(token)
	next.UseHTTP2 = useHTTP2
	m.settings.Cloudflared = next
	return m.saveSettings()
}

func (m *Manager) SetCloudflaredEnabled(enabled bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	next := normalizeCloudflaredSettings(m.settings.Cloudflared)
	next.Enabled = enabled
	m.settings.Cloudflared = next
	return m.saveSettings()
}

func normalizeStreamMode(mode StreamMode) StreamMode {
	_ = mode
	return StreamModeEnabled
}

func normalizeSchedulingMode(mode SchedulingMode) SchedulingMode {
	switch SchedulingMode(strings.TrimSpace(string(mode))) {
	case SchedulingModeCacheFirst:
		return SchedulingModeCacheFirst
	case SchedulingModePerformance:
		return SchedulingModePerformance
	default:
		return SchedulingModeBalance
	}
}

func normalizeCloudflaredMode(mode CloudflaredMode) CloudflaredMode {
	switch CloudflaredMode(strings.TrimSpace(string(mode))) {
	case CloudflaredModeAuth:
		return CloudflaredModeAuth
	default:
		return CloudflaredModeQuick
	}
}

func defaultCloudflaredSettings() CloudflaredSettings {
	return CloudflaredSettings{
		Enabled:  false,
		Mode:     CloudflaredModeQuick,
		Token:    "",
		UseHTTP2: true,
	}
}

func normalizeCloudflaredSettings(settings CloudflaredSettings) CloudflaredSettings {
	normalized := defaultCloudflaredSettings()
	normalized.Enabled = settings.Enabled
	normalized.Mode = normalizeCloudflaredMode(settings.Mode)
	normalized.Token = strings.TrimSpace(settings.Token)
	normalized.UseHTTP2 = settings.UseHTTP2
	return normalized
}

func defaultCircuitSteps() []int {
	return append([]int(nil), defaultCircuitStepValues...)
}

func normalizeCircuitSteps(steps []int) []int {
	normalized := defaultCircuitSteps()
	for i := range normalized {
		if i >= len(steps) {
			continue
		}
		value := steps[i]
		if value <= 0 {
			continue
		}
		if value > 3600 {
			value = 3600
		}
		normalized[i] = value
	}
	return normalized
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

func cloneIntSlice(src []int) []int {
	if len(src) == 0 {
		return defaultCircuitSteps()
	}
	return append([]int(nil), src...)
}

func BlockedAccountReason(message string) (string, bool) {
	normalizedMessage := normalizeBlockedAccountMessage(message)
	if normalizedMessage == "" {
		return "", false
	}

	value := strings.ToLower(normalizedMessage)
	blockIndicators := []string{
		"deactivated",
		"banned",
		"suspended",
		"disabled by",
		"terminated",
		"closed",
		"token invalidated",
		"invalidated token",
		"token was invalidated",
		"token has been invalidated",
		"auth revoked",
		"access revoked",
		"refresh token revoked",
		"revoked",
	}

	for _, indicator := range blockIndicators {
		if strings.Contains(value, indicator) {
			return normalizedMessage, true
		}
	}

	return "", false
}

func RefreshableAuthReason(message string) (string, bool) {
	normalizedMessage := normalizeBlockedAccountMessage(message)
	if normalizedMessage == "" {
		return "", false
	}

	value := strings.ToLower(normalizedMessage)
	refreshableIndicators := []string{
		"bearer token included in the request is invalid",
		"bearer token is invalid",
		"token included in the request is invalid",
		"invalid bearer token",
		"token expired",
		"session expired",
		"expired access token",
		"access token expired",
		"unauthorized",
	}

	for _, indicator := range refreshableIndicators {
		if strings.Contains(value, indicator) {
			return normalizedMessage, true
		}
	}

	return "", false
}

func normalizeBlockedAccountMessage(message string) string {
	trimmed := strings.TrimSpace(message)
	if trimmed == "" {
		return ""
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(trimmed), &payload); err == nil {
		if extracted := strings.TrimSpace(extractBlockedMessageField(payload)); extracted != "" {
			return extracted
		}
	}

	return trimmed
}

func extractBlockedMessageField(payload map[string]any) string {
	if payload == nil {
		return ""
	}
	if msg, ok := payload["message"].(string); ok && strings.TrimSpace(msg) != "" {
		return msg
	}
	if reason, ok := payload["reason"].(string); ok && strings.TrimSpace(reason) != "" {
		return reason
	}
	if nested, ok := payload["error"].(map[string]any); ok {
		if msg := extractBlockedMessageField(nested); msg != "" {
			return msg
		}
	}
	return ""
}

func AccountLabel(account Account) string {
	if email := strings.TrimSpace(account.Email); email != "" {
		return email
	}
	if accountID := strings.TrimSpace(account.AccountID); accountID != "" {
		return accountID
	}
	return strings.TrimSpace(account.ID)
}
