package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"syscall"
	"time"

	"cliro-go/internal/config"
	"cliro-go/internal/logger"

	"github.com/google/uuid"
)

const (
	clientID            = "app_EMoamEEZ73f0CkXaXp7hrann"
	oauthAuthorizeURL   = "https://auth.openai.com/oauth/authorize"
	oauthTokenURL       = "https://auth.openai.com/oauth/token"
	oauthRedirectURI    = "http://localhost:1455/auth/callback"
	oauthCallbackAddr   = "127.0.0.1:1455"
	defaultOAuthTimeout = 15 * time.Minute
	refreshSkew         = 5 * time.Minute
	codexVersion        = "0.117.0"
	codexUserAgent      = "codex_cli_rs/0.117.0 (Windows NT 10.0; Win64; x64)"
)

type SessionStatus string

const (
	SessionPending SessionStatus = "pending"
	SessionSuccess SessionStatus = "success"
	SessionError   SessionStatus = "error"
)

type CodexAuthStart struct {
	SessionID   string `json:"sessionId"`
	AuthURL     string `json:"authUrl"`
	CallbackURL string `json:"callbackUrl"`
	Status      string `json:"status"`
}

type CodexAuthSessionView struct {
	SessionID   string        `json:"sessionId"`
	AuthURL     string        `json:"authUrl"`
	CallbackURL string        `json:"callbackUrl"`
	Status      SessionStatus `json:"status"`
	Error       string        `json:"error,omitempty"`
	AccountID   string        `json:"accountId,omitempty"`
	Email       string        `json:"email,omitempty"`
}

type Manager struct {
	store          *config.Manager
	log            *logger.Logger
	client         *http.Client
	mu             sync.RWMutex
	oauthSessions  map[string]*oauthSession
	callbackServer *http.Server
}

type oauthSession struct {
	CodexAuthSessionView
	state        string
	codeVerifier string
	createdAt    time.Time
	ctx          context.Context
	cancel       context.CancelFunc
}

type tokenExchangeResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	IDToken      string `json:"id_token"`
	ExpiresIn    int    `json:"expires_in"`
	TokenType    string `json:"token_type"`
}

type jwtClaims struct {
	Email         string `json:"email"`
	Exp           int64  `json:"exp"`
	CodexAuthInfo struct {
		ChatgptAccountID string `json:"chatgpt_account_id"`
		ChatgptPlanType  string `json:"chatgpt_plan_type"`
	} `json:"https://api.openai.com/auth"`
}

func NewManager(store *config.Manager, log *logger.Logger) *Manager {
	return &Manager{
		store:         store,
		log:           log,
		client:        &http.Client{Timeout: 60 * time.Second},
		oauthSessions: map[string]*oauthSession{},
	}
}

func (m *Manager) Shutdown(ctx context.Context) error {
	m.mu.Lock()
	for _, session := range m.oauthSessions {
		if session.cancel != nil {
			session.cancel()
		}
	}
	server := m.callbackServer
	m.callbackServer = nil
	m.mu.Unlock()
	if server == nil {
		return nil
	}
	return server.Shutdown(ctx)
}

func (m *Manager) StartCodexAuth() (*CodexAuthStart, error) {
	if err := m.ensureCallbackServer(); err != nil {
		return nil, err
	}

	verifier, err := generateCodeVerifier()
	if err != nil {
		return nil, err
	}
	sessionID := uuid.NewString()
	ctx, cancel := context.WithTimeout(context.Background(), defaultOAuthTimeout)
	authURL := buildCodexAuthURL(sessionID, codeChallenge(verifier))
	session := &oauthSession{
		CodexAuthSessionView: CodexAuthSessionView{
			SessionID:   sessionID,
			AuthURL:     authURL,
			CallbackURL: oauthRedirectURI,
			Status:      SessionPending,
		},
		state:        sessionID,
		codeVerifier: verifier,
		createdAt:    time.Now(),
		ctx:          ctx,
		cancel:       cancel,
	}

	m.mu.Lock()
	m.oauthSessions[sessionID] = session
	m.mu.Unlock()

	go m.expireOAuthSession(sessionID, ctx)
	m.log.Info("auth", "started Codex OAuth session "+sessionID)

	return &CodexAuthStart{
		SessionID:   sessionID,
		AuthURL:     authURL,
		CallbackURL: oauthRedirectURI,
		Status:      string(SessionPending),
	}, nil
}

func (m *Manager) GetCodexAuthSession(sessionID string) CodexAuthSessionView {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if session, ok := m.oauthSessions[sessionID]; ok {
		return session.CodexAuthSessionView
	}
	return CodexAuthSessionView{SessionID: sessionID, Status: SessionError, Error: "session not found"}
}

