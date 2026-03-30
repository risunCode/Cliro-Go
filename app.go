package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"cliro-go/internal/account"
	"cliro-go/internal/auth"
	"cliro-go/internal/clisync"
	"cliro-go/internal/cloudflared"
	"cliro-go/internal/config"
	"cliro-go/internal/gateway"
	"cliro-go/internal/logger"
	"cliro-go/internal/platform"

	"github.com/google/uuid"
	"github.com/wailsapp/wails/v2/pkg/options"
	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

type App struct {
	ctx   context.Context
	store *config.Manager
	log   *logger.Logger
	auth  *auth.Manager
	pool  *account.Pool
	proxy *gateway.Server
	cf    *cloudflared.Manager
	cli   *clisync.Service
}

type State struct {
	AuthMode          string                  `json:"authMode"`
	ProxyPort         int                     `json:"proxyPort"`
	ProxyURL          string                  `json:"proxyUrl"`
	ProxyBindAddr     string                  `json:"proxyBindAddress"`
	AllowLAN          bool                    `json:"allowLan"`
	AutoStartProxy    bool                    `json:"autoStartProxy"`
	ProxyAPIKey       string                  `json:"proxyApiKey,omitempty"`
	AuthorizationMode bool                    `json:"authorizationMode,omitempty"`
	SchedulingMode    string                  `json:"schedulingMode,omitempty"`
	CircuitBreaker    bool                    `json:"circuitBreaker,omitempty"`
	CircuitSteps      []int                   `json:"circuitSteps,omitempty"`
	ProxyRunning      bool                    `json:"proxyRunning"`
	AvailableCount    int                     `json:"availableCount"`
	Accounts          []config.Account        `json:"accounts"`
	Stats             config.ProxyStats       `json:"stats"`
	StartupWarnings   []config.StartupWarning `json:"startupWarnings,omitempty"`
}

type SecondLaunchNotice struct {
	Message          string   `json:"message"`
	Args             []string `json:"args,omitempty"`
	WorkingDirectory string   `json:"workingDirectory,omitempty"`
	ReceivedAt       int64    `json:"receivedAt"`
}

func NewApp() *App {
	return &App{}
}

func resolveDataDir() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(homeDir, ".cliro-go"), nil
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.log = logger.New(1000)
	a.log.AttachContext(ctx)

	dataDir, err := resolveDataDir()
	if err != nil {
		panic(err)
	}
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		panic(err)
	}
	if err := a.log.SetFile(filepath.Join(dataDir, "app.log")); err != nil {
		a.log.Warn("app", "failed to attach persistent log file: "+err.Error())
	}

	store, err := config.NewManager(dataDir)
	if err != nil {
		panic(err)
	}
	a.store = store
	a.pool = account.NewPool(store)
	a.auth = auth.NewManager(store, a.log)
	a.proxy = gateway.NewServer(store, a.auth, a.pool, a.log)
	a.cf = cloudflared.NewManager(dataDir, a.log)
	a.cli = clisync.NewService(a.log)
	for _, warning := range store.StartupWarnings() {
		a.log.Warn("config", warning.Message)
	}

	if store.AutoStartProxy() {
		if err := a.proxy.Start(store.ProxyPort(), store.AllowLAN()); err != nil {
			a.log.Error("app", "failed to auto-start proxy: "+err.Error())
		} else if err := a.startCloudflaredIfEnabled(); err != nil {
			a.log.Warn("cloudflared", "failed to auto-start cloudflared: "+err.Error())
		}
	} else {
		a.log.Info("app", "proxy auto-start disabled by configuration")
	}
	a.log.Info("app", "CLIro-Go backend initialized")
}

func (a *App) shutdown(_ context.Context) {
	if a.proxy != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = a.proxy.Stop(ctx)
	}
	if a.cf != nil {
		a.cf.Shutdown()
	}
	if a.auth != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = a.auth.Shutdown(ctx)
	}
}

