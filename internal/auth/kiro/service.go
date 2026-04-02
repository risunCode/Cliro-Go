package kiro

import (
	"cliro-go/internal/util"
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"cliro-go/internal/config"
	"cliro-go/internal/logger"

	"github.com/google/uuid"
)

const (
	defaultDeviceWait   = 15 * time.Minute
	minimumPollInterval = 5
	socialAuthTimeout   = 10 * time.Minute

	sessionPending = "pending"
	sessionSuccess = "success"
	sessionError   = "error"
)

type authSession struct {
	AuthSessionView
	deviceCode   string
	interval     int
	clientID     string
	clientSecret string
	state        string
	codeVerifier string
	redirectURI  string
	callbackCh   <-chan SocialCallbackResult
	createdAt    time.Time
	ctx          context.Context
	cancel       context.CancelFunc
}

type Service struct {
	store        *config.Manager
	log          *logger.Logger
	httpClient   func() *http.Client
	refreshQuota func(string) error

	mu           sync.RWMutex
	kiroSessions map[string]*authSession
}

func NewService(store *config.Manager, log *logger.Logger, httpClient func() *http.Client, refreshQuota func(string) error) *Service {
	return &Service{
		store:        store,
		log:          log,
		httpClient:   httpClient,
		refreshQuota: refreshQuota,
		kiroSessions: map[string]*authSession{},
	}
}

func (s *Service) StartAuth() (*AuthStart, error) {
	s.cleanupCompletedAuthSessions()

	client, err := s.registerClient(context.Background())
	if err != nil {
		s.log.Error("auth", "Kiro client registration failed: "+err.Error())
		return nil, err
	}

	device, err := s.startDeviceAuthorization(context.Background(), client.ClientID, client.ClientSecret)
	if err != nil {
		s.log.Error("auth", "Kiro device authorization failed: "+err.Error())
		return nil, err
	}

	authURL := util.FirstNonEmpty(device.VerificationURIComplete, device.VerificationURI)
	if strings.TrimSpace(authURL) == "" {
		return nil, fmt.Errorf("kiro device authorization did not return a verification URL")
	}

	expiresIn := device.ExpiresIn
	if expiresIn <= 0 {
		expiresIn = int(defaultDeviceWait / time.Second)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(expiresIn)*time.Second)

	interval := device.Interval
	if interval < minimumPollInterval {
		interval = minimumPollInterval
	}

	sessionID := uuid.NewString()
	session := &authSession{
		AuthSessionView: AuthSessionView{
			SessionID:       sessionID,
			AuthURL:         strings.TrimSpace(authURL),
			VerificationURL: strings.TrimSpace(device.VerificationURI),
			UserCode:        strings.TrimSpace(device.UserCode),
			ExpiresAt:       time.Now().Add(time.Duration(expiresIn) * time.Second).Unix(),
			Status:          sessionPending,
		},
		deviceCode:   strings.TrimSpace(device.DeviceCode),
		interval:     interval,
		clientID:     strings.TrimSpace(client.ClientID),
		clientSecret: strings.TrimSpace(client.ClientSecret),
		createdAt:    time.Now(),
		ctx:          ctx,
		cancel:       cancel,
	}

	s.mu.Lock()
	s.kiroSessions[sessionID] = session
	s.mu.Unlock()

	go s.expireAuthSession(sessionID, ctx)
	go s.completeAuthSession(sessionID)

	s.log.Info("auth", "started Kiro device auth session "+sessionID)

	return &AuthStart{
		SessionID:       sessionID,
		AuthURL:         session.AuthURL,
		VerificationURL: session.VerificationURL,
		UserCode:        session.UserCode,
		ExpiresAt:       session.ExpiresAt,
		Status:          sessionPending,
		AuthMethod:      "device",
		Provider:        "aws_builder_id",
	}, nil
}

