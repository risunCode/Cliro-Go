package auth

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"cliro-go/internal/config"

	"github.com/google/uuid"
)

const (
	kiroRegisterClientURL   = "https://oidc.us-east-1.amazonaws.com/client/register"
	kiroDeviceAuthURL       = "https://oidc.us-east-1.amazonaws.com/device_authorization"
	kiroDeviceTokenURL      = "https://oidc.us-east-1.amazonaws.com/token"
	kiroBuilderStartURL     = "https://view.awsapps.com/start"
	kiroBuilderClientName   = "kiro-oauth-client"
	kiroDefaultDeviceWait   = 15 * time.Minute
	kiroMinimumPollInterval = 5
	kiroSocialAuthTimeout   = 10 * time.Minute
	kiroSocialCallbackPort  = 9876
)

const (
	kiroRuntimeUserAgent    = "aws-sdk-js/1.0.27 ua/2.1 os/linux lang/js md/nodejs#22.21.1 api/codewhispererstreaming#1.0.27 m/E KiroIDE-0.10.32"
	kiroRuntimeAmzUserAgent = "aws-sdk-js/1.0.27 KiroIDE 0.10.32"
	kiroSocialAuthURL       = "https://prod.us-east-1.auth.desktop.kiro.dev"
)

var kiroBuilderScopes = []string{
	"codewhisperer:completions",
	"codewhisperer:analysis",
	"codewhisperer:conversations",
	"codewhisperer:transformations",
	"codewhisperer:taskassist",
}

type KiroAuthStart struct {
	SessionID       string `json:"sessionId"`
	AuthURL         string `json:"authUrl"`
	VerificationURL string `json:"verificationUrl,omitempty"`
	UserCode        string `json:"userCode"`
	ExpiresAt       int64  `json:"expiresAt,omitempty"`
	Status          string `json:"status"`
	AuthMethod      string `json:"authMethod,omitempty"`
	Provider        string `json:"provider,omitempty"`
}

type KiroAuthSessionView struct {
	SessionID       string        `json:"sessionId"`
	AuthURL         string        `json:"authUrl"`
	VerificationURL string        `json:"verificationUrl,omitempty"`
	UserCode        string        `json:"userCode,omitempty"`
	ExpiresAt       int64         `json:"expiresAt,omitempty"`
	Status          SessionStatus `json:"status"`
	Error           string        `json:"error,omitempty"`
	AccountID       string        `json:"accountId,omitempty"`
	Email           string        `json:"email,omitempty"`
	AuthMethod      string        `json:"authMethod,omitempty"`
	Provider        string        `json:"provider,omitempty"`
}

type kiroAuthSession struct {
	KiroAuthSessionView
	deviceCode   string
	interval     int
	clientID     string
	clientSecret string
	state        string
	codeVerifier string
	redirectURI  string
	callbackCh   <-chan kiroSocialCallbackResult
	createdAt    time.Time
	ctx          context.Context
	cancel       context.CancelFunc
}

type kiroSocialCallbackResult struct {
	Code  string
	State string
	Error string
}

type kiroSocialProvider string

const (
	kiroSocialProviderGoogle kiroSocialProvider = "Google"
	kiroSocialProviderGitHub kiroSocialProvider = "Github"
)

type kiroClientRegistrationResponse struct {
	ClientID              string `json:"clientId"`
	ClientSecret          string `json:"clientSecret"`
	ClientSecretExpiresAt int64  `json:"clientSecretExpiresAt"`
}

type kiroDeviceAuthorizationResponse struct {
	DeviceCode              string `json:"deviceCode"`
	UserCode                string `json:"userCode"`
	VerificationURI         string `json:"verificationUri"`
	VerificationURIComplete string `json:"verificationUriComplete"`
	ExpiresIn               int    `json:"expiresIn"`
	Interval                int    `json:"interval"`
}

type kiroDeviceTokenResponse struct {
	AccessToken      string `json:"accessToken"`
	RefreshToken     string `json:"refreshToken"`
	ExpiresIn        int    `json:"expiresIn"`
	TokenType        string `json:"tokenType"`
	ProfileARN       string `json:"profileArn"`
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
}

type kiroTokenData struct {
	AccessToken  string
	RefreshToken string
	ExpiresIn    int
	TokenType    string
	ProfileARN   string
	Email        string
	ClientID     string
	ClientSecret string
}