func (a *App) GetState() State {
	if a.store == nil {
		return State{}
	}
	snap := a.store.Snapshot()
	port := a.store.ProxyPort()
	allowLAN := a.store.AllowLAN()
	bindAddr := platform.ProxyBindAddress(allowLAN, port)
	if a.proxy != nil && a.proxy.Running() {
		if runningBindAddr := a.proxy.BindAddress(); runningBindAddr != "" {
			bindAddr = runningBindAddr
		}
	}
	accounts := a.store.Accounts()
	return State{
		AuthMode:          "oauth_callback",
		ProxyPort:         port,
		ProxyURL:          platform.ProxyURL(port),
		ProxyBindAddr:     bindAddr,
		AllowLAN:          allowLAN,
		AutoStartProxy:    a.store.AutoStartProxy(),
		ProxyAPIKey:       a.store.ProxyAPIKey(),
		AuthorizationMode: a.store.AuthorizationMode(),
		SchedulingMode:    string(a.store.SchedulingMode()),
		CircuitBreaker:    a.store.CircuitBreaker(),
		CircuitSteps:      a.store.CircuitSteps(),
		ProxyRunning:      a.proxy != nil && a.proxy.Running(),
		AvailableCount:    a.pool.AvailableCount(),
		Accounts:          accounts,
		Stats:             snap.Stats,
		StartupWarnings:   a.store.StartupWarnings(),
	}
}

func (a *App) GetAccounts() []config.Account {
	if a.store == nil {
		return nil
	}
	return a.store.Accounts()
}

func (a *App) GetProxyStatus() map[string]any {
	if a.store == nil {
		return map[string]any{}
	}
	port := a.store.ProxyPort()
	allowLAN := a.store.AllowLAN()
	bindAddr := platform.ProxyBindAddress(allowLAN, port)
	if a.proxy != nil && a.proxy.Running() {
		if runningBindAddr := a.proxy.BindAddress(); runningBindAddr != "" {
			bindAddr = runningBindAddr
		}
	}
	cloudflaredConfig := a.store.Cloudflared()
	cloudflaredStatus := cloudflared.Status{}
	if a.cf != nil {
		cloudflaredStatus = a.cf.GetStatus()
	}
	return map[string]any{
		"running":           a.proxy != nil && a.proxy.Running(),
		"port":              port,
		"url":               platform.ProxyURL(port),
		"bindAddress":       bindAddr,
		"allowLan":          allowLAN,
		"autoStartProxy":    a.store.AutoStartProxy(),
		"proxyApiKey":       a.store.ProxyAPIKey(),
		"authorizationMode": a.store.AuthorizationMode(),
		"schedulingMode":    string(a.store.SchedulingMode()),
		"circuitBreaker":    a.store.CircuitBreaker(),
		"circuitSteps":      a.store.CircuitSteps(),
		"cloudflared": map[string]any{
			"enabled":   cloudflaredConfig.Enabled,
			"mode":      string(cloudflaredConfig.Mode),
			"token":     cloudflaredConfig.Token,
			"useHttp2":  cloudflaredConfig.UseHTTP2,
			"installed": cloudflaredStatus.Installed,
			"version":   cloudflaredStatus.Version,
			"running":   cloudflaredStatus.Running,
			"url":       cloudflaredStatus.URL,
			"error":     cloudflaredStatus.Error,
		},
	}
}

func (a *App) RefreshCloudflaredStatus() map[string]any {
	status := a.GetProxyStatus()
	if a.store == nil || a.cf == nil {
		return status
	}

	cloudflaredConfig := a.store.Cloudflared()
	cloudflaredStatus := a.cf.RefreshStatus()
	status["cloudflared"] = map[string]any{
		"enabled":   cloudflaredConfig.Enabled,
		"mode":      string(cloudflaredConfig.Mode),
		"token":     cloudflaredConfig.Token,
		"useHttp2":  cloudflaredConfig.UseHTTP2,
		"installed": cloudflaredStatus.Installed,
		"version":   cloudflaredStatus.Version,
		"running":   cloudflaredStatus.Running,
		"url":       cloudflaredStatus.URL,
		"error":     cloudflaredStatus.Error,
	}
	return status
}

