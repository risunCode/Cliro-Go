package quota

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"cliro-go/internal/auth"
	"cliro-go/internal/config"
	"cliro-go/internal/logger"
)

const (
	testOAuthTokenURL      = "https://auth.openai.com/oauth/token"
	testCodexQuotaBaseURL  = "https://chatgpt.com/backend-api/codex"
	testKiroQuotaBaseURL   = "https://codewhisperer.us-east-1.amazonaws.com"
	testKiroDeviceTokenURL = "https://oidc.us-east-1.amazonaws.com/token"
	testKiroSocialAuthURL  = "https://prod.us-east-1.auth.desktop.kiro.dev"
)

type mockTransport struct {
	mu                sync.Mutex
	oauthCalls        int
	oauthStatus       int
	oauthErrorCode    string
	oauthErrorMessage string
	kiroTokenCalls    int
	quotaCalls        int
	kiroQuotaCalls    int
	kiroUserInfoCalls int
	quotaAuthHeaders  []string
	kiroQuotaAuth     []string
	kiroUserInfoAuth  []string
	idToken           string
	accessToken       string
	refreshToken      string
	kiroAccessToken   string
	kiroRefreshToken  string
	kiroProfileARN    string
	kiroEmail         string
}

func (m *mockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	switch req.URL.String() {
	case testOAuthTokenURL:
		m.mu.Lock()
		m.oauthCalls++
		oauthStatus := m.oauthStatus
		oauthErrorCode := m.oauthErrorCode
		oauthErrorMessage := m.oauthErrorMessage
		m.mu.Unlock()
		if oauthStatus >= 400 {
			message := strings.TrimSpace(oauthErrorMessage)
			if message == "" {
				message = "Your refresh token has already been used to generate a new access token. Please try signing in again."
			}
			code := strings.TrimSpace(oauthErrorCode)
			if code == "" {
				code = "refresh_token_reused"
			}
			return jsonResponse(oauthStatus, map[string]any{
				"error": map[string]any{
					"message": message,
					"code":    code,
					"type":    "invalid_request_error",
				},
			}), nil
		}
		return jsonResponse(http.StatusOK, map[string]any{
			"access_token":  m.accessToken,
			"refresh_token": m.refreshToken,
			"id_token":      m.idToken,
			"expires_in":    3600,
			"token_type":    "Bearer",
		}), nil
	case testKiroDeviceTokenURL:
		m.mu.Lock()
		m.kiroTokenCalls++
		m.mu.Unlock()
		return jsonResponse(http.StatusOK, map[string]any{
			"accessToken":  m.kiroAccessToken,
			"refreshToken": m.kiroRefreshToken,
			"expiresIn":    3600,
			"tokenType":    "Bearer",
			"profileArn":   m.kiroProfileARN,
		}), nil
	case testKiroSocialAuthURL + "/refreshToken":
		m.mu.Lock()
		m.kiroTokenCalls++
		m.mu.Unlock()
		return jsonResponse(http.StatusOK, map[string]any{
			"accessToken":  m.kiroAccessToken,
			"refreshToken": m.kiroRefreshToken,
			"expiresIn":    3600,
			"profileArn":   m.kiroProfileARN,
		}), nil
	case testCodexQuotaBaseURL + "/quotas":
		m.mu.Lock()
		m.quotaCalls++
		m.quotaAuthHeaders = append(m.quotaAuthHeaders, req.Header.Get("Authorization"))
		m.mu.Unlock()
		return jsonResponse(http.StatusOK, map[string]any{
			"rate_limit": map[string]any{
				"primary_window":   map[string]any{"used_percent": 10},
				"secondary_window": map[string]any{"used_percent": 20},
			},
		}), nil
	case testKiroQuotaBaseURL + "/getUsageLimits?origin=AI_EDITOR&resourceType=AGENTIC_REQUEST&isEmailRequired=true":
		m.mu.Lock()
		m.kiroQuotaCalls++
		m.kiroQuotaAuth = append(m.kiroQuotaAuth, req.Header.Get("Authorization"))
		m.mu.Unlock()
		return jsonResponse(http.StatusOK, map[string]any{
			"usageBreakdownList": []map[string]any{{
				"currentUsage": 12,
				"usageLimit":   100,
			}},
			"subscriptionInfo": map[string]any{
				"subscriptionTitle": "Kiro Pro",
			},
			"userInfo": map[string]any{"email": ""},
		}), nil
	case testKiroQuotaBaseURL + "/GetUserInfo":
		m.mu.Lock()
		m.kiroUserInfoCalls++
		m.kiroUserInfoAuth = append(m.kiroUserInfoAuth, req.Header.Get("Authorization"))
		m.mu.Unlock()
		return jsonResponse(http.StatusOK, map[string]any{"email": m.kiroEmail}), nil
	default:
		return jsonResponse(http.StatusNotFound, map[string]any{"error": "not found"}), nil
	}
}