type kiroSocialTokenResponse struct {
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken"`
	ProfileARN   string `json:"profileArn"`
	ExpiresIn    int    `json:"expiresIn"`
}

func (m *Manager) StartKiroAuth() (*KiroAuthStart, error) {
	client, err := m.registerKiroClient(context.Background())
	if err != nil {
		m.log.Error("auth", "Kiro client registration failed: "+err.Error())
		return nil, err
	}

	device, err := m.startKiroDeviceAuthorization(context.Background(), client.ClientID, client.ClientSecret)
	if err != nil {
		m.log.Error("auth", "Kiro device authorization failed: "+err.Error())
		return nil, err
	}

	authURL := firstNonEmpty(device.VerificationURIComplete, device.VerificationURI)
	if strings.TrimSpace(authURL) == "" {
		return nil, fmt.Errorf("kiro device authorization did not return a verification URL")
	}

	expiresIn := device.ExpiresIn
	if expiresIn <= 0 {
		expiresIn = int(kiroDefaultDeviceWait / time.Second)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(expiresIn)*time.Second)

	interval := device.Interval
	if interval < kiroMinimumPollInterval {
		interval = kiroMinimumPollInterval
	}

	sessionID := uuid.NewString()
	session := &kiroAuthSession{
		KiroAuthSessionView: KiroAuthSessionView{
			SessionID:       sessionID,
			AuthURL:         strings.TrimSpace(authURL),
			VerificationURL: strings.TrimSpace(device.VerificationURI),
			UserCode:        strings.TrimSpace(device.UserCode),
			ExpiresAt:       time.Now().Add(time.Duration(expiresIn) * time.Second).Unix(),
			Status:          SessionPending,
		},
		deviceCode:   strings.TrimSpace(device.DeviceCode),
		interval:     interval,
		clientID:     strings.TrimSpace(client.ClientID),
		clientSecret: strings.TrimSpace(client.ClientSecret),
		createdAt:    time.Now(),
		ctx:          ctx,
		cancel:       cancel,
	}

	m.mu.Lock()
	if m.kiroSessions == nil {
		m.kiroSessions = map[string]*kiroAuthSession{}
	}
	m.kiroSessions[sessionID] = session
	m.mu.Unlock()

	go m.expireKiroAuthSession(sessionID, ctx)
	go m.completeKiroAuthSession(sessionID)

	m.log.Info("auth", "started Kiro device auth session "+sessionID)

	return &KiroAuthStart{
		SessionID:       sessionID,
		AuthURL:         session.AuthURL,
		VerificationURL: session.VerificationURL,
		UserCode:        session.UserCode,
		ExpiresAt:       session.ExpiresAt,
		Status:          string(SessionPending),
		AuthMethod:      "device",
		Provider:        "aws_builder_id",
	}, nil
}

func (m *Manager) StartKiroSocialAuth(provider string) (*KiroAuthStart, error) {
	resolvedProvider, err := normalizeKiroSocialProvider(provider)
	if err != nil {
		return nil, err
	}

	codeVerifier, codeChallenge, err := generateKiroPKCE()
	if err != nil {
		return nil, err
	}
	state, err := generateKiroState()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), kiroSocialAuthTimeout)
	redirectURI, callbackCh, err := m.startKiroSocialCallbackServer(ctx, state)
	if err != nil {
		cancel()
		return nil, err
	}

	authURL := buildKiroSocialLoginURL(resolvedProvider, redirectURI, codeChallenge, state)
	sessionID := uuid.NewString()
	session := &kiroAuthSession{
		KiroAuthSessionView: KiroAuthSessionView{
			SessionID:  sessionID,
			AuthURL:    authURL,
			ExpiresAt:  time.Now().Add(kiroSocialAuthTimeout).Unix(),
			Status:     SessionPending,
			AuthMethod: "social",
			Provider:   strings.ToLower(string(resolvedProvider)),
		},
		state:        state,
		codeVerifier: codeVerifier,
		redirectURI:  redirectURI,
		callbackCh:   callbackCh,
		createdAt:    time.Now(),
		ctx:          ctx,
		cancel:       cancel,
	}

	m.mu.Lock()
	if m.kiroSessions == nil {
		m.kiroSessions = map[string]*kiroAuthSession{}
	}
	m.kiroSessions[sessionID] = session
	m.mu.Unlock()

	go m.expireKiroAuthSession(sessionID, ctx)
	go m.completeKiroSocialAuthSession(sessionID)

	m.log.Info("auth", "started Kiro social auth session "+sessionID+" provider="+string(resolvedProvider))

	return &KiroAuthStart{
		SessionID:  sessionID,
		AuthURL:    authURL,
		ExpiresAt:  session.ExpiresAt,
		Status:     string(SessionPending),
		AuthMethod: "social",
		Provider:   strings.ToLower(string(resolvedProvider)),
	}, nil
}