func (a *App) GetLogs(limit int) []logger.Entry {
	if a.log == nil {
		return nil
	}
	return a.log.Entries(limit)
}

func (a *App) GetHostName() string {
	host, err := os.Hostname()
	if err != nil {
		return "This PC"
	}
	host = strings.TrimSpace(host)
	if host == "" {
		return "This PC"
	}
	return host
}

func (a *App) StartCodexAuth() (*auth.CodexAuthStart, error) {
	return a.auth.StartCodexAuth()
}

func (a *App) GetCodexAuthSession(sessionID string) auth.CodexAuthSessionView {
	return a.auth.GetCodexAuthSession(sessionID)
}

func (a *App) CancelCodexAuth(sessionID string) {
	a.auth.CancelCodexAuth(sessionID)
}

func (a *App) StartKiroAuth() (*auth.KiroAuthStart, error) {
	return a.auth.StartKiroAuth()
}

func (a *App) StartKiroSocialAuth(provider string) (*auth.KiroAuthStart, error) {
	return a.auth.StartKiroSocialAuth(provider)
}

func (a *App) GetKiroAuthSession(sessionID string) auth.KiroAuthSessionView {
	return a.auth.GetKiroAuthSession(sessionID)
}

func (a *App) CancelKiroAuth(sessionID string) {
	a.auth.CancelKiroAuth(sessionID)
}

func (a *App) RefreshAccount(accountID string) error {
	_, err := a.auth.RefreshAccount(accountID)
	return err
}

func (a *App) RefreshAccountWithQuota(accountID string) error {
	_, err := a.auth.RefreshAccountWithQuota(accountID)
	return err
}

func (a *App) RefreshQuota(accountID string) error {
	_, err := a.auth.RefreshQuota(accountID)
	return err
}

func (a *App) RefreshAllQuotas() error {
	return a.auth.RefreshAllQuotas()
}

func (a *App) ForceRefreshAllQuotas() error {
	return a.auth.ForceRefreshAllQuotas()
}

func (a *App) GetLocalModelCatalog() []map[string]any {
	if a.cli == nil {
		return nil
	}
	models := a.cli.ModelCatalog()
	out := make([]map[string]any, 0, len(models))
	for _, model := range models {
		out = append(out, map[string]any{
			"id":      model.ID,
			"ownedBy": model.OwnedBy,
		})
	}
	return out
}

func (a *App) GetCLISyncStatuses() ([]map[string]any, error) {
	if a.cli == nil || a.store == nil {
		return nil, fmt.Errorf("cli sync service is not ready")
	}
	statuses, err := a.cli.Statuses(platform.ProxyURL(a.store.ProxyPort()))
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(statuses))
	for _, status := range statuses {
		files := make([]map[string]any, 0, len(status.Files))
		for _, file := range status.Files {
			files = append(files, map[string]any{"name": file.Name, "path": file.Path})
		}
		out = append(out, map[string]any{
			"id":             status.ID,
			"label":          status.Label,
			"installed":      status.Installed,
			"version":        status.Version,
			"synced":         status.Synced,
			"currentBaseUrl": status.CurrentBaseURL,
			"currentModel":   status.CurrentModel,
			"files":          files,
		})
	}
	return out, nil
}

func (a *App) SyncCLIConfig(appID string, model string) (map[string]any, error) {
	if a.cli == nil || a.store == nil {
		return nil, fmt.Errorf("cli sync service is not ready")
	}
	apiKey := strings.TrimSpace(a.store.ProxyAPIKey())
	if apiKey == "" {
		generated, err := generateProxyAPIKey()
		if err != nil {
			return nil, err
		}
		if err := a.store.SetProxyAPIKey(generated); err != nil {
			return nil, err
		}
		apiKey = generated
		a.log.Info("proxy", "proxy API key autogenerated for cli sync")
	}
	result, err := a.cli.Sync(clisync.App(appID), platform.ProxyURL(a.store.ProxyPort()), apiKey, model)
	if err != nil {
		return nil, err
	}
	files := make([]map[string]any, 0, len(result.Files))
	for _, file := range result.Files {
		files = append(files, map[string]any{"name": file.Name, "path": file.Path})
	}
	return map[string]any{
		"id":             result.ID,
		"label":          result.Label,
		"model":          result.Model,
		"currentBaseUrl": result.CurrentBaseURL,
		"files":          files,
	}, nil
}

