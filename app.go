package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"cliro-go/internal/auth"
	"cliro-go/internal/config"
	"cliro-go/internal/logger"
	"cliro-go/internal/pool"
	"cliro-go/internal/proxy"

	"github.com/google/uuid"
	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

type App struct {
	ctx   context.Context
	store *config.Manager
	log   *logger.Logger
	auth  *auth.Manager
	pool  *pool.Pool
	proxy *proxy.Service
}

type State struct {
	AuthMode       string            `json:"authMode"`
	ProxyPort      int               `json:"proxyPort"`
	ProxyURL       string            `json:"proxyUrl"`
	ProxyBindAddr  string            `json:"proxyBindAddress"`
	AllowLAN       bool              `json:"allowLan"`
	AutoStartProxy bool              `json:"autoStartProxy"`
	ProxyRunning   bool              `json:"proxyRunning"`
	AvailableCount int               `json:"availableCount"`
	Accounts       []config.Account  `json:"accounts"`
	Stats          config.ProxyStats `json:"stats"`
}

func proxyBindHost(allowLAN bool) string {
	if allowLAN {
		return "0.0.0.0"
	}
	return "127.0.0.1"
}

func proxyURL(port int) string {
	return fmt.Sprintf("http://127.0.0.1:%d", port)
}

func proxyBindAddress(allowLAN bool, port int) string {
	return fmt.Sprintf("%s:%d", proxyBindHost(allowLAN), port)
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

	store, err := config.NewManager(dataDir)
	if err != nil {
		panic(err)
	}
	a.store = store
	a.pool = pool.New(store)
	a.auth = auth.NewManager(store, a.log)
	a.proxy = proxy.NewService(store, a.auth, a.pool, a.log)

	if store.AutoStartProxy() {
		if err := a.proxy.Start(store.ProxyPort(), store.AllowLAN()); err != nil {
			a.log.Error("app", "failed to auto-start proxy: "+err.Error())
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
	bindAddr := proxyBindAddress(allowLAN, port)
	if a.proxy != nil && a.proxy.Running() {
		if runningBindAddr := a.proxy.BindAddress(); runningBindAddr != "" {
			bindAddr = runningBindAddr
		}
	}
	accounts := a.store.Accounts()
	return State{
		AuthMode:       "oauth_callback",
		ProxyPort:      port,
		ProxyURL:       proxyURL(port),
		ProxyBindAddr:  bindAddr,
		AllowLAN:       allowLAN,
		AutoStartProxy: a.store.AutoStartProxy(),
		ProxyRunning:   a.proxy != nil && a.proxy.Running(),
		AvailableCount: a.pool.AvailableCount(),
		Accounts:       accounts,
		Stats:          snap.Stats,
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
	bindAddr := proxyBindAddress(allowLAN, port)
	if a.proxy != nil && a.proxy.Running() {
		if runningBindAddr := a.proxy.BindAddress(); runningBindAddr != "" {
			bindAddr = runningBindAddr
		}
	}
	return map[string]any{
		"running":        a.proxy != nil && a.proxy.Running(),
		"port":           port,
		"url":            proxyURL(port),
		"bindAddress":    bindAddr,
		"allowLan":       allowLAN,
		"autoStartProxy": a.store.AutoStartProxy(),
	}
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

func (a *App) RefreshAccount(accountID string) error {
	_, err := a.auth.RefreshAccount(accountID)
	return err
}

func (a *App) RefreshQuota(accountID string) error {
	_, err := a.auth.RefreshQuota(accountID)
	return err
}

func (a *App) RefreshAllQuotas() error {
	return a.auth.RefreshAllQuotas()
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
			account.Provider = "codex"
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

func (a *App) StartProxy() error {
	a.log.Info("proxy", "starting proxy service")
	return a.proxy.Start(a.store.ProxyPort(), a.store.AllowLAN())
}

func (a *App) StopProxy() error {
	a.log.Info("proxy", "stopping proxy service")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return a.proxy.Stop(ctx)
}

func (a *App) SetProxyPort(port int) error {
	if a.store == nil {
		return fmt.Errorf("store is not ready")
	}
	running := a.proxy != nil && a.proxy.Running()
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
		return a.proxy.Start(a.store.ProxyPort(), a.store.AllowLAN())
	}
	return nil
}

func (a *App) SetAllowLAN(enabled bool) error {
	if a.store == nil {
		return fmt.Errorf("store is not ready")
	}
	running := a.proxy != nil && a.proxy.Running()
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
		return a.proxy.Start(a.store.ProxyPort(), enabled)
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