func (m *Manager) GetKiroAuthSession(sessionID string) KiroAuthSessionView {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if session, ok := m.kiroSessions[sessionID]; ok {
		return session.KiroAuthSessionView
	}
	return KiroAuthSessionView{SessionID: sessionID, Status: SessionError, Error: "session not found"}
}

func (m *Manager) CancelKiroAuth(sessionID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if session, ok := m.kiroSessions[sessionID]; ok {
		session.Status = SessionError
		session.Error = "session cancelled"
		if session.cancel != nil {
			session.cancel()
			session.cancel = nil
		}
	}
}

func (m *Manager) completeKiroAuthSession(sessionID string) {
	session, ok := m.kiroSessionSnapshot(sessionID)
	if !ok {
		return
	}

	tokens, err := m.pollKiroDeviceToken(session.ctx, session.clientID, session.clientSecret, session.deviceCode, session.interval)
	if err != nil {
		m.finishKiroSessionError(sessionID, err)
		return
	}

	account, err := m.accountFromKiroToken(tokens)
	if err != nil {
		m.finishKiroSessionError(sessionID, err)
		return
	}
	if err := m.upsertKiroAccount(&account); err != nil {
		m.finishKiroSessionError(sessionID, err)
		return
	}

	m.finishKiroSessionSuccess(sessionID, account)
}

func (m *Manager) completeKiroSocialAuthSession(sessionID string) {
	session, ok := m.kiroSessionSnapshot(sessionID)
	if !ok {
		return
	}

	select {
	case <-session.ctx.Done():
		if session.ctx.Err() != nil && session.ctx.Err() != context.Canceled {
			m.finishKiroSessionError(sessionID, session.ctx.Err())
		}
		return
	case callback, ok := <-session.callbackCh:
		if !ok {
			m.finishKiroSessionError(sessionID, fmt.Errorf("kiro social auth callback closed before completion"))
			return
		}
		if strings.TrimSpace(callback.Error) != "" {
			m.finishKiroSessionError(sessionID, fmt.Errorf(callback.Error))
			return
		}
		if strings.TrimSpace(callback.Code) == "" {
			m.finishKiroSessionError(sessionID, fmt.Errorf("kiro social auth did not return an authorization code"))
			return
		}
		if strings.TrimSpace(callback.State) != strings.TrimSpace(session.state) {
			m.finishKiroSessionError(sessionID, fmt.Errorf("kiro social auth state mismatch"))
			return
		}

		tokens, err := m.exchangeKiroSocialCode(session.ctx, callback.Code, session.codeVerifier, session.redirectURI)
		if err != nil {
			m.finishKiroSessionError(sessionID, err)
			return
		}

		account, err := m.accountFromKiroToken(tokens)
		if err != nil {
			m.finishKiroSessionError(sessionID, err)
			return
		}
		if err := m.upsertKiroAccount(&account); err != nil {
			m.finishKiroSessionError(sessionID, err)
			return
		}
		m.finishKiroSessionSuccess(sessionID, account)
	}
}

func (m *Manager) kiroSessionSnapshot(sessionID string) (kiroAuthSession, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	session, ok := m.kiroSessions[sessionID]
	if !ok || session == nil {
		return kiroAuthSession{}, false
	}
	return *session, true
}

func normalizeKiroSocialProvider(provider string) (kiroSocialProvider, error) {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "google", "":
		return kiroSocialProviderGoogle, nil
	case "github":
		return kiroSocialProviderGitHub, nil
	default:
		return "", fmt.Errorf("unsupported Kiro social provider: %s", strings.TrimSpace(provider))
	}
}

func generateKiroPKCE() (string, string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", "", err
	}
	verifier := base64.RawURLEncoding.EncodeToString(raw)
	hashed := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(hashed[:])
	return verifier, challenge, nil
}