func (a *App) GetCLISyncFileContent(appID string, path string) (string, error) {
	if a.cli == nil {
		return "", fmt.Errorf("cli sync service is not ready")
	}
	return a.cli.ReadConfigFile(clisync.App(appID), path)
}

func (a *App) SaveCLISyncFileContent(appID string, path string, content string) error {
	if a.cli == nil {
		return fmt.Errorf("cli sync service is not ready")
	}
	return a.cli.WriteConfigFile(clisync.App(appID), path, content)
}

func (a *App) SyncCodexAccountToKiloAuth(accountID string) (auth.KiloAuthSyncResult, error) {
	a.log.Info("sync", "syncing account to Kilo CLI auth: "+accountID)
	result, err := a.auth.SyncCodexAccountToKiloAuth(accountID)
	if err != nil {
		a.log.Error("sync", "Kilo CLI sync failed for "+accountID+": "+err.Error())
		return auth.KiloAuthSyncResult{}, err
	}
	a.log.Info("sync", "Kilo CLI sync completed for "+accountID+" -> "+result.TargetPath)
	return result, nil
}

func (a *App) SyncCodexAccountToCodexCLI(accountID string) (auth.CodexAuthSyncResult, error) {
	a.log.Info("sync", "syncing account to Codex CLI auth: "+accountID)
	result, err := a.auth.SyncCodexAccountToCodexCLI(accountID)
	if err != nil {
		a.log.Error("sync", "Codex CLI sync failed for "+accountID+": "+err.Error())
		return auth.CodexAuthSyncResult{}, err
	}
	a.log.Info("sync", "Codex CLI sync completed for "+accountID+" -> "+result.TargetPath)
	return result, nil
}

func (a *App) SyncCodexAccountToOpencodeAuth(accountID string) (auth.OpencodeAuthSyncResult, error) {
	a.log.Info("sync", "syncing account to Opencode auth: "+accountID)
	result, err := a.auth.SyncCodexAccountToOpencodeAuth(accountID)
	if err != nil {
		a.log.Error("sync", "Opencode sync failed for "+accountID+": "+err.Error())
		return auth.OpencodeAuthSyncResult{}, err
	}
	a.log.Info("sync", "Opencode sync completed for "+accountID+" -> "+result.TargetPath)
	return result, nil
}

func (a *App) DeleteAccount(accountID string) error {
	a.log.Info("account", "deleting account "+accountID)
	return a.store.DeleteAccount(accountID)
}

func (a *App) ToggleAccount(accountID string, enabled bool) error {
	return a.store.UpdateAccount(accountID, func(account *config.Account) {
		account.Enabled = enabled
	})
}

