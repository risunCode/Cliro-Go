package config

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

type ThinkingSettings struct {
	Suffix       string   `json:"suffix,omitempty"`
	FallbackTags []string `json:"fallbackTags,omitempty"`
}

type AppSettings struct {
	ProxyPort         int                 `json:"proxyPort"`
	AllowLAN          bool                `json:"allowLan"`
	AutoStartProxy    bool                `json:"autoStartProxy"`
	ProxyAPIKey       string              `json:"proxyApiKey,omitempty"`
	AuthorizationMode bool                `json:"authorizationMode,omitempty"`
	SchedulingMode    string              `json:"schedulingMode,omitempty"`
	Cloudflared       CloudflaredSettings `json:"cloudflared,omitempty"`
	Thinking          ThinkingSettings    `json:"thinking,omitempty"`
	ModelAliases      map[string]string   `json:"modelAliases,omitempty"`
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
