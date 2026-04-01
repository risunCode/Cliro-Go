package codex

import (
	"cliro-go/internal/util"
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
	"syscall"
	"time"

	"cliro-go/internal/config"
)

const (
	ClientID            = "app_EMoamEEZ73f0CkXaXp7hrann"
	OAuthAuthorizeURL   = "https://auth.openai.com/oauth/authorize"
	OAuthTokenURL       = "https://auth.openai.com/oauth/token"
	OAuthRedirectURI    = "http://localhost:1455/auth/callback"
	OAuthCallbackAddr   = "127.0.0.1:1455"
	DefaultOAuthTimeout = 15 * time.Minute
	Version             = "0.117.0"

	sessionPending = "pending"
	sessionSuccess = "success"
	sessionError   = "error"
)

var (
	errEmptyAuthorizationCode = fmt.Errorf("authorization code is empty")
	errSessionNotFound        = fmt.Errorf("session not found")
	errSessionNotPending      = fmt.Errorf("session is not pending")
)

type TokenExchangeResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	IDToken      string `json:"id_token"`
	ExpiresIn    int    `json:"expires_in"`
	TokenType    string `json:"token_type"`
}

type JWTClaims struct {
	Email         string `json:"email"`
	Exp           int64  `json:"exp"`
	CodexAuthInfo struct {
		ChatgptAccountID string `json:"chatgpt_account_id"`
		ChatgptPlanType  string `json:"chatgpt_plan_type"`
	} `json:"https://api.openai.com/auth"`
}

type AuthStart struct {
	SessionID   string `json:"sessionId"`
	AuthURL     string `json:"authUrl"`
	CallbackURL string `json:"callbackUrl"`
	Status      string `json:"status"`
}

type AuthSessionView struct {
	SessionID   string `json:"sessionId"`
	AuthURL     string `json:"authUrl"`
	CallbackURL string `json:"callbackUrl"`
	Status      string `json:"status"`
	Error       string `json:"error,omitempty"`
	AccountID   string `json:"accountId,omitempty"`
	Email       string `json:"email,omitempty"`
}

type oauthSession struct {
	AuthSessionView
	state        string
	codeVerifier string
	createdAt    time.Time
	ctx          context.Context
	cancel       context.CancelFunc
}

func GenerateCodeVerifier() (string, error) {
	raw := make([]byte, 48)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func CodeChallenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func BuildAuthURL(state, challenge string) string {
	params := url.Values{
		"client_id":                  {ClientID},
		"response_type":              {"code"},
		"redirect_uri":               {OAuthRedirectURI},
		"scope":                      {"openid email profile offline_access"},
		"state":                      {state},
		"code_challenge":             {challenge},
		"code_challenge_method":      {"S256"},
		"prompt":                     {"login"},
		"id_token_add_organizations": {"true"},
		"codex_cli_simplified_flow":  {"true"},
		"originator":                 {"opencode"},
	}
	return OAuthAuthorizeURL + "?" + params.Encode()
}

func RenderCallbackPage(title, message string) string {
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

func ParseIDToken(token string) (*JWTClaims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid id token")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, err
	}
	var claims JWTClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, err
	}
	return &claims, nil
}

func (s *Service) expireAuthSession(sessionID string, ctx context.Context) {
	<-ctx.Done()

	s.mu.Lock()
	defer s.mu.Unlock()

	session, ok := s.oauthSessions[sessionID]
	if !ok || session.Status != sessionPending {
		return
	}

	session.Status = sessionError
	session.Error = "oauth session expired"
	if session.cancel != nil {
		session.cancel = nil
	}
	s.log.Warn("auth", "Codex OAuth session expired: "+sessionID)
}

func (s *Service) exchangeAuthorizationCodeWithRedirect(ctx context.Context, code, redirectURI, codeVerifier string) (*TokenExchangeResponse, error) {
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"client_id":     {ClientID},
		"code":          {strings.TrimSpace(code)},
		"redirect_uri":  {strings.TrimSpace(redirectURI)},
		"code_verifier": {strings.TrimSpace(codeVerifier)},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, OAuthTokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := s.client().Do(req)
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

	var parsed TokenExchangeResponse
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil, err
	}
	return &parsed, nil
}