func (a *App) ImportAccounts(accounts []config.Account) (int, error) {
	if a.store == nil {
		return 0, fmt.Errorf("store unavailable")
	}
	if len(accounts) == 0 {
		return 0, fmt.Errorf("no accounts provided")
	}

	now := time.Now().Unix()
	imported := 0
	var failures []string

	for idx, raw := range accounts {
		account := raw
		account.ID = strings.TrimSpace(account.ID)
		account.Provider = strings.TrimSpace(strings.ToLower(account.Provider))
		account.Email = strings.TrimSpace(account.Email)
		account.AccountID = strings.TrimSpace(account.AccountID)
		account.AccessToken = strings.TrimSpace(account.AccessToken)
		account.RefreshToken = strings.TrimSpace(account.RefreshToken)
		account.IDToken = strings.TrimSpace(account.IDToken)
		account.PlanType = strings.TrimSpace(account.PlanType)
		account.LastError = strings.TrimSpace(account.LastError)

		if account.ID == "" {
			account.ID = uuid.NewString()
		}
		if account.Provider == "" {
			failures = append(failures, fmt.Sprintf("entry %d missing provider", idx+1))
			continue
		}
		if account.Provider != "codex" && account.Provider != "kiro" {
			failures = append(failures, fmt.Sprintf("entry %d has unsupported provider %q", idx+1, account.Provider))
			continue
		}
		if !account.Enabled {
			account.Enabled = true
		}
		if account.CreatedAt == 0 {
			account.CreatedAt = now
		}
		if account.LastRefresh == 0 {
			account.LastRefresh = now
		}
		account.UpdatedAt = now

		if account.AccessToken == "" || account.RefreshToken == "" {
			failures = append(failures, fmt.Sprintf("entry %d missing access/refresh token", idx+1))
			continue
		}

		if err := a.store.UpsertAccount(account); err != nil {
			failures = append(failures, fmt.Sprintf("entry %d failed: %v", idx+1, err))
			continue
		}
		imported++
	}

	if imported == 0 && len(failures) > 0 {
		return 0, fmt.Errorf("no accounts imported: %s", strings.Join(failures, "; "))
	}
	if len(failures) > 0 {
		return imported, fmt.Errorf("partial import (%d/%d): %s", imported, len(accounts), strings.Join(failures, "; "))
	}

	return imported, nil
}

func (a *App) ClearCooldown(accountID string) error {
	return a.store.UpdateAccount(accountID, func(account *config.Account) {
		account.CooldownUntil = 0
		account.ConsecutiveFailures = 0
		account.LastError = ""
		if account.Quota.Status == "exhausted" {
			account.Quota.Status = "healthy"
			account.Quota.Summary = "Cooldown cleared manually."
			for i := range account.Quota.Buckets {
				if account.Quota.Buckets[i].Status == "exhausted" {
					account.Quota.Buckets[i].Status = "healthy"
				}
			}
		}
	})
}

func (a *App) startCloudflaredIfEnabled() error {
	if a.store == nil || a.cf == nil || a.proxy == nil || !a.proxy.Running() {
		return nil
	}
	settings := a.store.Cloudflared()
	if !settings.Enabled {
		return nil
	}
	_, err := a.cf.Start(settings, a.store.ProxyPort())
	return err
}

func (a *App) stopCloudflaredRuntime() error {
	if a.cf == nil {
		return nil
	}
	_, err := a.cf.Stop()
	return err
}

func (a *App) SetCloudflaredConfig(mode string, token string, useHTTP2 bool) error {
	if a.store == nil {
		return fmt.Errorf("store is not ready")
	}
	if err := a.store.SetCloudflaredConfig(config.CloudflaredMode(mode), token, useHTTP2); err != nil {
		return err
	}
	settings := a.store.Cloudflared()
	a.log.Info("cloudflared", fmt.Sprintf("cloudflared config updated mode=%q useHttp2=%t", settings.Mode, settings.UseHTTP2))
	return nil
}

