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
	"sync"
	"time"

	"cliro-go/internal/account"
	"cliro-go/internal/auth"
	"cliro-go/internal/cloudflared"
	"cliro-go/internal/config"
	"cliro-go/internal/gateway"
	"cliro-go/internal/logger"
	"cliro-go/internal/platform"
	providerquota "cliro-go/internal/provider/quota"
	"cliro-go/internal/sync/cliconfig"
	"cliro-go/internal/tray"

	"github.com/google/uuid"
	"github.com/wailsapp/wails/v2/pkg/options"
	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

type App struct {
	ctx   context.Context
	store *config.Manager
	log   *logger.Logger
	auth  *auth.Manager
	quota *providerquota.Service
	pool  *account.Pool
	proxy *gateway.Server
	cf    *cloudflared.Manager
	cli   *cliconfig.Service
	tray  tray.Controller

	lifecycleMu    sync.Mutex
	quitAuthorized bool
	shuttingDown   bool

	emitEvent          func(context.Context, string, ...interface{})
	quitApp            func(context.Context)
	hideWindow         func(context.Context)
	showWindow         func(context.Context)
	showApp            func(context.Context)
	unminimiseWindow   func(context.Context)
	setWindowAlwaysTop func(context.Context, bool)
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
	ProxyRunning      bool                    `json:"proxyRunning"`
	AvailableCount    int                     `json:"availableCount"`
	Accounts          []config.Account        `json:"accounts"`
	Stats             config.ProxyStats       `json:"stats"`
	StartupWarnings   []config.StartupWarning `json:"startupWarnings,omitempty"`
	TraySupported     bool                    `json:"traySupported"`
	TrayAvailable     bool                    `json:"trayAvailable"`
}

type SecondLaunchNotice struct {
	Message          string   `json:"message"`
	Args             []string `json:"args,omitempty"`
	WorkingDirectory string   `json:"workingDirectory,omitempty"`
	ReceivedAt       int64    `json:"receivedAt"`
}

func NewApp() *App {
	return &App{
		tray:               tray.NewController(),
		emitEvent:          wruntime.EventsEmit,
		quitApp:            wruntime.Quit,
		hideWindow:         wruntime.WindowHide,
		showWindow:         wruntime.WindowShow,
		showApp:            wruntime.Show,
		unminimiseWindow:   wruntime.WindowUnminimise,
		setWindowAlwaysTop: wruntime.WindowSetAlwaysOnTop,
	}
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

	// Auto-register kiro:// protocol handler on Windows
	if registered, err := platform.EnsureProtocolRegistered(); err != nil {
		a.log.Warn("app", "failed to register kiro:// protocol handler: "+err.Error())
	} else if registered {
		a.log.Info("app", "kiro:// protocol handler registered successfully")
	} else {
		a.log.Info("app", "kiro:// protocol handler already registered")
	}

	store, err := config.NewManager(dataDir)
	if err != nil {
		panic(err)
	}
	a.store = store
	a.pool = account.NewPool(store)
	a.auth = auth.NewManager(store, a.log)
	a.quota = providerquota.NewService(store, a.auth, a.log, a.auth.HTTPClient())
	a.auth.SetQuotaRefresher(a.quota)
	a.proxy = gateway.NewServer(store, a.auth, a.pool, a.log)
	a.cf = cloudflared.NewManager(dataDir, a.log)
	if a.cf != nil {
		a.cf.RefreshStatus()
	}
	a.cli = cliconfig.NewService(a.log)
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
	a.initTray(ctx)
	a.syncTrayProxyState()
	a.log.Info("app", "CLIro-Go backend initialized")
}

