package auth

import (
	"context"
	"fmt"
	"testing"

	"cliro/internal/config"
	"cliro/internal/logger"
)

type stubAuthProvider struct {
	refreshed config.Account
	err       error
}

func (s stubAuthProvider) StartAuth() (*AuthStart, error) { return nil, fmt.Errorf("unused") }
func (s stubAuthProvider) StartSocialAuth(string) (*AuthStart, error) {
	return nil, fmt.Errorf("unused")
}
func (s stubAuthProvider) GetSession(string) AuthSessionView { return AuthSessionView{} }
func (s stubAuthProvider) CancelSession(string)              {}
func (s stubAuthProvider) SubmitCode(string, string) error   { return nil }
func (s stubAuthProvider) RefreshAccount(account config.Account, force bool) (config.Account, error) {
	return s.refreshed, s.err
}
func (s stubAuthProvider) Shutdown(context.Context) error { return nil }

type stubQuotaRefresher struct {
	called int
	lastID string
	err    error
}

func (s *stubQuotaRefresher) RefreshQuotaOnly(accountID string) error {
	s.called++
	s.lastID = accountID
	return s.err
}

func TestRefreshAccountAlsoRefreshesQuota(t *testing.T) {
	store, err := config.NewManager(t.TempDir())
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	m := &Manager{store: store, providers: map[string]authProvider{}, client: nil}
	account := config.Account{ID: "acc_1", Provider: "codex", Email: "user@example.com", Enabled: true}
	if err := store.UpsertAccount(account); err != nil {
		t.Fatalf("upsert account: %v", err)
	}
	provider := stubAuthProvider{refreshed: config.Account{ID: "acc_1", Provider: "codex", Email: "user@example.com", Enabled: true, AccessToken: "new-token"}}
	refresher := &stubQuotaRefresher{}
	m.providers["codex"] = provider
	m.SetQuotaRefresher(refresher)

	refreshed, err := m.RefreshAccount("acc_1")
	if err != nil {
		t.Fatalf("RefreshAccount error: %v", err)
	}
	if refresher.called != 1 {
		t.Fatalf("quota refresher called %d times", refresher.called)
	}
	if refresher.lastID != "acc_1" {
		t.Fatalf("quota refresher account = %q", refresher.lastID)
	}
	if refreshed.ID != "acc_1" {
		t.Fatalf("refreshed account id = %q", refreshed.ID)
	}
}

func TestRefreshAccountReturnsQuotaErrorWhenQuotaRefreshFails(t *testing.T) {
	store, err := config.NewManager(t.TempDir())
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	m := &Manager{store: store, providers: map[string]authProvider{}, client: nil}
	account := config.Account{ID: "acc_1", Provider: "kiro", Email: "user@example.com", Enabled: true}
	if err := store.UpsertAccount(account); err != nil {
		t.Fatalf("upsert account: %v", err)
	}
	m.providers["kiro"] = stubAuthProvider{refreshed: account}
	refresher := &stubQuotaRefresher{err: fmt.Errorf("quota failed")}
	m.SetQuotaRefresher(refresher)

	_, err = m.RefreshAccount("acc_1")
	if err == nil || err.Error() != "quota failed" {
		t.Fatalf("expected quota failed error, got %v", err)
	}
}

func TestEnsureFreshAccountDoesNotTriggerQuotaRefresh(t *testing.T) {
	store, err := config.NewManager(t.TempDir())
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	m := &Manager{store: store, providers: map[string]authProvider{}, client: nil}
	account := config.Account{ID: "acc_1", Provider: "codex", Email: "user@example.com", Enabled: true}
	if err := store.UpsertAccount(account); err != nil {
		t.Fatalf("upsert account: %v", err)
	}
	m.providers["codex"] = stubAuthProvider{refreshed: account}
	refresher := &stubQuotaRefresher{}
	m.SetQuotaRefresher(refresher)

	_, err = m.EnsureFreshAccount("acc_1")
	if err != nil {
		t.Fatalf("EnsureFreshAccount error: %v", err)
	}
	if refresher.called != 0 {
		t.Fatalf("quota refresher called %d times", refresher.called)
	}
}

func TestNewManagerConstructionForCoverage(t *testing.T) {
	store, err := config.NewManager(t.TempDir())
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	if NewManager(store, logger.New(10)) == nil {
		t.Fatalf("expected manager")
	}
}