func (a *App) InstallCloudflared() error {
	if a.cf == nil {
		return fmt.Errorf("cloudflared manager is not ready")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	_, err := a.cf.Install(ctx)
	if err != nil {
		a.log.Error("cloudflared", "cloudflared install failed: "+err.Error())
		return err
	}
	a.log.Info("cloudflared", "cloudflared installed")
	return nil
}

func (a *App) StartCloudflared() error {
	if a.store == nil || a.cf == nil {
		return fmt.Errorf("cloudflared manager is not ready")
	}
	if a.proxy == nil || !a.proxy.Running() {
		return fmt.Errorf("please start the local proxy service first")
	}
	settings := a.store.Cloudflared()
	if _, err := a.cf.Start(settings, a.store.ProxyPort()); err != nil {
		a.log.Error("cloudflared", "cloudflared start failed: "+err.Error())
		return err
	}
	if err := a.store.SetCloudflaredEnabled(true); err != nil {
		return err
	}
	a.log.Info("cloudflared", "cloudflared tunnel started")
	return nil
}

func (a *App) StopCloudflared() error {
	if a.store == nil || a.cf == nil {
		return fmt.Errorf("cloudflared manager is not ready")
	}
	if _, err := a.cf.Stop(); err != nil {
		a.log.Error("cloudflared", "cloudflared stop failed: "+err.Error())
		return err
	}
	if err := a.store.SetCloudflaredEnabled(false); err != nil {
		return err
	}
	a.log.Info("cloudflared", "cloudflared tunnel stopped")
	return nil
}

func (a *App) StartProxy() error {
	a.log.Info("proxy", "starting proxy service")
	if err := a.proxy.Start(a.store.ProxyPort(), a.store.AllowLAN()); err != nil {
		return err
	}
	if err := a.startCloudflaredIfEnabled(); err != nil {
		a.log.Warn("cloudflared", "cloudflared did not start with proxy: "+err.Error())
	}
	return nil
}

func (a *App) StopProxy() error {
	a.log.Info("proxy", "stopping proxy service")
	if err := a.stopCloudflaredRuntime(); err != nil {
		a.log.Warn("cloudflared", "cloudflared did not stop with proxy: "+err.Error())
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return a.proxy.Stop(ctx)
}

func (a *App) SetProxyPort(port int) error {
	if a.store == nil {
		return fmt.Errorf("store is not ready")
	}
	running := a.proxy != nil && a.proxy.Running()
	restartCloudflared := a.store.Cloudflared().Enabled
	if running && restartCloudflared {
		if err := a.stopCloudflaredRuntime(); err != nil {
			a.log.Warn("cloudflared", "cloudflared did not stop before proxy port change: "+err.Error())
		}
	}
	if running {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := a.proxy.Stop(ctx); err != nil {
			return err
		}
	}
	if err := a.store.SetProxyPort(port); err != nil {
		return err
	}
	a.log.Info("proxy", fmt.Sprintf("proxy port updated to %d", a.store.ProxyPort()))
	if running {
		if err := a.proxy.Start(a.store.ProxyPort(), a.store.AllowLAN()); err != nil {
			return err
		}
		if restartCloudflared {
			if err := a.startCloudflaredIfEnabled(); err != nil {
				a.log.Warn("cloudflared", "cloudflared did not restart after proxy port change: "+err.Error())
			}
		}
	}
	return nil
}

func (a *App) SetAllowLAN(enabled bool) error {
	if a.store == nil {
		return fmt.Errorf("store is not ready")
	}
	running := a.proxy != nil && a.proxy.Running()
	restartCloudflared := a.store.Cloudflared().Enabled
	if running && restartCloudflared {
		if err := a.stopCloudflaredRuntime(); err != nil {
			a.log.Warn("cloudflared", "cloudflared did not stop before proxy network update: "+err.Error())
		}
	}
	if running {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := a.proxy.Stop(ctx); err != nil {
			return err
		}
	}
	if err := a.store.SetAllowLAN(enabled); err != nil {
		return err
	}
	a.log.Info("proxy", fmt.Sprintf("proxy allowLan updated to %t", enabled))
	if running {
		if err := a.proxy.Start(a.store.ProxyPort(), enabled); err != nil {
			return err
		}
		if restartCloudflared {
			if err := a.startCloudflaredIfEnabled(); err != nil {
				a.log.Warn("cloudflared", "cloudflared did not restart after proxy network update: "+err.Error())
			}
		}
	}
	return nil
}

func (a *App) SetAutoStartProxy(enabled bool) error {
	if a.store == nil {
		return fmt.Errorf("store is not ready")
	}
	if err := a.store.SetAutoStartProxy(enabled); err != nil {
		return err
	}
	a.log.Info("proxy", fmt.Sprintf("proxy autoStartProxy updated to %t", enabled))
	return nil
}

func (a *App) SetProxyAPIKey(apiKey string) error {
	if a.store == nil {
		return fmt.Errorf("store is not ready")
	}
	normalized := strings.TrimSpace(apiKey)
	if normalized == "" {
		return fmt.Errorf("proxy API key cannot be empty")
	}
	if err := a.store.SetProxyAPIKey(normalized); err != nil {
		return err
	}
	a.log.Info("proxy", "proxy API key updated")
	return nil
}

func generateProxyAPIKey() (string, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return "sk-cliro_" + hex.EncodeToString(raw), nil
}

func (a *App) RegenerateProxyAPIKey() (string, error) {
	if a.store == nil {
		return "", fmt.Errorf("store is not ready")
	}
	apiKey, err := generateProxyAPIKey()
	if err != nil {
		return "", err
	}
	if err := a.store.SetProxyAPIKey(apiKey); err != nil {
		return "", err
	}
	a.log.Info("proxy", "proxy API key regenerated")
	return apiKey, nil
}

func (a *App) SetAuthorizationMode(enabled bool) error {
	if a.store == nil {
		return fmt.Errorf("store is not ready")
	}
	if err := a.store.SetAuthorizationMode(enabled); err != nil {
		return err
	}
	a.log.Info("proxy", fmt.Sprintf("proxy authorizationMode updated to %t", enabled))
	return nil
}

func (a *App) SetSchedulingMode(mode string) error {
	if a.store == nil {
		return fmt.Errorf("store is not ready")
	}
	if err := a.store.SetSchedulingMode(config.SchedulingMode(mode)); err != nil {
		return err
	}
	a.log.Info("proxy", fmt.Sprintf("proxy schedulingMode updated to %q", string(a.store.SchedulingMode())))
	return nil
}

func (a *App) SetCircuitBreaker(enabled bool) error {
	if a.store == nil {
		return fmt.Errorf("store is not ready")
	}
	if err := a.store.SetCircuitBreaker(enabled); err != nil {
		return err
	}
	a.log.Info("proxy", fmt.Sprintf("proxy circuitBreaker updated to %t", a.store.CircuitBreaker()))
	return nil
}

func (a *App) SetCircuitSteps(steps []int) error {
	if a.store == nil {
		return fmt.Errorf("store is not ready")
	}
	if err := a.store.SetCircuitSteps(steps); err != nil {
		return err
	}
	a.log.Info("proxy", fmt.Sprintf("proxy circuitSteps updated to %v", a.store.CircuitSteps()))
	return nil
}

func (a *App) ClearLogs() {
	if a.log == nil {
		return
	}
	a.log.Clear()
	a.log.Info("system", "logs cleared")
}

func (a *App) OpenExternalURL(rawURL string) {
	if a.ctx == nil || rawURL == "" {
		return
	}
	wruntime.BrowserOpenURL(a.ctx, rawURL)
}

func (a *App) OpenDataDir() error {
	dataDir, err := resolveDataDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return err
	}

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("explorer", filepath.Clean(dataDir))
	case "darwin":
		cmd = exec.Command("open", dataDir)
	default:
		cmd = exec.Command("xdg-open", dataDir)
	}

	if err := cmd.Start(); err != nil {
		return err
	}
	return nil
}

func buildSecondLaunchNotice(data options.SecondInstanceData) SecondLaunchNotice {
	return SecondLaunchNotice{
		Message:          "CLIro-Go is already running. Restored the existing window.",
		Args:             append([]string(nil), data.Args...),
		WorkingDirectory: strings.TrimSpace(data.WorkingDirectory),
		ReceivedAt:       time.Now().Unix(),
	}
}

func (a *App) onSecondInstanceLaunch(data options.SecondInstanceData) {
	if a.ctx == nil {
		return
	}
	notice := buildSecondLaunchNotice(data)
	a.log.Info("app", fmt.Sprintf("second instance launch received working_dir=%q args=%q", notice.WorkingDirectory, strings.Join(notice.Args, " ")))
	wruntime.WindowUnminimise(a.ctx)
	wruntime.WindowShow(a.ctx)
	wruntime.Show(a.ctx)
	wruntime.WindowSetAlwaysOnTop(a.ctx, true)
	go func(ctx context.Context) {
		time.Sleep(250 * time.Millisecond)
		wruntime.WindowSetAlwaysOnTop(ctx, false)
	}(a.ctx)
	wruntime.EventsEmit(a.ctx, "app:second-instance", notice)
}
