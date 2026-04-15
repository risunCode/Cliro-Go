package auth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	authcodex "cliro/internal/auth/codex"
	authkiro "cliro/internal/auth/kiro"
	"cliro/internal/config"
	"cliro/internal/logger"
	syncauth "cliro/internal/sync/authtoken"
)

type Manager struct {
	store *config.Manager

	mu             sync.RWMutex
	client         *http.Client
	quotaRefresher quotaRefresher

	providers map[string]authProvider
}

type quotaRefresher interface {
	RefreshQuotaOnly(accountID string) error
}

type codexAuthProvider struct {
	service *authcodex.Service
}

type kiroAuthProvider struct {
	service *authkiro.Service
}

var (
	_ authProvider = (*codexAuthProvider)(nil)
	_ authProvider = (*kiroAuthProvider)(nil)
)

func NewManager(store *config.Manager, log *logger.Logger) *Manager {
	m := &Manager{
		store:  store,
		client: &http.Client{Timeout: 60 * time.Second},
	}

	codexSvc := authcodex.NewService(store, log, m.httpClient, m.refreshQuotaOnly)
	kiroSvc := authkiro.NewService(store, log, m.httpClient, m.refreshQuotaOnly)

	m.providers = map[string]authProvider{
		"codex": &codexAuthProvider{service: codexSvc},
		"kiro":  &kiroAuthProvider{service: kiroSvc},
	}

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
	for _, provider := range m.providers {
		if provider == nil {
			continue
		}
		if err := provider.Shutdown(ctx); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (m *Manager) StartAuth(provider string) (*AuthStart, error) {
	authProvider, err := m.providerFor(provider)
	if err != nil {
		return nil, err
	}
	return authProvider.StartAuth()
}

func (m *Manager) StartSocialAuth(provider string, socialProvider string) (*AuthStart, error) {
	authProvider, err := m.providerFor(provider)
	if err != nil {
		return nil, err
	}
	return authProvider.StartSocialAuth(socialProvider)
}

func (m *Manager) GetAuthSession(provider string, sessionID string) AuthSessionView {
	authProvider, err := m.providerFor(provider)
	if err != nil {
		return AuthSessionView{
			SessionID: sessionID,
			Status:    SessionError,
			Error:     err.Error(),
			Provider:  strings.ToLower(strings.TrimSpace(provider)),
		}
	}
	return authProvider.GetSession(sessionID)
}

func (m *Manager) GetAllKiroAuthSessions() []AuthSessionView {
	authProvider, err := m.providerFor("kiro")
	if err != nil {
		return nil
	}
	lister, ok := authProvider.(allSessionsProvider)
	if !ok {
		return nil
	}
	return lister.AllSessions()
}

func (m *Manager) CancelAuth(provider string, sessionID string) {
	authProvider, err := m.providerFor(provider)
	if err != nil {
		return
	}
	authProvider.CancelSession(sessionID)
}

func (m *Manager) SubmitAuthCode(provider string, sessionID string, code string) error {
	authProvider, err := m.providerFor(provider)
	if err != nil {
		return err
	}
	return authProvider.SubmitCode(sessionID, code)
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

func (m *Manager) SyncAccountAuth(accountID string, target AuthSyncTarget) (AuthSyncResult, error) {
	account, err := m.findCodexAccountForSync(accountID, syncTargetDisplayName(target))
	if err != nil {
		return AuthSyncResult{}, err
	}

	result := AuthSyncResult{Target: string(target)}

	switch target {
	case SyncTargetKilo:
		syncResult, syncErr := syncauth.SyncCodexAccountToKiloAuth(account)
		if syncErr != nil {
			return AuthSyncResult{}, syncErr
		}
		result.TargetPath = syncResult.TargetPath
		result.FileExisted = syncResult.FileExisted
		result.OpenAICreated = syncResult.OpenAICreated
		result.UpdatedFields = append([]string(nil), syncResult.UpdatedFields...)
		result.AccountID = syncResult.AccountID
		result.Provider = syncResult.Provider
		result.SyncedExpires = syncResult.SyncedExpires
		result.SyncedExpiresAt = syncResult.SyncedExpiresAt
		return result, nil
	case SyncTargetOpencode:
		syncResult, syncErr := syncauth.SyncCodexAccountToOpencodeAuth(account)
		if syncErr != nil {
			return AuthSyncResult{}, syncErr
		}
		result.TargetPath = syncResult.TargetPath
		result.FileExisted = syncResult.FileExisted
		result.OpenAICreated = syncResult.OpenAICreated
		result.UpdatedFields = append([]string(nil), syncResult.UpdatedFields...)
		result.AccountID = syncResult.AccountID
		result.Provider = syncResult.Provider
		result.SyncedExpires = syncResult.SyncedExpires
		result.SyncedExpiresAt = syncResult.SyncedExpiresAt
		return result, nil
	case SyncTargetCodexCLI:
		syncResult, syncErr := syncauth.SyncCodexAccountToCodexCLI(account)
		if syncErr != nil {
			return AuthSyncResult{}, syncErr
		}
		result.TargetPath = syncResult.TargetPath
		result.FileExisted = syncResult.FileExisted
		result.BackupPath = syncResult.BackupPath
		result.BackupCreated = syncResult.BackupCreated
		result.UpdatedFields = append([]string(nil), syncResult.UpdatedFields...)
		result.AccountID = syncResult.AccountID
		result.Provider = syncResult.Provider
		result.SyncedAt = syncResult.SyncedAt
		return result, nil
	default:
		return AuthSyncResult{}, fmt.Errorf("unsupported auth sync target: %s", target)
	}
}

func (m *Manager) refreshAccount(account config.Account, force bool) (config.Account, error) {
	authProvider, err := m.providerFor(account.Provider)
	if err != nil {
		verb := "ensure fresh account"
		if force {
			verb = "refresh account"
		}
		return account, fmt.Errorf("%s only supports provider codex or kiro", verb)
	}
	refreshed, err := authProvider.RefreshAccount(account, force)
	if err != nil {
		return refreshed, err
	}
	if !force {
		return refreshed, nil
	}
	if quotaErr := m.refreshQuotaOnly(refreshed.ID); quotaErr != nil {
		if updated, ok := m.store.GetAccount(refreshed.ID); ok {
			return updated, quotaErr
		}
		return refreshed, quotaErr
	}
	if updated, ok := m.store.GetAccount(refreshed.ID); ok {
		return updated, nil
	}
	return refreshed, nil
}

func (m *Manager) providerFor(name string) (authProvider, error) {
	provider := strings.ToLower(strings.TrimSpace(name))
	if provider == "" {
		return nil, fmt.Errorf("provider is required")
	}
	authProvider, ok := m.providers[provider]
	if !ok || authProvider == nil {
		return nil, fmt.Errorf("unsupported provider: %s", provider)
	}
	return authProvider, nil
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

// StartCodexRefreshLoop starts the background Codex token auto-refresh goroutine.
// Call this once after the app is ready to serve requests.
func (m *Manager) StartCodexRefreshLoop(ctx context.Context) {
	p, err := m.providerFor("codex")
	if err != nil {
		return
	}
	type refreshLoopStarter interface {
		StartRefreshLoop(ctx context.Context)
	}
	if starter, ok := p.(refreshLoopStarter); ok {
		starter.StartRefreshLoop(ctx)
	}
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

func syncTargetDisplayName(target AuthSyncTarget) string {
	switch target {
	case SyncTargetKilo:
		return "Kilo CLI auth.json"
	case SyncTargetOpencode:
		return "Opencode CLI auth.json"
	case SyncTargetCodexCLI:
		return "Codex CLI auth.json"
	default:
		return string(target)
	}
}

func (p *codexAuthProvider) StartAuth() (*AuthStart, error) {
	start, err := p.service.StartAuth()
	if err != nil {
		return nil, err
	}
	return &AuthStart{
		SessionID:   start.SessionID,
		AuthURL:     start.AuthURL,
		CallbackURL: start.CallbackURL,
		Status:      start.Status,
		Provider:    "codex",
	}, nil
}

func (p *codexAuthProvider) StartSocialAuth(_ string) (*AuthStart, error) {
	return nil, fmt.Errorf("social auth not supported for provider codex")
}

func (p *codexAuthProvider) GetSession(sessionID string) AuthSessionView {
	session := p.service.GetAuthSession(sessionID)
	return AuthSessionView{
		SessionID:   session.SessionID,
		AuthURL:     session.AuthURL,
		CallbackURL: session.CallbackURL,
		Status:      session.Status,
		Error:       session.Error,
		AccountID:   session.AccountID,
		Email:       session.Email,
		Provider:    "codex",
	}
}

func (p *codexAuthProvider) CancelSession(sessionID string) {
	p.service.CancelAuth(sessionID)
}

func (p *codexAuthProvider) SubmitCode(sessionID, code string) error {
	return p.service.SubmitAuthCode(sessionID, code)
}

func (p *codexAuthProvider) RefreshAccount(account config.Account, force bool) (config.Account, error) {
	return p.service.RefreshAccount(account, force)
}

func (p *codexAuthProvider) Shutdown(ctx context.Context) error {
	return p.service.Shutdown(ctx)
}

func (p *kiroAuthProvider) StartAuth() (*AuthStart, error) {
	start, err := p.service.StartAuth()
	if err != nil {
		return nil, err
	}
	return &AuthStart{
		SessionID:       start.SessionID,
		AuthURL:         start.AuthURL,
		VerificationURL: start.VerificationURL,
		UserCode:        start.UserCode,
		ExpiresAt:       start.ExpiresAt,
		Status:          start.Status,
		AuthMethod:      start.AuthMethod,
		Provider:        start.Provider,
	}, nil
}

func (p *kiroAuthProvider) StartSocialAuth(socialProvider string) (*AuthStart, error) {
	start, err := p.service.StartSocialAuth(socialProvider)
	if err != nil {
		return nil, err
	}
	return &AuthStart{
		SessionID:       start.SessionID,
		AuthURL:         start.AuthURL,
		VerificationURL: start.VerificationURL,
		UserCode:        start.UserCode,
		ExpiresAt:       start.ExpiresAt,
		Status:          start.Status,
		AuthMethod:      start.AuthMethod,
		Provider:        start.Provider,
	}, nil
}

func (p *kiroAuthProvider) GetSession(sessionID string) AuthSessionView {
	session := p.service.GetAuthSession(sessionID)
	return AuthSessionView{
		SessionID:       session.SessionID,
		AuthURL:         session.AuthURL,
		VerificationURL: session.VerificationURL,
		UserCode:        session.UserCode,
		ExpiresAt:       session.ExpiresAt,
		Status:          session.Status,
		Error:           session.Error,
		AccountID:       session.AccountID,
		Email:           session.Email,
		AuthMethod:      session.AuthMethod,
		Provider:        session.Provider,
	}
}

func (p *kiroAuthProvider) CancelSession(sessionID string) {
	p.service.CancelAuth(sessionID)
}

func (p *kiroAuthProvider) SubmitCode(sessionID, code string) error {
	return p.service.SubmitAuthCode(sessionID, code)
}

func (p *kiroAuthProvider) RefreshAccount(account config.Account, force bool) (config.Account, error) {
	return p.service.RefreshAccount(account, force)
}

func (p *kiroAuthProvider) Shutdown(ctx context.Context) error {
	return p.service.Shutdown(ctx)
}

func (p *kiroAuthProvider) AllSessions() []AuthSessionView {
	list := p.service.GetAllAuthSessions()
	result := make([]AuthSessionView, 0, len(list))
	for _, session := range list {
		result = append(result, AuthSessionView{
			SessionID:       session.SessionID,
			AuthURL:         session.AuthURL,
			VerificationURL: session.VerificationURL,
			UserCode:        session.UserCode,
			ExpiresAt:       session.ExpiresAt,
			Status:          session.Status,
			Error:           session.Error,
			AccountID:       session.AccountID,
			Email:           session.Email,
			AuthMethod:      session.AuthMethod,
			Provider:        session.Provider,
		})
	}
	return result
}