func (m *Manager) CancelCodexAuth(sessionID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if session, ok := m.oauthSessions[sessionID]; ok {
		session.Status = SessionError
		session.Error = "session cancelled"
		if session.cancel != nil {
			session.cancel()
			session.cancel = nil
		}
	}
}

func (m *Manager) expireOAuthSession(sessionID string, ctx context.Context) {
	<-ctx.Done()
	m.mu.Lock()
	defer m.mu.Unlock()
	session, ok := m.oauthSessions[sessionID]
	if !ok || session.Status != SessionPending {
		return
	}
	session.Status = SessionError
	session.Error = "oauth session expired"
	if session.cancel != nil {
		session.cancel = nil
	}
	m.log.Warn("auth", "Codex OAuth session expired: "+sessionID)
}

func generateCodeVerifier() (string, error) {
	raw := make([]byte, 48)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func codeChallenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func buildCodexAuthURL(state, challenge string) string {
	params := url.Values{
		"client_id":                  {clientID},
		"response_type":              {"code"},
		"redirect_uri":               {oauthRedirectURI},
		"scope":                      {"openid email profile offline_access"},
		"state":                      {state},
		"code_challenge":             {challenge},
		"code_challenge_method":      {"S256"},
		"prompt":                     {"login"},
		"id_token_add_organizations": {"true"},
		"codex_cli_simplified_flow":  {"true"},
	}
	return oauthAuthorizeURL + "?" + params.Encode()
}

func renderOAuthCallbackPage(title, message string) string {
	return fmt.Sprintf(`<!doctype html>
<html>
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>%s</title>
  <style>
    :root { color-scheme: dark; }
    body { margin: 0; min-height: 100vh; display: grid; place-items: center; background: #0d0d10; color: #f5f7fb; font-family: Segoe UI, sans-serif; }
    .card { width: min(30rem, calc(100vw - 2rem)); border: 1px solid rgba(255,255,255,0.08); border-radius: 20px; padding: 1.5rem; background: linear-gradient(180deg, rgba(255,255,255,0.05), rgba(255,255,255,0.03)); box-shadow: 0 30px 80px rgba(0,0,0,0.35); }
    .eyebrow { margin: 0 0 0.5rem; color: #9aa3b2; font-size: 0.76rem; text-transform: uppercase; letter-spacing: 0.22em; }
    h1 { margin: 0; font-size: 1.35rem; }
    p { margin: 0.85rem 0 0; color: #c6ccd6; line-height: 1.5; }
  </style>
</head>
<body>
  <div class="card">
    <p class="eyebrow">Cliro-Go</p>
    <h1>%s</h1>
    <p>%s</p>
  </div>
</body>
</html>`, title, title, message)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func (m *Manager) exchangeAuthorizationCodeWithRedirect(ctx context.Context, code, redirectURI, codeVerifier string) (*tokenExchangeResponse, error) {
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"client_id":     {clientID},
		"code":          {strings.TrimSpace(code)},
		"redirect_uri":  {strings.TrimSpace(redirectURI)},
		"code_verifier": {strings.TrimSpace(codeVerifier)},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, oauthTokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := m.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("token exchange failed (%d): %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	var parsed tokenExchangeResponse
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil, err
	}
	return &parsed, nil
}

func (m *Manager) ensureCallbackServer() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.callbackServer != nil {
		return nil
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/auth/callback", m.handleOAuthCallback)
	mux.HandleFunc("/auth/health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"status":"ok"}`)
	})
	listener, err := net.Listen("tcp", oauthCallbackAddr)
	if err != nil {
		if errors.Is(err, syscall.EADDRINUSE) || strings.Contains(strings.ToLower(err.Error()), "address already in use") {
			return fmt.Errorf("oauth callback server failed to bind %s: port 1455 is already in use; stop the conflicting process and retry", oauthCallbackAddr)
		}
		return fmt.Errorf("oauth callback server failed to bind %s: %w", oauthCallbackAddr, err)
	}

	server := &http.Server{Addr: oauthCallbackAddr, Handler: mux}
	m.callbackServer = server
	go func(srv *http.Server, ln net.Listener) {
		err := srv.Serve(ln)
		if err != nil && err != http.ErrServerClosed {
			m.log.Error("auth", "oauth callback server stopped: "+err.Error())
			m.mu.Lock()
			if m.callbackServer == srv {
				m.callbackServer = nil
			}
			m.mu.Unlock()
		}
	}(server, listener)
	m.log.Info("auth", "oauth callback server listening on http://"+oauthCallbackAddr)
	return nil
}

func (m *Manager) handleOAuthCallback(w http.ResponseWriter, r *http.Request) {
	state := strings.TrimSpace(r.URL.Query().Get("state"))
	code := strings.TrimSpace(r.URL.Query().Get("code"))
	callbackErr := strings.TrimSpace(r.URL.Query().Get("error"))
	callbackErrDesc := strings.TrimSpace(r.URL.Query().Get("error_description"))

	if state == "" {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, renderOAuthCallbackPage("Authentication failed", "Missing OAuth state parameter."))
		return
	}

	m.mu.RLock()
	session := m.oauthSessions[state]
	m.mu.RUnlock()
	if session == nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, renderOAuthCallbackPage("Authentication failed", "Session was not found or already expired."))
		return
	}

	if callbackErr != "" {
		message := firstNonEmpty(callbackErrDesc, callbackErr)
		m.finishOAuthSessionError(state, fmt.Errorf(message))
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, renderOAuthCallbackPage("Authentication failed", message))
		return
	}
	if code == "" {
		m.finishOAuthSessionError(state, fmt.Errorf("authorization code missing from callback"))
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, renderOAuthCallbackPage("Authentication failed", "Authorization code missing from callback."))
		return
	}

	tokens, err := m.exchangeAuthorizationCodeWithRedirect(session.ctx, code, oauthRedirectURI, session.codeVerifier)
	if err != nil {
		m.finishOAuthSessionError(state, err)
		w.WriteHeader(http.StatusBadGateway)
		_, _ = io.WriteString(w, renderOAuthCallbackPage("Authentication failed", err.Error()))
		return
	}

	account, err := m.accountFromTokens(tokens)
	if err != nil {
		m.finishOAuthSessionError(state, err)
		w.WriteHeader(http.StatusBadGateway)
		_, _ = io.WriteString(w, renderOAuthCallbackPage("Authentication failed", err.Error()))
		return
	}
	if err := m.upsertCodexAccount(&account); err != nil {
		m.finishOAuthSessionError(state, err)
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(w, renderOAuthCallbackPage("Authentication failed", err.Error()))
		return
	}

	if _, err := m.RefreshQuota(account.ID); err != nil {
		m.log.Warn("quota", "initial quota refresh failed for "+account.Email+": "+err.Error())
	}

	m.mu.Lock()
	if current, ok := m.oauthSessions[state]; ok {
		current.Status = SessionSuccess
		current.Email = account.Email
		current.AccountID = account.ID
		if current.cancel != nil {
			current.cancel()
			current.cancel = nil
		}
	}
	m.mu.Unlock()
	m.log.Info("auth", "Codex OAuth completed for "+account.Email)

	_, _ = io.WriteString(w, renderOAuthCallbackPage("Authentication complete", "Account connected successfully. Close this window."))
}

