package config

type StartupWarning struct {
	Code       string `json:"code"`
	FilePath   string `json:"filePath"`
	BackupPath string `json:"backupPath,omitempty"`
	Message    string `json:"message"`
}

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

type ThinkingMode string

const (
	ThinkingModeOff   ThinkingMode = "off"
	ThinkingModeAuto  ThinkingMode = "auto"
	ThinkingModeForce ThinkingMode = "force"
)

type ThinkingSettings struct {
	Suffix                    string       `json:"suffix,omitempty"`
	Mode                      ThinkingMode `json:"mode,omitempty"`
	FallbackTags              []string     `json:"fallbackTags,omitempty"`
	RequireAnthropicSignature bool         `json:"requireAnthropicSignature"`
	ForceForAnthropic         bool         `json:"forceForAnthropic"`
	MaxForcedThinkingTokens   int          `json:"maxForcedThinkingTokens,omitempty"`
}

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
	AuthMethod          string             `json:"authMethod,omitempty"`
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
	SchedulingMode    SchedulingMode      `json:"schedulingMode,omitempty"`
	Cloudflared       CloudflaredSettings `json:"cloudflared,omitempty"`
	Thinking          ThinkingSettings    `json:"thinking,omitempty"`
	ModelAliases      map[string]string   `json:"modelAliases,omitempty"`
	Accounts          []Account           `json:"accounts"`
	Stats             ProxyStats          `json:"stats"`
	StartupWarnings   []StartupWarning    `json:"startupWarnings,omitempty"`
}

const defaultProxyPort = 8095
const defaultThinkingSuffix = "-thinking"
const defaultMaxForcedThinkingTokens = 4000