func (s *Service) ensureCallbackServer() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.callbackServer != nil {
		return nil
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/auth/callback", s.handleOAuthCallback)
	mux.HandleFunc("/auth/health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"status":"ok"}`)
	})

	listener, err := net.Listen("tcp", OAuthCallbackAddr)
	if err != nil {
		if errors.Is(err, syscall.EADDRINUSE) || strings.Contains(strings.ToLower(err.Error()), "address already in use") {
			return fmt.Errorf("oauth callback server failed to bind %s: port 1455 is already in use; stop the conflicting process and retry", OAuthCallbackAddr)
		}
		return fmt.Errorf("oauth callback server failed to bind %s: %w", OAuthCallbackAddr, err)
	}

	server := &http.Server{Addr: OAuthCallbackAddr, Handler: mux}
	s.callbackServer = server
	go func(srv *http.Server, ln net.Listener) {
		err := srv.Serve(ln)
		if err != nil && err != http.ErrServerClosed {
			s.log.Error("auth", "oauth callback server stopped: "+err.Error())
			s.mu.Lock()
			if s.callbackServer == srv {
				s.callbackServer = nil
			}
			s.mu.Unlock()
		}
	}(server, listener)

	s.log.Info("auth", "oauth callback server listening on http://"+OAuthCallbackAddr)
	return nil
}

func (s *Service) handleOAuthCallback(w http.ResponseWriter, r *http.Request) {
	state := strings.TrimSpace(r.URL.Query().Get("state"))
	code := strings.TrimSpace(r.URL.Query().Get("code"))
	callbackErr := strings.TrimSpace(r.URL.Query().Get("error"))
	callbackErrDesc := strings.TrimSpace(r.URL.Query().Get("error_description"))

	if state == "" {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, RenderCallbackPage("Authentication failed", "Missing OAuth state parameter."))
		return
	}

	s.mu.RLock()
	session := s.oauthSessions[state]
	s.mu.RUnlock()
	if session == nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, RenderCallbackPage("Authentication failed", "Session was not found or already expired."))
		return
	}

	if callbackErr != "" {
		message := util.FirstNonEmpty(callbackErrDesc, callbackErr)
		s.finishAuthError(state, fmt.Errorf(message))
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, RenderCallbackPage("Authentication failed", message))
		return
	}
	if code == "" {
		s.finishAuthError(state, fmt.Errorf("authorization code missing from callback"))
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, RenderCallbackPage("Authentication failed", "Authorization code missing from callback."))
		return
	}

	tokens, err := s.exchangeAuthorizationCodeWithRedirect(session.ctx, code, OAuthRedirectURI, session.codeVerifier)
	if err != nil {
		s.finishAuthError(state, err)
		w.WriteHeader(http.StatusBadGateway)
		_, _ = io.WriteString(w, RenderCallbackPage("Authentication failed", err.Error()))
		return
	}

	account, err := s.accountFromTokens(tokens)
	if err != nil {
		s.finishAuthError(state, err)
		w.WriteHeader(http.StatusBadGateway)
		_, _ = io.WriteString(w, RenderCallbackPage("Authentication failed", err.Error()))
		return
	}
	if err := s.upsertAccount(&account); err != nil {
		s.finishAuthError(state, err)
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(w, RenderCallbackPage("Authentication failed", err.Error()))
		return
	}

	s.refreshNewAccountQuota(account.ID, "initial quota refresh failed for "+account.Email)
	s.finishAuthSuccess(state, account)
	s.log.Info("auth", "Codex OAuth completed for "+account.Email)

	_, _ = io.WriteString(w, RenderCallbackPage("Authentication complete", "Account connected successfully. Close this window."))
}

func (s *Service) finishAuthSuccess(sessionID string, account config.Account) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if current, ok := s.oauthSessions[sessionID]; ok {
		current.Status = sessionSuccess
		current.Email = account.Email
		current.AccountID = account.ID
		if current.cancel != nil {
			current.cancel()
			current.cancel = nil
		}
	}
}

func (s *Service) finishAuthError(sessionID string, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if session, ok := s.oauthSessions[sessionID]; ok {
		session.Status = sessionError
		session.Error = err.Error()
		if session.cancel != nil {
			session.cancel()
			session.cancel = nil
		}
	}
	s.log.Error("auth", "Codex OAuth failed: "+err.Error())
}