func generateKiroState() (string, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func buildKiroSocialLoginURL(provider kiroSocialProvider, redirectURI string, codeChallenge string, state string) string {
	return fmt.Sprintf("%s/login?idp=%s&redirect_uri=%s&code_challenge=%s&code_challenge_method=S256&state=%s&prompt=select_account",
		kiroSocialAuthURL,
		url.QueryEscape(string(provider)),
		url.QueryEscape(strings.TrimSpace(redirectURI)),
		url.QueryEscape(strings.TrimSpace(codeChallenge)),
		url.QueryEscape(strings.TrimSpace(state)),
	)
}

func (m *Manager) startKiroSocialCallbackServer(ctx context.Context, expectedState string) (string, <-chan kiroSocialCallbackResult, error) {
	listener, err := net.Listen("tcp", fmt.Sprintf("localhost:%d", kiroSocialCallbackPort))
	if err != nil {
		listener, err = net.Listen("tcp", "localhost:0")
		if err != nil {
			return "", nil, fmt.Errorf("failed to start Kiro social callback server: %w", err)
		}
	}

	port := listener.Addr().(*net.TCPAddr).Port
	redirectURI := fmt.Sprintf("http://localhost:%d/oauth/callback", port)
	resultCh := make(chan kiroSocialCallbackResult, 1)
	server := &http.Server{ReadHeaderTimeout: 10 * time.Second}
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth/callback", func(w http.ResponseWriter, r *http.Request) {
		code := strings.TrimSpace(r.URL.Query().Get("code"))
		state := strings.TrimSpace(r.URL.Query().Get("state"))
		errParam := strings.TrimSpace(r.URL.Query().Get("error"))

		writeResult := func(result kiroSocialCallbackResult, status int, title string, body string) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(status)
			_, _ = fmt.Fprintf(w, "<!DOCTYPE html><html><head><title>%s</title></head><body><h1>%s</h1><p>%s</p><p>You can close this window.</p></body></html>", html.EscapeString(title), html.EscapeString(title), html.EscapeString(body))
			select {
			case resultCh <- result:
			default:
			}
			go func() {
				_ = server.Shutdown(context.Background())
			}()
		}

		if errParam != "" {
			writeResult(kiroSocialCallbackResult{Error: errParam}, http.StatusBadRequest, "Login Failed", errParam)
			return
		}
		if state != strings.TrimSpace(expectedState) {
			writeResult(kiroSocialCallbackResult{Error: "state mismatch"}, http.StatusBadRequest, "Login Failed", "Invalid state parameter")
			return
		}
		writeResult(kiroSocialCallbackResult{Code: code, State: state}, http.StatusOK, "Login Successful", "Return to CLIro-Go to finish connecting your Kiro account.")
	})
	server.Handler = mux

	go func() {
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			select {
			case resultCh <- kiroSocialCallbackResult{Error: err.Error()}:
			default:
			}
		}
	}()

	go func() {
		<-ctx.Done()
		_ = server.Shutdown(context.Background())
	}()

	return redirectURI, resultCh, nil
}

func buildKiroSocialUserAgent() string {
	return fmt.Sprintf("KiroIDE-0.10.32-%s", strings.ReplaceAll(uuid.NewString(), "-", ""))
}

func (m *Manager) exchangeKiroSocialCode(ctx context.Context, code string, codeVerifier string, redirectURI string) (*kiroTokenData, error) {
	payload := map[string]string{
		"code":          strings.TrimSpace(code),
		"code_verifier": strings.TrimSpace(codeVerifier),
		"redirect_uri":  strings.TrimSpace(redirectURI),
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, kiroSocialAuthURL+"/oauth/token", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("User-Agent", buildKiroSocialUserAgent())

	resp, err := m.httpClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("kiro social token exchange failed (%d): %s", resp.StatusCode, compactHTTPBody(respBody))
	}

	var parsed kiroSocialTokenResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return nil, err
	}
	token := &kiroTokenData{
		AccessToken:  strings.TrimSpace(parsed.AccessToken),
		RefreshToken: strings.TrimSpace(parsed.RefreshToken),
		ProfileARN:   strings.TrimSpace(parsed.ProfileARN),
		ExpiresIn:    parsed.ExpiresIn,
		TokenType:    "Bearer",
	}
	if token.ExpiresIn <= 0 {
		token.ExpiresIn = 3600
	}
	token.Email = extractKiroEmailFromJWT(token.AccessToken)
	if strings.TrimSpace(token.ProfileARN) == "" {
		profileARN, profileEmail, profileErr := m.fetchKiroProfile(ctx, token.AccessToken)
		if profileErr == nil {
			token.ProfileARN = firstNonEmpty(strings.TrimSpace(token.ProfileARN), strings.TrimSpace(profileARN))
			token.Email = firstNonEmpty(strings.TrimSpace(token.Email), strings.TrimSpace(profileEmail))
		}
	}
	if strings.TrimSpace(token.Email) == "" {
		if email, emailErr := m.fetchKiroUserEmailWithToken(ctx, token.AccessToken); emailErr == nil {
			token.Email = strings.TrimSpace(email)
		}
	}
	return token, nil
}