func (m *Manager) finishOAuthSessionError(sessionID string, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if session, ok := m.oauthSessions[sessionID]; ok {
		session.Status = SessionError
		session.Error = err.Error()
		if session.cancel != nil {
			session.cancel()
			session.cancel = nil
		}
	}
	m.log.Error("auth", "Codex OAuth failed: "+err.Error())
}

func (m *Manager) accountFromTokens(tokens *tokenExchangeResponse) (config.Account, error) {
	claims, err := parseIDToken(tokens.IDToken)
	if err != nil {
		return config.Account{}, err
	}
	account := config.Account{
		ID:           uuid.NewString(),
		Provider:     "codex",
		Email:        claims.Email,
		AccountID:    claims.CodexAuthInfo.ChatgptAccountID,
		PlanType:     strings.TrimSpace(claims.CodexAuthInfo.ChatgptPlanType),
		AccessToken:  tokens.AccessToken,
		RefreshToken: tokens.RefreshToken,
		IDToken:      tokens.IDToken,
		ExpiresAt:    time.Now().Add(time.Duration(tokens.ExpiresIn) * time.Second).Unix(),
		Enabled:      true,
		CreatedAt:    time.Now().Unix(),
		UpdatedAt:    time.Now().Unix(),
		LastRefresh:  time.Now().Unix(),
	}
	if claims.Exp > 0 {
		account.ExpiresAt = claims.Exp
	}
	return account, nil
}

