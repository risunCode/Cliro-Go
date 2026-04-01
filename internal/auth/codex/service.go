package codex

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"cliro-go/internal/config"
	"cliro-go/internal/logger"

	"github.com/google/uuid"
)

type Service struct {
	store        *config.Manager
	log          *logger.Logger
	httpClient   func() *http.Client
	refreshQuota func(string) error

	mu             sync.RWMutex
	oauthSessions  map[string]*oauthSession
	callbackServer *http.Server
}

func NewService(store *config.Manager, log *logger.Logger, httpClient func() *http.Client, refreshQuota func(string) error) *Service {
	return &Service{
		store:         store,
		log:           log,
		httpClient:    httpClient,
		refreshQuota:  refreshQuota,
		oauthSessions: map[string]*oauthSession{},
	}
}

func (s *Service) StartAuth() (*AuthStart, error) {
	if err := s.ensureCallbackServer(); err != nil {
		return nil, err
	}

	verifier, err := GenerateCodeVerifier()
	if err != nil {
		return nil, err
	}
	sessionID := uuid.NewString()
	ctx, cancel := context.WithTimeout(context.Background(), DefaultOAuthTimeout)
	authURL := BuildAuthURL(sessionID, CodeChallenge(verifier))
	session := &oauthSession{
		AuthSessionView: AuthSessionView{
			SessionID:   sessionID,
			AuthURL:     authURL,
			CallbackURL: OAuthRedirectURI,
			Status:      sessionPending,
		},
		state:        sessionID,
		codeVerifier: verifier,
		createdAt:    time.Now(),
		ctx:          ctx,
		cancel:       cancel,
	}

	s.mu.Lock()
	s.oauthSessions[sessionID] = session
	s.mu.Unlock()

	go s.expireAuthSession(sessionID, ctx)
	s.log.Info("auth", "started Codex OAuth session "+sessionID)

	return &AuthStart{
		SessionID:   sessionID,
		AuthURL:     authURL,
		CallbackURL: OAuthRedirectURI,
		Status:      sessionPending,
	}, nil
}

func (s *Service) GetAuthSession(sessionID string) AuthSessionView {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if session, ok := s.oauthSessions[sessionID]; ok {
		return session.AuthSessionView
	}
	return AuthSessionView{SessionID: sessionID, Status: sessionError, Error: "session not found"}
}

func (s *Service) CancelAuth(sessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if session, ok := s.oauthSessions[sessionID]; ok {
		session.Status = sessionError
		session.Error = "session cancelled"
		if session.cancel != nil {
			session.cancel()
			session.cancel = nil
		}
	}
}

func (s *Service) SubmitAuthCode(sessionID string, code string) error {
	code = strings.TrimSpace(code)
	if code == "" {
		return errEmptyAuthorizationCode
	}

	s.mu.RLock()
	session := s.oauthSessions[sessionID]
	s.mu.RUnlock()

	if session == nil {
		return errSessionNotFound
	}
	if session.Status != sessionPending {
		return errSessionNotPending
	}

	tokens, err := s.exchangeAuthorizationCodeWithRedirect(session.ctx, code, OAuthRedirectURI, session.codeVerifier)
	if err != nil {
		s.finishAuthError(sessionID, err)
		return err
	}

	account, err := s.accountFromTokens(tokens)
	if err != nil {
		s.finishAuthError(sessionID, err)
		return err
	}

	if err := s.upsertAccount(&account); err != nil {
		s.finishAuthError(sessionID, err)
		return err
	}

	s.refreshNewAccountQuota(account.ID, "failed to refresh quota for new Codex account")
	s.finishAuthSuccess(sessionID, account)
	s.log.Info("auth", "completed Codex auth session "+sessionID+" via manual code submission")

	return nil
}

func (s *Service) Shutdown(ctx context.Context) error {
	s.mu.Lock()
	for _, session := range s.oauthSessions {
		if session.cancel != nil {
			session.cancel()
		}
	}
	server := s.callbackServer
	s.callbackServer = nil
	s.mu.Unlock()
	if server == nil {
		return nil
	}
	return server.Shutdown(ctx)
}