func (m *Manager) expireKiroAuthSession(sessionID string, ctx context.Context) {
	<-ctx.Done()

	m.mu.Lock()
	defer m.mu.Unlock()

	session, ok := m.kiroSessions[sessionID]
	if !ok || session.Status != SessionPending {
		return
	}

	session.Status = SessionError
	session.Error = "kiro device auth session expired"
	if session.cancel != nil {
		session.cancel = nil
	}
	m.log.Warn("auth", "Kiro auth session expired: "+sessionID)
}

func (m *Manager) finishKiroSessionSuccess(sessionID string, account config.Account) {
	m.mu.Lock()
	defer m.mu.Unlock()

	session, ok := m.kiroSessions[sessionID]
	if !ok || session.Status != SessionPending {
		return
	}

	session.Status = SessionSuccess
	session.Email = strings.TrimSpace(account.Email)
	session.AccountID = strings.TrimSpace(account.ID)
	if session.cancel != nil {
		session.cancel()
		session.cancel = nil
	}
	m.log.Info("auth", "Kiro auth completed for "+firstNonEmpty(account.Email, account.ID))
}

func (m *Manager) finishKiroSessionError(sessionID string, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	session, ok := m.kiroSessions[sessionID]
	if !ok || session.Status != SessionPending {
		return
	}

	session.Status = SessionError
	session.Error = err.Error()
	if session.cancel != nil {
		session.cancel()
		session.cancel = nil
	}
	m.log.Error("auth", "Kiro auth failed: "+err.Error())
}

func (m *Manager) accountFromKiroToken(tokens *kiroTokenData) (config.Account, error) {
	if tokens == nil {
		return config.Account{}, fmt.Errorf("kiro auth returned empty token payload")
	}
	if strings.TrimSpace(tokens.AccessToken) == "" {
		return config.Account{}, fmt.Errorf("kiro auth returned empty access token")
	}
	if strings.TrimSpace(tokens.RefreshToken) == "" {
		return config.Account{}, fmt.Errorf("kiro auth returned empty refresh token")
	}
	if tokens.ExpiresIn <= 0 {
		tokens.ExpiresIn = 3600
	}

	now := time.Now()
	account := config.Account{
		ID:           uuid.NewString(),
		Provider:     "kiro",
		Email:        strings.TrimSpace(tokens.Email),
		AccountID:    strings.TrimSpace(tokens.ProfileARN),
		AccessToken:  strings.TrimSpace(tokens.AccessToken),
		RefreshToken: strings.TrimSpace(tokens.RefreshToken),
		ClientID:     strings.TrimSpace(tokens.ClientID),
		ClientSecret: strings.TrimSpace(tokens.ClientSecret),
		ExpiresAt:    now.Add(time.Duration(tokens.ExpiresIn) * time.Second).Unix(),
		Enabled:      true,
		CreatedAt:    now.Unix(),
		UpdatedAt:    now.Unix(),
		LastRefresh:  now.Unix(),
	}

	if strings.TrimSpace(account.Email) == "" {
		account.Email = "kiro-" + account.ID[:8]
	}

	return account, nil
}