func (m *Manager) upsertCodexAccount(account *config.Account) error {
	for _, existing := range m.store.Accounts() {
		if existing.Provider != "" && existing.Provider != "codex" {
			continue
		}
		matchAccountID := existing.AccountID != "" && account.AccountID != "" && existing.AccountID == account.AccountID
		sameEmail := strings.EqualFold(strings.TrimSpace(existing.Email), strings.TrimSpace(account.Email)) && strings.TrimSpace(account.Email) != ""
		if !matchAccountID && !sameEmail {
			continue
		}
		account.ID = existing.ID
		account.CreatedAt = existing.CreatedAt
		account.RequestCount = existing.RequestCount
		account.ErrorCount = existing.ErrorCount
		account.PromptTokens = existing.PromptTokens
		account.CompletionTokens = existing.CompletionTokens
		account.TotalTokens = existing.TotalTokens
		account.LastUsed = existing.LastUsed
		account.CooldownUntil = existing.CooldownUntil
		account.LastError = existing.LastError
		if account.Quota.Status == "" {
			account.Quota = existing.Quota
		}
		break
	}
	return m.store.UpsertAccount(*account)
}

func (m *Manager) RefreshAccount(accountID string) (config.Account, error) {
	account, ok := m.store.GetAccount(accountID)
	if !ok {
		return config.Account{}, fmt.Errorf("account not found")
	}
	if strings.TrimSpace(account.RefreshToken) == "" {
		return account, fmt.Errorf("account has no refresh token")
	}
	tokens, err := m.refreshTokens(context.Background(), account.RefreshToken)
	if err != nil {
		if blockedMsg, blocked := blockedAccountMessageFromAuthError(err); blocked {
			_ = m.store.UpdateAccount(accountID, func(a *config.Account) {
				a.Enabled = false
				a.LastError = blockedMsg
			})
		}
		m.log.Error("auth", "refresh failed for "+account.Email+": "+err.Error())
		return account, err
	}
	claims, err := parseIDToken(tokens.IDToken)
	if err != nil {
		return account, err
	}
	err = m.store.UpdateAccount(accountID, func(a *config.Account) {
		a.Provider = "codex"
		a.AccessToken = tokens.AccessToken
		if strings.TrimSpace(tokens.RefreshToken) != "" {
			a.RefreshToken = tokens.RefreshToken
		}
		a.IDToken = tokens.IDToken
		a.ExpiresAt = time.Now().Add(time.Duration(tokens.ExpiresIn) * time.Second).Unix()
		if claims.Exp > 0 {
			a.ExpiresAt = claims.Exp
		}
		a.Email = claims.Email
		a.AccountID = claims.CodexAuthInfo.ChatgptAccountID
		a.PlanType = strings.TrimSpace(claims.CodexAuthInfo.ChatgptPlanType)
		a.LastRefresh = time.Now().Unix()
		a.LastError = ""
	})
	if err != nil {
		return account, err
	}
	refreshed, _ := m.store.GetAccount(accountID)
	m.log.Info("auth", "refreshed token for "+refreshed.Email)
	return refreshed, nil
}

func blockedAccountMessageFromAuthError(err error) (string, bool) {
	if err == nil {
		return "", false
	}

	message := strings.TrimSpace(err.Error())
	if message == "" {
		return "", false
	}

	normalized := strings.ToLower(message)
	blockIndicators := []string{
		"deactivated",
		"banned",
		"suspended",
		"forbidden",
		"disabled by",
		"terminated",
		"closed",
	}

	for _, indicator := range blockIndicators {
		if strings.Contains(normalized, indicator) {
			return message, true
		}
	}

	return "", false
}

func (m *Manager) EnsureFreshAccount(accountID string) (config.Account, error) {
	account, ok := m.store.GetAccount(accountID)
	if !ok {
		return config.Account{}, fmt.Errorf("account not found")
	}
	if account.ExpiresAt == 0 || time.Now().Unix() < account.ExpiresAt-int64(refreshSkew.Seconds()) {
		return account, nil
	}
	return m.RefreshAccount(accountID)
}

func (m *Manager) refreshTokens(ctx context.Context, refreshToken string) (*tokenExchangeResponse, error) {
	form := url.Values{
		"client_id":     {clientID},
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
		"scope":         {"openid profile email offline_access"},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, oauthTokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := m.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("refresh token failed (%d): %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	var parsed tokenExchangeResponse
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil, err
	}
	return &parsed, nil
}

func parseIDToken(token string) (*jwtClaims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid id token")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, err
	}
	var claims jwtClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, err
	}
	return &claims, nil
}

func AccountFingerprint(account config.Account) string {
	base := strings.TrimSpace(account.AccountID)
	if base == "" {
		base = strings.TrimSpace(account.Email)
	}
	sum := sha256.Sum256([]byte(base))
	return base64.RawURLEncoding.EncodeToString(sum[:6])
}