func (s *Service) RefreshAccount(account config.Account, force bool) (config.Account, error) {
	if !force && !tokenExpired(account, time.Now()) {
		return account, nil
	}
	if strings.TrimSpace(account.RefreshToken) == "" {
		return account, fmt.Errorf("account has no refresh token")
	}

	tokens, err := s.refreshTokens(context.Background(), account.RefreshToken)
	if err != nil {
		if blockedMsg, blocked := blockedAccountMessageFromAuthError(err); blocked {
			_ = s.store.UpdateAccount(account.ID, func(a *config.Account) {
				a.Enabled = false
				a.Banned = true
				a.BannedReason = blockedMsg
				a.LastError = blockedMsg
			})
		}
		s.log.Error("auth", "refresh failed for "+account.Email+": "+err.Error())
		return account, err
	}

	claims, err := ParseIDToken(tokens.IDToken)
	if err != nil {
		return account, err
	}

	err = s.store.UpdateAccount(account.ID, func(a *config.Account) {
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
		a.HealthState = config.AccountHealthReady
		a.HealthReason = ""
		a.Banned = false
		a.BannedReason = ""
		a.CooldownUntil = 0
		a.ConsecutiveFailures = 0
		a.LastError = ""
	})
	if err != nil {
		return account, err
	}

	refreshed, _ := s.store.GetAccount(account.ID)
	s.log.Info("auth", "refreshed token for "+refreshed.Email)
	return refreshed, nil
}

func (s *Service) refreshNewAccountQuota(accountID string, logMessage string) {
	if s.refreshQuota == nil {
		return
	}
	if err := s.refreshQuota(accountID); err != nil {
		s.log.Warn("auth", logMessage+": "+err.Error())
	}
}

func (s *Service) client() *http.Client {
	if s.httpClient != nil {
		if client := s.httpClient(); client != nil {
			return client
		}
	}
	return &http.Client{Timeout: 60 * time.Second}
}

func (s *Service) accountFromTokens(tokens *TokenExchangeResponse) (config.Account, error) {
	claims, err := ParseIDToken(tokens.IDToken)
	if err != nil {
		return config.Account{}, err
	}

	now := time.Now()
	account := config.Account{
		ID:           uuid.NewString(),
		Provider:     "codex",
		Email:        claims.Email,
		AccountID:    claims.CodexAuthInfo.ChatgptAccountID,
		PlanType:     strings.TrimSpace(claims.CodexAuthInfo.ChatgptPlanType),
		AccessToken:  tokens.AccessToken,
		RefreshToken: tokens.RefreshToken,
		IDToken:      tokens.IDToken,
		ExpiresAt:    now.Add(time.Duration(tokens.ExpiresIn) * time.Second).Unix(),
		Enabled:      true,
		CreatedAt:    now.Unix(),
		UpdatedAt:    now.Unix(),
		LastRefresh:  now.Unix(),
	}
	if claims.Exp > 0 {
		account.ExpiresAt = claims.Exp
	}
	return account, nil
}

func (s *Service) upsertAccount(account *config.Account) error {
	for _, existing := range s.store.Accounts() {
		if !strings.EqualFold(strings.TrimSpace(existing.Provider), "codex") {
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

	return s.store.UpsertAccount(*account)
}

func (s *Service) refreshTokens(ctx context.Context, refreshToken string) (*TokenExchangeResponse, error) {
	form := url.Values{
		"client_id":     {ClientID},
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
		"scope":         {"openid profile email offline_access"},
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
		return nil, fmt.Errorf("refresh token failed (%d): %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}

	var parsed TokenExchangeResponse
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil, err
	}
	return &parsed, nil
}

func AccountFingerprint(account config.Account) string {
	base := strings.TrimSpace(account.AccountID)
	if base == "" {
		base = strings.TrimSpace(account.Email)
	}
	sum := sha256.Sum256([]byte(base))
	return base64.RawURLEncoding.EncodeToString(sum[:6])
}

func blockedAccountMessageFromAuthError(err error) (string, bool) {
	if err == nil {
		return "", false
	}
	return config.BlockedAccountReason(err.Error())
}

func tokenExpired(account config.Account, now time.Time) bool {
	if account.ExpiresAt <= 0 {
		return false
	}
	return now.Unix() >= account.ExpiresAt
}