func (m *Manager) upsertKiroAccount(account *config.Account) error {
	for _, existing := range m.store.Accounts() {
		if !strings.EqualFold(strings.TrimSpace(existing.Provider), "kiro") {
			continue
		}

		sameEmail := strings.EqualFold(strings.TrimSpace(existing.Email), strings.TrimSpace(account.Email)) && strings.TrimSpace(account.Email) != ""
		if !sameEmail {
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
		account.Banned = existing.Banned
		account.BannedReason = existing.BannedReason
		if strings.TrimSpace(account.AccountID) == "" {
			account.AccountID = existing.AccountID
		}
		if strings.TrimSpace(account.ClientID) == "" {
			account.ClientID = existing.ClientID
		}
		if strings.TrimSpace(account.ClientSecret) == "" {
			account.ClientSecret = existing.ClientSecret
		}
		if account.Quota.Status == "" {
			account.Quota = existing.Quota
		}
		break
	}

	return m.store.UpsertAccount(*account)
}

func (m *Manager) registerKiroClient(ctx context.Context) (*kiroClientRegistrationResponse, error) {
	payload := map[string]any{
		"clientName": kiroBuilderClientName,
		"clientType": "public",
		"scopes":     kiroBuilderScopes,
		"grantTypes": []string{"urn:ietf:params:oauth:grant-type:device_code", "refresh_token"},
		"issuerUrl":  kiroBuilderStartURL,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, kiroRegisterClientURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	applyKiroOIDCHeaders(req)

	resp, err := m.httpClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("kiro client registration failed (%d): %s", resp.StatusCode, compactHTTPBody(respBody))
	}

	var parsed kiroClientRegistrationResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return nil, err
	}
	if strings.TrimSpace(parsed.ClientID) == "" || strings.TrimSpace(parsed.ClientSecret) == "" {
		return nil, fmt.Errorf("kiro client registration returned incomplete credentials")
	}
	return &parsed, nil
}

func (m *Manager) startKiroDeviceAuthorization(ctx context.Context, clientID, clientSecret string) (*kiroDeviceAuthorizationResponse, error) {
	payload := map[string]string{
		"clientId":     strings.TrimSpace(clientID),
		"clientSecret": strings.TrimSpace(clientSecret),
		"startUrl":     kiroBuilderStartURL,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, kiroDeviceAuthURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	applyKiroOIDCHeaders(req)

	resp, err := m.httpClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("kiro device authorization failed (%d): %s", resp.StatusCode, compactHTTPBody(respBody))
	}

	var parsed kiroDeviceAuthorizationResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return nil, err
	}
	if strings.TrimSpace(parsed.DeviceCode) == "" || strings.TrimSpace(parsed.UserCode) == "" {
		return nil, fmt.Errorf("kiro device authorization returned incomplete device code data")
	}
	return &parsed, nil
}

func (m *Manager) pollKiroDeviceToken(ctx context.Context, clientID, clientSecret, deviceCode string, intervalSeconds int) (*kiroTokenData, error) {
	if intervalSeconds < kiroMinimumPollInterval {
		intervalSeconds = kiroMinimumPollInterval
	}

	deadline := time.Now().Add(kiroDefaultDeviceWait)
	for {
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("kiro device authorization timed out")
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(time.Duration(intervalSeconds) * time.Second):
		}

		payload := map[string]string{
			"clientId":     strings.TrimSpace(clientID),
			"clientSecret": strings.TrimSpace(clientSecret),
			"deviceCode":   strings.TrimSpace(deviceCode),
			"grantType":    "urn:ietf:params:oauth:grant-type:device_code",
		}
		body, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, kiroDeviceTokenURL, bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		applyKiroOIDCHeaders(req)

		resp, err := m.httpClient().Do(req)
		if err != nil {
			return nil, err
		}

		respBody, readErr := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if readErr != nil {
			return nil, readErr
		}

		var parsed kiroDeviceTokenResponse
		if err := json.Unmarshal(respBody, &parsed); err != nil {
			if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				return nil, err
			}
			return nil, fmt.Errorf("kiro device token request failed (%d): %s", resp.StatusCode, compactHTTPBody(respBody))
		}

		if resp.StatusCode < 200 || resp.StatusCode >= 300 || strings.TrimSpace(parsed.Error) != "" {
			switch strings.TrimSpace(parsed.Error) {
			case "authorization_pending":
				continue
			case "slow_down":
				intervalSeconds += kiroMinimumPollInterval
				continue
			case "expired_token":
				return nil, fmt.Errorf("kiro device code expired")
			case "access_denied":
				return nil, fmt.Errorf("kiro device authorization denied")
			}

			message := strings.TrimSpace(parsed.ErrorDescription)
			if message == "" {
				message = compactHTTPBody(respBody)
			}
			return nil, fmt.Errorf("kiro device token request failed (%d): %s", resp.StatusCode, message)
		}

		token := &kiroTokenData{
			AccessToken:  strings.TrimSpace(parsed.AccessToken),
			RefreshToken: strings.TrimSpace(parsed.RefreshToken),
			ExpiresIn:    parsed.ExpiresIn,
			TokenType:    firstNonEmpty(strings.TrimSpace(parsed.TokenType), "Bearer"),
			ProfileARN:   strings.TrimSpace(parsed.ProfileARN),
			ClientID:     strings.TrimSpace(clientID),
			ClientSecret: strings.TrimSpace(clientSecret),
		}
		token.Email = extractKiroEmailFromJWT(token.AccessToken)
		if strings.TrimSpace(token.ProfileARN) == "" {
			profileARN, profileEmail, profileErr := m.fetchKiroProfile(ctx, token.AccessToken)
			if profileErr == nil {
				token.ProfileARN = firstNonEmpty(strings.TrimSpace(token.ProfileARN), strings.TrimSpace(profileARN))
				token.Email = firstNonEmpty(strings.TrimSpace(token.Email), strings.TrimSpace(profileEmail))
			}
		}
		if strings.TrimSpace(token.Email) == "" {
			if email, emailErr := m.fetchKiroUserEmailWithToken(ctx, token.AccessToken); emailErr == nil {
				token.Email = strings.TrimSpace(email)
			}
		}
		return token, nil
	}
}