func TestRefreshAccountWithQuota_CodexUsesRefreshedToken(t *testing.T) {
	now := time.Now().Unix()
	store, authManager, service := newTestService(t, &mockTransport{
		idToken:      buildTestIDToken(now+7200, "after@example.com", "account-after", "team"),
		accessToken:  "access-after",
		refreshToken: "refresh-after",
	})

	account := config.Account{
		ID:           "acct-not-expired",
		Provider:     "codex",
		Email:        "before@example.com",
		AccountID:    "account-before",
		PlanType:     "plus",
		AccessToken:  "access-before",
		RefreshToken: "refresh-before",
		IDToken:      buildTestIDToken(now+3600, "before@example.com", "account-before", "plus"),
		ExpiresAt:    now + 3600,
		Enabled:      true,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := store.UpsertAccount(account); err != nil {
		t.Fatalf("upsert account: %v", err)
	}
	authManager.SetQuotaRefresher(service)

	updated, err := service.RefreshAccountWithQuota(account.ID)
	if err != nil {
		t.Fatalf("refresh account with quota: %v", err)
	}

	transport := authManager.HTTPClient().Transport.(*mockTransport)
	transport.mu.Lock()
	defer transport.mu.Unlock()
	if transport.oauthCalls != 1 {
		t.Fatalf("expected one token refresh call, got %d", transport.oauthCalls)
	}
	if transport.quotaCalls != 1 {
		t.Fatalf("expected one quota call, got %d", transport.quotaCalls)
	}
	if len(transport.quotaAuthHeaders) != 1 || transport.quotaAuthHeaders[0] != "Bearer access-after" {
		t.Fatalf("expected refreshed auth header, got %v", transport.quotaAuthHeaders)
	}
	if updated.AccessToken != "access-after" {
		t.Fatalf("expected access token updated, got %q", updated.AccessToken)
	}
	if updated.Quota.Source != "codex/quotas" {
		t.Fatalf("expected quota source codex/quotas, got %q", updated.Quota.Source)
	}
}

func TestRefreshQuota_KiroRefreshesTokenAndResolvesEmail(t *testing.T) {
	now := time.Now().Unix()
	store, authManager, service := newTestService(t, &mockTransport{
		kiroAccessToken:  "kiro-access-fresh",
		kiroRefreshToken: "kiro-refresh-fresh",
		kiroProfileARN:   "arn:aws:codewhisperer:us-east-1:123456789012:profile/ABC",
		kiroEmail:        "kiro-expired@example.com",
	})

	account := config.Account{
		ID:           "acct-kiro-expired",
		Provider:     "kiro",
		Email:        "kiro-fallback",
		AccessToken:  "kiro-access",
		RefreshToken: "kiro-refresh",
		ClientID:     "kiro-client",
		ClientSecret: "kiro-secret",
		ExpiresAt:    now - 60,
		Enabled:      true,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := store.UpsertAccount(account); err != nil {
		t.Fatalf("upsert account: %v", err)
	}

	updated, err := service.RefreshQuota(account.ID)
	if err != nil {
		t.Fatalf("refresh quota: %v", err)
	}

	transport := authManager.HTTPClient().Transport.(*mockTransport)
	transport.mu.Lock()
	defer transport.mu.Unlock()
	if transport.oauthCalls != 0 {
		t.Fatalf("expected no codex oauth refresh, got %d", transport.oauthCalls)
	}
	if transport.kiroTokenCalls != 1 {
		t.Fatalf("expected one kiro token refresh, got %d", transport.kiroTokenCalls)
	}
	if transport.kiroQuotaCalls != 1 {
		t.Fatalf("expected one kiro quota call, got %d", transport.kiroQuotaCalls)
	}
	if len(transport.kiroQuotaAuth) != 1 || transport.kiroQuotaAuth[0] != "Bearer kiro-access-fresh" {
		t.Fatalf("expected refreshed kiro quota auth header, got %v", transport.kiroQuotaAuth)
	}
	if updated.Email != "kiro-expired@example.com" {
		t.Fatalf("expected email updated from user info, got %q", updated.Email)
	}
	if updated.AccessToken != "kiro-access-fresh" {
		t.Fatalf("expected refreshed kiro access token, got %q", updated.AccessToken)
	}
	if updated.Quota.Source != "kiro/getUsageLimits" {
		t.Fatalf("expected kiro quota source, got %q", updated.Quota.Source)
	}
}

func TestRefreshAccountWithQuota_RefreshErrorMarksNeedRelogin(t *testing.T) {
	now := time.Now().Unix()
	store, _, service := newTestService(t, &mockTransport{
		oauthStatus:       http.StatusUnauthorized,
		oauthErrorCode:    "refresh_token_reused",
		oauthErrorMessage: "Your refresh token has already been used to generate a new access token. Please try signing in again.",
	})

	account := config.Account{
		ID:           "acct-codex-refresh-failed",
		Provider:     "codex",
		Email:        "retry@example.com",
		AccessToken:  "access-old",
		RefreshToken: "refresh-old",
		IDToken:      buildTestIDToken(now-60, "retry@example.com", "acct-retry", "plus"),
		ExpiresAt:    now - 60,
		Enabled:      true,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := store.UpsertAccount(account); err != nil {
		t.Fatalf("upsert account: %v", err)
	}

	updated, err := service.RefreshAccountWithQuota(account.ID)
	if err == nil {
		t.Fatalf("expected refresh failure")
	}

	if updated.HealthState != config.AccountHealthCooldownTransient {
		t.Fatalf("health state = %q, want %q", updated.HealthState, config.AccountHealthCooldownTransient)
	}
	if updated.HealthReason != "Need re-login" {
		t.Fatalf("health reason = %q, want Need re-login", updated.HealthReason)
	}
	if updated.Quota.Status != "unknown" {
		t.Fatalf("quota status = %q, want unknown", updated.Quota.Status)
	}
	if updated.Quota.Summary != "Authentication required" {
		t.Fatalf("quota summary = %q, want Authentication required", updated.Quota.Summary)
	}
	if !strings.Contains(strings.ToLower(updated.Quota.Error), "refresh token") {
		t.Fatalf("quota error = %q, expected refresh token detail", updated.Quota.Error)
	}
}

func TestApplyQuotaSnapshot_KeepsManualDisabledAccountOff(t *testing.T) {
	now := time.Now().Unix()
	store, _, service := newTestService(t, &mockTransport{})

	account := config.Account{
		ID:           "acct-manual-disabled",
		Provider:     "codex",
		Email:        "manual@example.com",
		Enabled:      false,
		HealthState:  config.AccountHealthDisabledDurable,
		HealthReason: "Disabled by user",
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := store.UpsertAccount(account); err != nil {
		t.Fatalf("upsert account: %v", err)
	}

	quota := config.QuotaInfo{
		Status:        "exhausted",
		Summary:       "Quota exhausted",
		LastCheckedAt: now,
		Buckets: []config.QuotaBucket{{
			Name:    "session",
			Status:  "exhausted",
			ResetAt: now + 7200,
		}},
	}

	if err := service.applyQuotaSnapshot(account.ID, quota, ""); err != nil {
		t.Fatalf("applyQuotaSnapshot: %v", err)
	}

	updated, ok := store.GetAccount(account.ID)
	if !ok {
		t.Fatalf("expected account to exist")
	}
	if updated.Enabled {
		t.Fatalf("expected manually disabled account to stay disabled")
	}
	if updated.HealthState != config.AccountHealthDisabledDurable {
		t.Fatalf("health state = %q, want %q", updated.HealthState, config.AccountHealthDisabledDurable)
	}
}

func TestBlockedAccountMessageFromQuota_DetectsDeactivatedStatus(t *testing.T) {
	message, blocked := blockedAccountMessageFromQuota(config.QuotaInfo{
		Status:  "deactivated",
		Summary: "Your OpenAI account has been deactivated.",
	})
	if !blocked {
		t.Fatalf("expected blocked=true for deactivated quota status")
	}
	if message != "Your OpenAI account has been deactivated." {
		t.Fatalf("message = %q", message)
	}
}

func TestRefreshAllQuotas_SkipsCooldownDisabledAndBannedAccounts(t *testing.T) {
	now := time.Now().Unix()
	store, authManager, service := newTestService(t, &mockTransport{})

	accounts := []config.Account{
		{ID: "ready", Provider: "codex", Email: "ready@example.com", AccessToken: "ready-access", RefreshToken: "ready-refresh", IDToken: buildTestIDToken(now+3600, "ready@example.com", "acct-ready", "plus"), ExpiresAt: now + 3600, Enabled: true, CreatedAt: now, UpdatedAt: now},
		{ID: "cooldown-future", Provider: "codex", Email: "cooldown@example.com", AccessToken: "cooldown-access", RefreshToken: "cooldown-refresh", IDToken: buildTestIDToken(now+3600, "cooldown@example.com", "acct-cooldown", "plus"), ExpiresAt: now + 3600, Enabled: true, Quota: config.QuotaInfo{Status: "exhausted", LastCheckedAt: now, Buckets: []config.QuotaBucket{{Name: "session", Status: "exhausted", ResetAt: now + 3600}}}, CreatedAt: now, UpdatedAt: now},
		{ID: "cooldown-expired", Provider: "codex", Email: "expired@example.com", AccessToken: "expired-access", RefreshToken: "expired-refresh", IDToken: buildTestIDToken(now+3600, "expired@example.com", "acct-expired", "plus"), ExpiresAt: now + 3600, Enabled: true, Quota: config.QuotaInfo{Status: "exhausted", LastCheckedAt: now, Buckets: []config.QuotaBucket{{Name: "session", Status: "exhausted", ResetAt: now - 60}}}, CreatedAt: now, UpdatedAt: now},
		{ID: "disabled", Provider: "codex", Email: "disabled@example.com", AccessToken: "disabled-access", RefreshToken: "disabled-refresh", IDToken: buildTestIDToken(now+3600, "disabled@example.com", "acct-disabled", "plus"), ExpiresAt: now + 3600, Enabled: false, CreatedAt: now, UpdatedAt: now},
		{ID: "banned", Provider: "codex", Email: "banned@example.com", AccessToken: "banned-access", RefreshToken: "banned-refresh", IDToken: buildTestIDToken(now+3600, "banned@example.com", "acct-banned", "plus"), ExpiresAt: now + 3600, Enabled: true, Banned: true, HealthState: config.AccountHealthBanned, CreatedAt: now, UpdatedAt: now},
	}
	for _, account := range accounts {
		if err := store.UpsertAccount(account); err != nil {
			t.Fatalf("upsert account %s: %v", account.ID, err)
		}
	}

	if err := service.RefreshAllQuotas(); err != nil {
		t.Fatalf("refresh all quotas: %v", err)
	}

	transport := authManager.HTTPClient().Transport.(*mockTransport)
	transport.mu.Lock()
	defer transport.mu.Unlock()
	if transport.quotaCalls != 2 {
		t.Fatalf("expected 2 quota calls, got %d", transport.quotaCalls)
	}
}

func TestForceRefreshAllQuotas_IncludesPreviouslySkippedAccounts(t *testing.T) {
	now := time.Now().Unix()
	store, authManager, service := newTestService(t, &mockTransport{})

	accounts := []config.Account{
		{ID: "ready", Provider: "codex", Email: "ready@example.com", AccessToken: "ready-access", RefreshToken: "ready-refresh", IDToken: buildTestIDToken(now+3600, "ready@example.com", "acct-ready", "plus"), ExpiresAt: now + 3600, Enabled: true, CreatedAt: now, UpdatedAt: now},
		{ID: "cooldown-future", Provider: "codex", Email: "cooldown@example.com", AccessToken: "cooldown-access", RefreshToken: "cooldown-refresh", IDToken: buildTestIDToken(now+3600, "cooldown@example.com", "acct-cooldown", "plus"), ExpiresAt: now + 3600, Enabled: true, Quota: config.QuotaInfo{Status: "exhausted", LastCheckedAt: now, Buckets: []config.QuotaBucket{{Name: "session", Status: "exhausted", ResetAt: now + 3600}}}, CreatedAt: now, UpdatedAt: now},
		{ID: "disabled", Provider: "codex", Email: "disabled@example.com", AccessToken: "disabled-access", RefreshToken: "disabled-refresh", IDToken: buildTestIDToken(now+3600, "disabled@example.com", "acct-disabled", "plus"), ExpiresAt: now + 3600, Enabled: false, CreatedAt: now, UpdatedAt: now},
		{ID: "banned", Provider: "codex", Email: "banned@example.com", AccessToken: "banned-access", RefreshToken: "banned-refresh", IDToken: buildTestIDToken(now+3600, "banned@example.com", "acct-banned", "plus"), ExpiresAt: now + 3600, Enabled: true, Banned: true, HealthState: config.AccountHealthBanned, CreatedAt: now, UpdatedAt: now},
	}
	for _, account := range accounts {
		if err := store.UpsertAccount(account); err != nil {
			t.Fatalf("upsert account %s: %v", account.ID, err)
		}
	}

	if err := service.ForceRefreshAllQuotas(); err != nil {
		t.Fatalf("force refresh all quotas: %v", err)
	}

	transport := authManager.HTTPClient().Transport.(*mockTransport)
	transport.mu.Lock()
	defer transport.mu.Unlock()
	if transport.quotaCalls != 4 {
		t.Fatalf("expected 4 quota calls, got %d", transport.quotaCalls)
	}
}

func newTestService(t *testing.T, transport *mockTransport) (*config.Manager, *auth.Manager, *Service) {
	t.Helper()
	store, err := config.NewManager(t.TempDir())
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	authManager := auth.NewManager(store, logger.New(10))
	authManager.SetHTTPClient(&http.Client{Transport: transport, Timeout: 5 * time.Second})
	service := NewService(store, authManager, logger.New(10), authManager.HTTPClient())
	return store, authManager, service
}

func jsonResponse(status int, payload map[string]any) *http.Response {
	data, _ := json.Marshal(payload)
	return &http.Response{
		StatusCode: status,
		Header:     make(http.Header),
		Body:       io.NopCloser(bytes.NewReader(data)),
	}
}

func buildTestIDToken(exp int64, email string, accountID string, planType string) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","typ":"JWT"}`))
	payloadMap := map[string]any{
		"email": email,
		"exp":   exp,
		"https://api.openai.com/auth": map[string]any{
			"chatgpt_account_id": accountID,
			"chatgpt_plan_type":  planType,
		},
	}
	payloadJSON, _ := json.Marshal(payloadMap)
	payload := base64.RawURLEncoding.EncodeToString(payloadJSON)
	signature := base64.RawURLEncoding.EncodeToString([]byte("sig"))
	return strings.Join([]string{header, payload, signature}, ".")
}