func (a *App) shutdown(_ context.Context) {
	a.lifecycleMu.Lock()
	a.shuttingDown = true
	a.lifecycleMu.Unlock()
	if a.tray != nil {
		if err := a.tray.Close(); err != nil && a.log != nil {
			a.log.Warn("tray", "failed to close system tray: "+err.Error())
		}
	}
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
	traySupported, trayAvailable := a.trayState()
	if a.store == nil {
		return State{TraySupported: traySupported, TrayAvailable: trayAvailable}
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
		ProxyRunning:      a.proxy != nil && a.proxy.Running(),
		AvailableCount:    a.pool.AvailableCount(),
		Accounts:          accounts,
		Stats:             snap.Stats,
		StartupWarnings:   a.store.StartupWarnings(),
		TraySupported:     traySupported,
		TrayAvailable:     trayAvailable,
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

func (a *App) SubmitCodexAuthCode(sessionID string, code string) error {
	return a.auth.SubmitCodexAuthCode(sessionID, code)
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

func (a *App) SubmitKiroAuthCode(sessionID string, code string) error {
	return a.auth.SubmitKiroAuthCode(sessionID, code)
}

func (a *App) RefreshAccount(accountID string) error {
	_, err := a.auth.RefreshAccount(accountID)
	return err
}

func (a *App) RefreshAccountWithQuota(accountID string) error {
	_, err := a.quota.RefreshAccountWithQuota(accountID)
	return err
}

func (a *App) RefreshQuota(accountID string) error {
	_, err := a.quota.RefreshQuota(accountID)
	return err
}

func (a *App) RefreshAllQuotas() error {
	return a.quota.RefreshAllQuotas()
}

func (a *App) ForceRefreshAllQuotas() error {
	return a.quota.ForceRefreshAllQuotas()
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
			"installPath":    status.InstallPath,
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
	result, err := a.cli.Sync(cliconfig.App(appID), platform.ProxyURL(a.store.ProxyPort()), apiKey, model)
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
	return a.cli.ReadConfigFile(cliconfig.App(appID), path)
}

func (a *App) SaveCLISyncFileContent(appID string, path string, content string) error {
	if a.cli == nil {
		return fmt.Errorf("cli sync service is not ready")
	}
	return a.cli.WriteConfigFile(cliconfig.App(appID), path, content)
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
	now := time.Now().Unix()
	return a.store.UpdateAccount(accountID, func(account *config.Account) {
		previousState := account.HealthState
		account.Enabled = enabled

		if !enabled {
			if account.Banned || account.HealthState == config.AccountHealthBanned {
				return
			}
			account.HealthState = config.AccountHealthDisabledDurable
			account.HealthReason = "Disabled by user"
			account.CooldownUntil = 0
			account.ConsecutiveFailures = 0
			return
		}

		if account.Banned || account.HealthState == config.AccountHealthBanned {
			account.HealthState = config.AccountHealthBanned
			account.HealthReason = strings.TrimSpace(account.BannedReason)
			return
		}

		if shouldApplyQuotaCooldownToAccount(*account, now) {
			resetAt := account.CooldownUntil
			if quotaReset := config.QuotaResetAt(account.Quota); quotaReset > resetAt {
				resetAt = quotaReset
			}
			if resetAt > now {
				account.CooldownUntil = resetAt
			}
			account.HealthState = config.AccountHealthCooldownQuota
			if strings.TrimSpace(account.HealthReason) == "" {
				account.HealthReason = firstNonEmptyMessage(strings.TrimSpace(account.Quota.Summary), strings.TrimSpace(account.LastError), "Quota exhausted")
			}
			return
		}

		account.CooldownUntil = 0
		account.ConsecutiveFailures = 0
		account.HealthState = config.AccountHealthReady
		if previousState == config.AccountHealthDisabledDurable || strings.EqualFold(strings.TrimSpace(account.HealthReason), "disabled by user") {
			account.HealthReason = ""
			return
		}
		account.HealthReason = ""
	})
}

func shouldApplyQuotaCooldownToAccount(account config.Account, now int64) bool {
	if account.HealthState == config.AccountHealthCooldownQuota && account.CooldownUntil > now {
		return true
	}

	status := strings.ToLower(strings.TrimSpace(account.Quota.Status))
	if status != "exhausted" && status != "empty" {
		return false
	}

	if account.CooldownUntil > now {
		return true
	}

	return config.QuotaResetAt(account.Quota) > now
}

func firstNonEmptyMessage(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
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
	if a.store == nil {
		return fmt.Errorf("store is not ready")
	}
	err := a.store.ClearTransientCooldown(accountID)
	if err == nil && a.log != nil {
		a.log.InfoEvent("proxy", "cooldown.reset_transient", logger.String("account_id", accountID))
	}
	return err
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

func (a *App) GetModelAliases() (map[string]string, error) {
	if a.store == nil {
		return nil, fmt.Errorf("store is not ready")
	}
	return a.store.ModelAliases(), nil
}

func (a *App) SetModelAliases(aliases map[string]string) error {
	if a.store == nil {
		return fmt.Errorf("store is not ready")
	}
	if err := a.store.SetModelAliases(aliases); err != nil {
		return err
	}
	a.log.Info("config", fmt.Sprintf("model aliases updated count=%d", len(aliases)))
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
	defer a.syncTrayProxyState()
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
	defer a.syncTrayProxyState()
	a.log.Info("proxy", "stopping proxy service")
	if err := a.stopCloudflaredRuntime(); err != nil {
		a.log.Warn("cloudflared", "cloudflared did not stop with proxy: "+err.Error())
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return a.proxy.Stop(ctx)
}

func (a *App) SetProxyPort(port int) error {
	defer a.syncTrayProxyState()
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
	defer a.syncTrayProxyState()
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

func (a *App) ToggleProxyFromTray() error {
	if a.proxy == nil {
		return fmt.Errorf("proxy service is not ready")
	}
	if err := toggleProxyByState(a.proxy.Running(), a.StartProxy, a.StopProxy); err != nil {
		return err
	}
	running := a.proxy.Running()
	a.syncTrayProxyState()
	a.emit("app:proxy-state-changed", map[string]any{
		"source":  "tray",
		"running": running,
	})
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

func (a *App) beforeCloseGuard(ctx context.Context) bool {
	if a.ctx == nil && ctx != nil {
		a.ctx = ctx
	}
	if a.consumeQuitAuthorization() {
		return false
	}
	a.emit("app:close-requested")
	return true
}

func (a *App) ConfirmQuit() error {
	return a.requestQuit()
}

func (a *App) HideToTray() {
	if a.ctx == nil {
		return
	}
	if a.hideWindow != nil {
		a.hideWindow(a.ctx)
	}
}

func (a *App) RestoreWindow() {
	a.bringWindowToFront()
	a.emit("app:window-restored")
}

func (a *App) ExitFromTray() error {
	return a.requestQuit()
}

func (a *App) requestQuit() error {
	if a.ctx == nil {
		return fmt.Errorf("application context is not ready")
	}
	a.authorizeQuitOnce()
	if a.quitApp != nil {
		a.quitApp(a.ctx)
	}
	return nil
}

func (a *App) authorizeQuitOnce() {
	a.lifecycleMu.Lock()
	a.quitAuthorized = true
	a.lifecycleMu.Unlock()
}

func (a *App) consumeQuitAuthorization() bool {
	a.lifecycleMu.Lock()
	defer a.lifecycleMu.Unlock()
	if a.quitAuthorized {
		a.quitAuthorized = false
		return true
	}
	return false
}

func (a *App) bringWindowToFront() {
	if a.ctx == nil {
		return
	}
	if a.unminimiseWindow != nil {
		a.unminimiseWindow(a.ctx)
	}
	if a.showWindow != nil {
		a.showWindow(a.ctx)
	}
	if a.showApp != nil {
		a.showApp(a.ctx)
	}
	if a.setWindowAlwaysTop != nil {
		a.setWindowAlwaysTop(a.ctx, true)
		go func(ctx context.Context, setAlwaysTop func(context.Context, bool)) {
			time.Sleep(250 * time.Millisecond)
			setAlwaysTop(ctx, false)
		}(a.ctx, a.setWindowAlwaysTop)
	}
}

func (a *App) emit(name string, data ...interface{}) {
	if a.ctx == nil || a.emitEvent == nil {
		return
	}
	a.emitEvent(a.ctx, name, data...)
}

func (a *App) trayState() (supported bool, available bool) {
	if a.tray == nil {
		return false, false
	}
	return a.tray.Supported(), a.tray.Available()
}

func (a *App) initTray(ctx context.Context) {
	if a.tray == nil {
		a.tray = tray.NewController()
	}
	if a.tray == nil || !a.tray.Supported() {
		return
	}
	err := a.tray.Start(ctx, tray.MenuCallbacks{
		OnReady: func() {
			a.syncTrayProxyState()
			supported, available := a.trayState()
			a.emit("app:tray-state-changed", map[string]any{
				"source":    "tray",
				"supported": supported,
				"available": available,
			})
		},
		OnOpen: func() {
			a.RestoreWindow()
		},
		OnToggleProxy: func() error {
			return a.ToggleProxyFromTray()
		},
		OnExit: func() {
			_ = a.ExitFromTray()
		},
		IsProxyRunning: func() bool {
			return a.proxy != nil && a.proxy.Running()
		},
	})
	if err != nil && a.log != nil {
		a.log.Warn("tray", "failed to initialize system tray: "+err.Error())
	}
}

func (a *App) syncTrayProxyState() {
	if a.tray == nil {
		return
	}
	a.tray.SetProxyRunning(a.proxy != nil && a.proxy.Running())
}

func toggleProxyByState(running bool, startFn func() error, stopFn func() error) error {
	if running {
		return stopFn()
	}
	return startFn()
}

func buildSecondLaunchNotice(data options.SecondInstanceData) SecondLaunchNotice {
	// Check if this is a Kiro protocol URL launch
	isKiroAuth := false
	for _, arg := range data.Args {
		if strings.HasPrefix(arg, "kiro://") {
			isKiroAuth = true
			break
		}
	}

	message := "CLIro-Go is already running. Restored the existing window."
	if isKiroAuth {
		message = "App Restored: Kiro account logging in, please wait..."
	}

	return SecondLaunchNotice{
		Message:          message,
		Args:             append([]string(nil), data.Args...),
		WorkingDirectory: strings.TrimSpace(data.WorkingDirectory),
		ReceivedAt:       time.Now().Unix(),
	}
}

func (a *App) handleKiroProtocolURL(rawURL string) {
	// Parse kiro:// URL to extract code and state
	// Expected format: kiro://kiro.kiroAgent/authenticate-success?code=xxx&state=yyy

	a.log.Info("app", "handling kiro:// protocol URL: "+rawURL)

	// Remove kiro:// prefix
	urlPath := strings.TrimPrefix(rawURL, "kiro://")

	// Find query string
	queryStart := strings.Index(urlPath, "?")
	if queryStart == -1 {
		a.log.Error("app", "kiro:// URL missing query string: "+rawURL)
		wruntime.EventsEmit(a.ctx, "app:notification", map[string]interface{}{
			"type":    "error",
			"title":   "Kiro Auth Failed",
			"message": "Invalid authorization URL format",
		})
		return
	}

	queryString := urlPath[queryStart+1:]
	a.log.Info("app", "query string: "+queryString)

	// Parse query parameters
	var code, state string
	for _, param := range strings.Split(queryString, "&") {
		parts := strings.SplitN(param, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := parts[0]
		value := parts[1]

		if key == "code" {
			code = value
		} else if key == "state" {
			state = value
		}
	}

	if code == "" {
		a.log.Error("app", "kiro:// URL missing code parameter: "+rawURL)
		wruntime.EventsEmit(a.ctx, "app:notification", map[string]interface{}{
			"type":    "error",
			"title":   "Kiro Auth Failed",
			"message": "Authorization code not found in URL",
		})
		return
	}

	a.log.Info("app", fmt.Sprintf("extracted code from kiro:// URL (code length=%d, state=%s)", len(code), state))

	// Find active Kiro auth session with matching state (if state provided)
	// For now, we'll try to submit to any pending social auth session
	sessions := a.auth.GetAllKiroAuthSessions()
	a.log.Info("app", fmt.Sprintf("found %d Kiro auth sessions", len(sessions)))

	var targetSessionID string

	for _, session := range sessions {
		a.log.Info("app", fmt.Sprintf("session %s: status=%s, authMethod=%s", session.SessionID, session.Status, session.AuthMethod))
		if session.Status == "pending" && session.AuthMethod == "social" {
			// If state matches, use this session
			// If no state in URL, use first pending social session
			targetSessionID = session.SessionID
			break
		}
	}

	if targetSessionID == "" {
		a.log.Error("app", "no pending Kiro social auth session found for code submission")
		wruntime.EventsEmit(a.ctx, "app:notification", map[string]interface{}{
			"type":    "error",
			"title":   "Kiro Auth Failed",
			"message": "No pending social auth session found. Please start Google/GitHub auth first.",
		})
		return
	}

	a.log.Info("app", "submitting code to session "+targetSessionID)

	// Submit the code
	if err := a.auth.SubmitKiroAuthCode(targetSessionID, code); err != nil {
		a.log.Error("app", "failed to submit Kiro auth code: "+err.Error())
		wruntime.EventsEmit(a.ctx, "app:notification", map[string]interface{}{
			"type":    "error",
			"title":   "Kiro Auth Failed",
			"message": err.Error(),
		})
		return
	}

	a.log.Info("app", "successfully submitted Kiro auth code from protocol URL")
}

func (a *App) onSecondInstanceLaunch(data options.SecondInstanceData) {
	if a.ctx == nil {
		return
	}
	notice := buildSecondLaunchNotice(data)
	a.log.Info("app", fmt.Sprintf("second instance launch received working_dir=%q args=%q", notice.WorkingDirectory, strings.Join(notice.Args, " ")))

	// Check if this is a custom protocol URL (kiro://)
	foundProtocolURL := false
	for _, arg := range data.Args {
		a.log.Info("app", "checking arg: "+arg)
		if strings.HasPrefix(arg, "kiro://") {
			a.log.Info("app", "detected kiro:// protocol URL: "+arg)
			foundProtocolURL = true
			go a.handleKiroProtocolURL(arg)
			// Still show window for user feedback
			break
		}
	}

	if !foundProtocolURL {
		a.log.Info("app", "no kiro:// protocol URL found in args")
	}

	a.bringWindowToFront()
	wruntime.EventsEmit(a.ctx, "app:second-instance", notice)
}