func applyKiroRuntimeHeaders(req *http.Request, accessToken string) {
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(accessToken))
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", kiroRuntimeUserAgent)
	req.Header.Set("x-amz-user-agent", kiroRuntimeAmzUserAgent)
	req.Header.Set("x-amzn-codewhisperer-optout", "true")
}

func applyKiroOIDCHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("x-amz-user-agent", "aws-sdk-js/1.0.27 KiroIDE")
	req.Header.Set("User-Agent", "aws-sdk-js/1.0.27 ua/2.1 os/linux lang/js md/nodejs#22.21.1 api/sso-oidc#1.0.27 m/E KiroIDE")
	req.Header.Set("amz-sdk-invocation-id", uuid.NewString())
	req.Header.Set("amz-sdk-request", "attempt=1; max=4")
}

func (m *Manager) fetchKiroProfile(ctx context.Context, accessToken string) (string, string, error) {
	body := []byte(`{"origin":"AI_EDITOR"}`)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, kiroQuotaBaseURL, bytes.NewReader(body))
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Content-Type", "application/x-amz-json-1.0")
	req.Header.Set("X-Amz-Target", "AmazonCodeWhispererService.ListAvailableModels")
	applyKiroRuntimeHeaders(req, accessToken)

	resp, err := m.httpClient().Do(req)
	if err != nil {
		return "", "", err
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", "", fmt.Errorf("kiro profile fetch failed (%d): %s", resp.StatusCode, compactHTTPBody(respBody))
	}

	var data struct {
		ProfileARN string `json:"profileArn"`
	}
	if err := json.Unmarshal(respBody, &data); err != nil {
		return "", "", err
	}
	return strings.TrimSpace(data.ProfileARN), extractKiroEmailFromJWT(accessToken), nil
}

func (m *Manager) fetchKiroUserEmailWithToken(ctx context.Context, accessToken string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, kiroQuotaBaseURL+"/GetUserInfo", strings.NewReader(`{"origin":"KIRO_IDE"}`))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	applyKiroRuntimeHeaders(req, accessToken)

	resp, err := m.httpClient().Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("kiro GetUserInfo failed (%d): %s", resp.StatusCode, compactHTTPBody(respBody))
	}

	var data struct {
		Email string `json:"email"`
	}
	if err := json.Unmarshal(respBody, &data); err != nil {
		return "", err
	}
	return strings.TrimSpace(data.Email), nil
}