func (s *Service) StartSocialAuth(provider string) (*AuthStart, error) {
	s.cleanupCompletedAuthSessions()

	resolvedProvider, err := NormalizeSocialProvider(provider)
	if err != nil {
		return nil, err
	}

	codeVerifier, codeChallenge, err := GeneratePKCE()
	if err != nil {
		return nil, err
	}
	state, err := GenerateState()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), socialAuthTimeout)
	redirectURI, callbackCh, err := s.startSocialCallbackServer(ctx, state)
	if err != nil {
		cancel()
		return nil, err
	}

	authURL := BuildSocialLoginURL(resolvedProvider, codeChallenge, state)
	sessionID := uuid.NewString()
	session := &authSession{
		AuthSessionView: AuthSessionView{
			SessionID:  sessionID,
			AuthURL:    authURL,
			ExpiresAt:  time.Now().Add(socialAuthTimeout).Unix(),
			Status:     sessionPending,
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

	s.mu.Lock()
	s.kiroSessions[sessionID] = session
	s.mu.Unlock()

	go s.expireAuthSession(sessionID, ctx)
	go s.completeSocialAuthSession(sessionID)

	s.log.Info("auth", "started Kiro social auth session "+sessionID+" provider="+string(resolvedProvider))

	return &AuthStart{
		SessionID:  sessionID,
		AuthURL:    authURL,
		ExpiresAt:  session.ExpiresAt,
		Status:     sessionPending,
		AuthMethod: "social",
		Provider:   strings.ToLower(string(resolvedProvider)),
	}, nil
}

func (s *Service) GetAuthSession(sessionID string) AuthSessionView {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if session, ok := s.kiroSessions[sessionID]; ok {
		return session.AuthSessionView
	}
	return AuthSessionView{SessionID: sessionID, Status: sessionError, Error: "session not found"}
}

func (s *Service) GetAllAuthSessions() []AuthSessionView {
	s.mu.RLock()
	defer s.mu.RUnlock()

	sessions := make([]AuthSessionView, 0, len(s.kiroSessions))
	for _, session := range s.kiroSessions {
		sessions = append(sessions, session.AuthSessionView)
	}
	return sessions
}

func (s *Service) CancelAuth(sessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if session, ok := s.kiroSessions[sessionID]; ok {
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
		return fmt.Errorf("authorization code is empty")
	}

	session, ok := s.sessionSnapshot(sessionID)
	if !ok {
		return fmt.Errorf("session not found")
	}
	if session.Status != sessionPending {
		return fmt.Errorf("session is not pending")
	}
	if session.AuthMethod != "social" {
		return fmt.Errorf("session is not a social auth session")
	}

	tokens, err := s.exchangeSocialCode(session.ctx, code, session.codeVerifier)
	if err != nil {
		s.finishAuthError(sessionID, err)
		return err
	}

	account, err := s.accountFromToken(tokens)
	if err != nil {
		s.finishAuthError(sessionID, err)
		return err
	}

	if err := s.upsertAccount(&account); err != nil {
		s.finishAuthError(sessionID, err)
		return err
	}

	s.refreshNewAccountQuota(account.ID)
	s.finishAuthSuccess(sessionID, account)
	s.log.Info("auth", "completed Kiro social auth session "+sessionID+" via manual code submission")

	return nil
}

func (s *Service) RefreshAccount(account config.Account, force bool) (config.Account, error) {
	if !force && !tokenExpired(account, time.Now()) {
		return account, nil
	}
	if strings.TrimSpace(account.RefreshToken) == "" {
		return account, fmt.Errorf("account has no refresh token")
	}
	if (strings.TrimSpace(account.ClientID) == "" || strings.TrimSpace(account.ClientSecret) == "") && !looksLikeSocialRefreshToken(account.RefreshToken) {
		return account, fmt.Errorf("kiro account missing client credentials; reconnect account")
	}

	tokens, err := s.refreshTokens(context.Background(), account.ClientID, account.ClientSecret, account.RefreshToken)
	if err != nil {
		if blockedMsg, blocked := blockedAccountMessageFromAuthError(err); blocked {
			_ = s.store.MarkAccountBanned(account.ID, blockedMsg)
		} else if reloginMessage, refreshable := config.RefreshableAuthReason(err.Error()); refreshable {
			now := time.Now().Unix()
			_ = s.store.UpdateAccount(account.ID, func(a *config.Account) {
				a.HealthState = config.AccountHealthCooldownTransient
				a.HealthReason = "Need re-login"
				a.CooldownUntil = now + int64((30*time.Second)/time.Second)
				a.LastFailureAt = now
				a.LastError = reloginMessage
				a.Quota = config.QuotaInfo{
					Status:        "unknown",
					Summary:       "Authentication required",
					Source:        "runtime",
					Error:         reloginMessage,
					LastCheckedAt: now,
				}
			})
			if updated, ok := s.store.GetAccount(account.ID); ok {
				account = updated
			}
		}
		s.log.Error("auth", "kiro refresh failed for "+account.Email+": "+err.Error())
		return account, err
	}

	err = s.store.UpdateAccount(account.ID, func(a *config.Account) {
		a.Provider = "kiro"
		a.AccessToken = tokens.AccessToken
		if strings.TrimSpace(tokens.RefreshToken) != "" {
			a.RefreshToken = tokens.RefreshToken
		}
		a.ClientID = util.FirstNonEmpty(strings.TrimSpace(tokens.ClientID), strings.TrimSpace(a.ClientID))
		a.ClientSecret = util.FirstNonEmpty(strings.TrimSpace(tokens.ClientSecret), strings.TrimSpace(a.ClientSecret))
		if strings.TrimSpace(tokens.ProfileARN) != "" {
			a.AccountID = strings.TrimSpace(tokens.ProfileARN)
		}
		if strings.TrimSpace(tokens.Email) != "" {
			a.Email = strings.TrimSpace(tokens.Email)
		}
		a.ExpiresAt = time.Now().Add(time.Duration(tokens.ExpiresIn) * time.Second).Unix()
		a.LastRefresh = time.Now().Unix()
		a.HealthState = config.AccountHealthReady
		a.HealthReason = ""
		a.LastError = ""
		a.Banned = false
		a.BannedReason = ""
		a.CooldownUntil = 0
		a.ConsecutiveFailures = 0
	})
	if err != nil {
		return account, err
	}

	refreshed, _ := s.store.GetAccount(account.ID)
	s.log.Info("auth", "refreshed Kiro token for "+refreshed.Email)
	return refreshed, nil
}

func (s *Service) Shutdown(_ context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for sessionID, session := range s.kiroSessions {
		if session.cancel != nil {
			session.cancel()
			session.cancel = nil
		}
		delete(s.kiroSessions, sessionID)
	}
	return nil
}

func (s *Service) cleanupCompletedAuthSessions() {
	s.mu.Lock()
	defer s.mu.Unlock()

	for sessionID, session := range s.kiroSessions {
		if session.Status != sessionSuccess && session.Status != sessionError {
			continue
		}
		if session.cancel != nil {
			session.cancel()
		}
		delete(s.kiroSessions, sessionID)
		s.log.Info("auth", "cleaned up completed Kiro auth session "+sessionID+" status="+session.Status)
	}
}

func (s *Service) completeAuthSession(sessionID string) {
	session, ok := s.sessionSnapshot(sessionID)
	if !ok {
		return
	}

	tokens, err := s.pollDeviceToken(session.ctx, session.clientID, session.clientSecret, session.deviceCode, session.interval)
	if err != nil {
		s.finishAuthError(sessionID, err)
		return
	}

	account, err := s.accountFromToken(tokens)
	if err != nil {
		s.finishAuthError(sessionID, err)
		return
	}
	if err := s.upsertAccount(&account); err != nil {
		s.finishAuthError(sessionID, err)
		return
	}

	s.refreshNewAccountQuota(account.ID)
	s.finishAuthSuccess(sessionID, account)
}

func (s *Service) completeSocialAuthSession(sessionID string) {
	session, ok := s.sessionSnapshot(sessionID)
	if !ok {
		return
	}

	select {
	case <-session.ctx.Done():
		if session.ctx.Err() != nil && session.ctx.Err() != context.Canceled {
			s.finishAuthError(sessionID, session.ctx.Err())
		}
		return
	case callback, ok := <-session.callbackCh:
		if !ok {
			s.finishAuthError(sessionID, fmt.Errorf("kiro social auth callback closed before completion"))
			return
		}
		if strings.TrimSpace(callback.Error) != "" {
			s.finishAuthError(sessionID, fmt.Errorf(callback.Error))
			return
		}
		if strings.TrimSpace(callback.Code) == "" {
			s.finishAuthError(sessionID, fmt.Errorf("kiro social auth did not return an authorization code"))
			return
		}
		if strings.TrimSpace(callback.State) != strings.TrimSpace(session.state) {
			s.finishAuthError(sessionID, fmt.Errorf("kiro social auth state mismatch"))
			return
		}

		tokens, err := s.exchangeSocialCode(session.ctx, callback.Code, session.codeVerifier)
		if err != nil {
			s.finishAuthError(sessionID, err)
			return
		}

		account, err := s.accountFromToken(tokens)
		if err != nil {
			s.finishAuthError(sessionID, err)
			return
		}
		if err := s.upsertAccount(&account); err != nil {
			s.finishAuthError(sessionID, err)
			return
		}

		s.refreshNewAccountQuota(account.ID)
		s.finishAuthSuccess(sessionID, account)
	}
}

func (s *Service) expireAuthSession(sessionID string, ctx context.Context) {
	<-ctx.Done()

	s.mu.Lock()
	defer s.mu.Unlock()

	session, ok := s.kiroSessions[sessionID]
	if !ok || session.Status != sessionPending {
		return
	}

	session.Status = sessionError
	session.Error = "kiro device auth session expired"
	if session.cancel != nil {
		session.cancel = nil
	}
	s.log.Warn("auth", "Kiro auth session expired: "+sessionID)
}

func (s *Service) sessionSnapshot(sessionID string) (authSession, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	session, ok := s.kiroSessions[sessionID]
	if !ok || session == nil {
		return authSession{}, false
	}
	return *session, true
}

func (s *Service) accountFromToken(tokens *TokenData) (config.Account, error) {
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
		AuthMethod:   DetermineAuthMethod(tokens),
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

func (s *Service) upsertAccount(account *config.Account) error {
	for _, existing := range s.store.Accounts() {
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

	return s.store.UpsertAccount(*account)
}

func (s *Service) finishAuthSuccess(sessionID string, account config.Account) {
	s.mu.Lock()
	defer s.mu.Unlock()

	session, ok := s.kiroSessions[sessionID]
	if !ok || session.Status != sessionPending {
		return
	}

	session.Status = sessionSuccess
	session.Email = strings.TrimSpace(account.Email)
	session.AccountID = strings.TrimSpace(account.ID)
	if session.cancel != nil {
		session.cancel()
		session.cancel = nil
	}
	s.log.Info("auth", "Kiro auth completed for "+util.FirstNonEmpty(account.Email, account.ID))
}

func (s *Service) finishAuthError(sessionID string, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	session, ok := s.kiroSessions[sessionID]
	if !ok || session.Status != sessionPending {
		return
	}

	session.Status = sessionError
	session.Error = err.Error()
	if session.cancel != nil {
		session.cancel()
		session.cancel = nil
	}
	s.log.Error("auth", "Kiro auth failed: "+err.Error())
}

func (s *Service) refreshNewAccountQuota(accountID string) {
	if s.refreshQuota == nil {
		return
	}
	if err := s.refreshQuota(accountID); err != nil {
		s.log.Warn("auth", "failed to refresh quota for new Kiro account: "+err.Error())
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

func blockedAccountMessageFromAuthError(err error) (string, bool) {
	if err == nil {
		return "", false
	}
	return config.BlockedAccountReason(err.Error())
}

func looksLikeSocialRefreshToken(refreshToken string) bool {
	return strings.HasPrefix(strings.TrimSpace(refreshToken), "aorAAAAAG")
}

func tokenExpired(account config.Account, now time.Time) bool {
	if account.ExpiresAt <= 0 {
		return false
	}
	return now.Unix() >= account.ExpiresAt
}
