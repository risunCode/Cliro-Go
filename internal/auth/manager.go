package auth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	authcodex "cliro-go/internal/auth/codex"
	authkiro "cliro-go/internal/auth/kiro"
	"cliro-go/internal/config"
	"cliro-go/internal/logger"
	syncauth "cliro-go/internal/sync/authtoken"
)

type Manager struct {
	store *config.Manager

	mu             sync.RWMutex
	client         *http.Client
	quotaRefresher quotaRefresher

	codex *authcodex.Service
	kiro  *authkiro.Service
}

type quotaRefresher interface {
	RefreshQuotaOnly(accountID string) error
}

func NewManager(store *config.Manager, log *logger.Logger) *Manager {
	m := &Manager{
		store:  store,
		client: &http.Client{Timeout: 60 * time.Second},
	}
	m.codex = authcodex.NewService(store, log, m.httpClient, m.refreshQuotaOnly)
	m.kiro = authkiro.NewService(store, log, m.httpClient, m.refreshQuotaOnly)
	return m
}

func (m *Manager) httpClient() *http.Client {
	m.mu.RLock()
	client := m.client
	m.mu.RUnlock()
	if client != nil {
		return client
	}
	return &http.Client{Timeout: 60 * time.Second}
}

func (m *Manager) HTTPClient() *http.Client {
	return m.httpClient()
}

func (m *Manager) SetQuotaRefresher(refresher quotaRefresher) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.quotaRefresher = refresher
}

func (m *Manager) SetHTTPTimeout(timeout time.Duration) {
	if timeout <= 0 {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	m.client = &http.Client{Timeout: timeout}
}

func (m *Manager) SetHTTPClient(client *http.Client) {
	if client == nil {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	m.client = client
}

func (m *Manager) Shutdown(ctx context.Context) error {
	var errs []error
	if m.codex != nil {
		if err := m.codex.Shutdown(ctx); err != nil {
			errs = append(errs, err)
		}
	}
	if m.kiro != nil {
		if err := m.kiro.Shutdown(ctx); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (m *Manager) StartCodexAuth() (*CodexAuthStart, error) {
	return m.codex.StartAuth()
}

func (m *Manager) GetCodexAuthSession(sessionID string) CodexAuthSessionView {
	return m.codex.GetAuthSession(sessionID)
}

func (m *Manager) CancelCodexAuth(sessionID string) {
	m.codex.CancelAuth(sessionID)
}

func (m *Manager) SubmitCodexAuthCode(sessionID string, code string) error {
	return m.codex.SubmitAuthCode(sessionID, code)
}

func (m *Manager) StartKiroAuth() (*KiroAuthStart, error) {
	return m.kiro.StartAuth()
}

func (m *Manager) StartKiroSocialAuth(provider string) (*KiroAuthStart, error) {
	return m.kiro.StartSocialAuth(provider)
}

func (m *Manager) GetKiroAuthSession(sessionID string) KiroAuthSessionView {
	return m.kiro.GetAuthSession(sessionID)
}

func (m *Manager) GetAllKiroAuthSessions() []KiroAuthSessionView {
	return m.kiro.GetAllAuthSessions()
}

func (m *Manager) CancelKiroAuth(sessionID string) {
	m.kiro.CancelAuth(sessionID)
}

func (m *Manager) SubmitKiroAuthCode(sessionID string, code string) error {
	return m.kiro.SubmitAuthCode(sessionID, code)
}

func (m *Manager) RefreshAccount(accountID string) (config.Account, error) {
	account, ok := m.store.GetAccount(accountID)
	if !ok {
		return config.Account{}, fmt.Errorf("account not found")
	}
	return m.refreshAccount(account, true)
}

func (m *Manager) EnsureFreshAccount(accountID string) (config.Account, error) {
	account, ok := m.store.GetAccount(accountID)
	if !ok {
		return config.Account{}, fmt.Errorf("account not found")
	}
	return m.refreshAccount(account, false)
}

func (m *Manager) SyncCodexAccountToKiloAuth(accountID string) (KiloAuthSyncResult, error) {
	account, err := m.findCodexAccountForSync(accountID, "Kilo CLI auth.json")
	if err != nil {
		return KiloAuthSyncResult{}, err
	}
	return syncauth.SyncCodexAccountToKiloAuth(account)
}

func (m *Manager) SyncCodexAccountToOpencodeAuth(accountID string) (OpencodeAuthSyncResult, error) {
	account, err := m.findCodexAccountForSync(accountID, "Opencode CLI auth.json")
	if err != nil {
		return OpencodeAuthSyncResult{}, err
	}
	return syncauth.SyncCodexAccountToOpencodeAuth(account)
}

func (m *Manager) SyncCodexAccountToCodexCLI(accountID string) (CodexAuthSyncResult, error) {
	account, err := m.findCodexAccountForSync(accountID, "Codex CLI auth.json")
	if err != nil {
		return CodexAuthSyncResult{}, err
	}
	return syncauth.SyncCodexAccountToCodexCLI(account)
}

func AccountFingerprint(account config.Account) string {
	return authcodex.AccountFingerprint(account)
}

func (m *Manager) refreshAccount(account config.Account, force bool) (config.Account, error) {
	switch strings.ToLower(strings.TrimSpace(account.Provider)) {
	case "codex":
		return m.codex.RefreshAccount(account, force)
	case "kiro":
		return m.kiro.RefreshAccount(account, force)
	default:
		verb := "ensure fresh account"
		if force {
			verb = "refresh account"
		}
		return account, fmt.Errorf("%s only supports provider codex or kiro", verb)
	}
}

func (m *Manager) findCodexAccountForSync(accountID, targetName string) (config.Account, error) {
	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		return config.Account{}, fmt.Errorf("account id is required")
	}

	account, ok := m.store.GetAccount(accountID)
	if !ok {
		return config.Account{}, fmt.Errorf("account not found: %s", accountID)
	}

	provider := strings.TrimSpace(strings.ToLower(account.Provider))
	if provider == "" {
		return config.Account{}, fmt.Errorf("account provider is required for sync to %s", targetName)
	}
	if provider != "codex" {
		return config.Account{}, fmt.Errorf("sync to %s only supports provider codex", targetName)
	}

	return account, nil
}

func (m *Manager) refreshQuotaOnly(accountID string) error {
	m.mu.RLock()
	refresher := m.quotaRefresher
	m.mu.RUnlock()
	if refresher == nil {
		return nil
	}
	return refresher.RefreshQuotaOnly(accountID)
}