func (m *Manager) refreshKiroTokens(ctx context.Context, clientID, clientSecret, refreshToken string) (*kiroTokenData, error) {
	if strings.TrimSpace(clientID) == "" || strings.TrimSpace(clientSecret) == "" {
		return m.refreshKiroSocialToken(ctx, refreshToken)
	}

	payload := map[string]string{
		"clientId":     strings.TrimSpace(clientID),
		"clientSecret": strings.TrimSpace(clientSecret),
		"refreshToken": strings.TrimSpace(refreshToken),
		"grantType":    "refresh_token",
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, kiroDeviceTokenURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	applyKiroOIDCHeaders(req)

	resp, err := m.httpClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("kiro token refresh failed (%d): %s", resp.StatusCode, compactHTTPBody(respBody))
	}

	var parsed kiroDeviceTokenResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return nil, err
	}
	if strings.TrimSpace(parsed.AccessToken) == "" {
		return nil, fmt.Errorf("kiro token refresh returned empty access token")
	}

	token := &kiroTokenData{
		AccessToken:  strings.TrimSpace(parsed.AccessToken),
		RefreshToken: firstNonEmpty(strings.TrimSpace(parsed.RefreshToken), strings.TrimSpace(refreshToken)),
		ExpiresIn:    parsed.ExpiresIn,
		TokenType:    firstNonEmpty(strings.TrimSpace(parsed.TokenType), "Bearer"),
		ProfileARN:   strings.TrimSpace(parsed.ProfileARN),
		Email:        extractKiroEmailFromJWT(parsed.AccessToken),
		ClientID:     strings.TrimSpace(clientID),
		ClientSecret: strings.TrimSpace(clientSecret),
	}
	if token.ExpiresIn <= 0 {
		token.ExpiresIn = 3600
	}
	if strings.TrimSpace(token.ProfileARN) == "" {
		profileARN, profileEmail, profileErr := m.fetchKiroProfile(ctx, token.AccessToken)
		if profileErr == nil {
			token.ProfileARN = firstNonEmpty(strings.TrimSpace(token.ProfileARN), strings.TrimSpace(profileARN))
			token.Email = firstNonEmpty(strings.TrimSpace(token.Email), strings.TrimSpace(profileEmail))
		}
	}
	if strings.TrimSpace(token.Email) == "" {
		if email, emailErr := m.fetchKiroUserEmailWithToken(ctx, token.AccessToken); emailErr == nil {
			token.Email = strings.TrimSpace(email)
		}
	}
	return token, nil
}

func (m *Manager) refreshKiroSocialToken(ctx context.Context, refreshToken string) (*kiroTokenData, error) {
	trimmedRefreshToken := strings.TrimSpace(refreshToken)
	if trimmedRefreshToken == "" {
		return nil, fmt.Errorf("kiro social refresh token is empty")
	}

	payload := map[string]string{"refreshToken": trimmedRefreshToken}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, kiroSocialAuthURL+"/refreshToken", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", buildKiroSocialUserAgent())

	resp, err := m.httpClient().Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("kiro social token refresh failed (%d): %s", resp.StatusCode, compactHTTPBody(respBody))
	}

	var parsed struct {
		AccessToken  string `json:"accessToken"`
		RefreshToken string `json:"refreshToken"`
		ProfileARN   string `json:"profileArn"`
		ExpiresIn    int    `json:"expiresIn"`
	}
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return nil, err
	}
	if strings.TrimSpace(parsed.AccessToken) == "" {
		return nil, fmt.Errorf("kiro social token refresh returned empty access token")
	}

	token := &kiroTokenData{
		AccessToken:  strings.TrimSpace(parsed.AccessToken),
		RefreshToken: firstNonEmpty(strings.TrimSpace(parsed.RefreshToken), trimmedRefreshToken),
		ExpiresIn:    parsed.ExpiresIn,
		TokenType:    "Bearer",
		ProfileARN:   strings.TrimSpace(parsed.ProfileARN),
		Email:        extractKiroEmailFromJWT(parsed.AccessToken),
	}
	if token.ExpiresIn <= 0 {
		token.ExpiresIn = 3600
	}
	if strings.TrimSpace(token.ProfileARN) == "" {
		profileARN, profileEmail, profileErr := m.fetchKiroProfile(ctx, token.AccessToken)
		if profileErr == nil {
			token.ProfileARN = firstNonEmpty(strings.TrimSpace(token.ProfileARN), strings.TrimSpace(profileARN))
			token.Email = firstNonEmpty(strings.TrimSpace(token.Email), strings.TrimSpace(profileEmail))
		}
	}
	if strings.TrimSpace(token.Email) == "" {
		if email, emailErr := m.fetchKiroUserEmailWithToken(ctx, token.AccessToken); emailErr == nil {
			token.Email = strings.TrimSpace(email)
		}
	}
	return token, nil
}

func extractKiroEmailFromJWT(token string) string {
	parts := strings.Split(strings.TrimSpace(token), ".")
	if len(parts) < 2 {
		return ""
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return ""
	}

	var parsed struct {
		Email string `json:"email"`
	}
	if err := json.Unmarshal(payload, &parsed); err != nil {
		return ""
	}
	return strings.TrimSpace(parsed.Email)
}
