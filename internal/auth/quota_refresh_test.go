package auth

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

	"cliro-go/internal/config"
	"cliro-go/internal/logger"
)

type quotaRefreshMockTransport struct {
	mu                sync.Mutex
	oauthCalls        int
	kiroTokenCalls    int
	quotaCalls        int
	kiroQuotaCalls    int
	kiroUserInfoCalls int
	quotaAuthHeaders  []string
	kiroQuotaAuth     []string
	kiroUserInfoAuth  []string
	refreshAuthHeader []string
	idToken           string
	accessToken       string
	refreshToken      string
	kiroAccessToken   string
	kiroRefreshToken  string
	kiroProfileARN    string
	kiroEmail         string
}

func (m *quotaRefreshMockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.String() == oauthTokenURL {
		body, _ := io.ReadAll(req.Body)
		_ = req.Body.Close()

		m.mu.Lock()
		m.oauthCalls++
		m.refreshAuthHeader = append(m.refreshAuthHeader, string(body))
		m.mu.Unlock()

		payload := map[string]any{
			"access_token":  m.accessToken,
			"refresh_token": m.refreshToken,
			"id_token":      m.idToken,
			"expires_in":    3600,
			"token_type":    "Bearer",
		}
		return jsonResponse(http.StatusOK, payload), nil
	}

	if req.URL.String() == kiroDeviceTokenURL {
		body, _ := io.ReadAll(req.Body)
		_ = req.Body.Close()

		m.mu.Lock()
		m.kiroTokenCalls++
		m.refreshAuthHeader = append(m.refreshAuthHeader, string(body))
		m.mu.Unlock()

		payload := map[string]any{
			"accessToken":  m.kiroAccessToken,
			"refreshToken": m.kiroRefreshToken,
			"expiresIn":    3600,
			"tokenType":    "Bearer",
			"profileArn":   m.kiroProfileARN,
		}
		return jsonResponse(http.StatusOK, payload), nil
	}

	if req.URL.String() == kiroSocialAuthURL+"/refreshToken" {
		body, _ := io.ReadAll(req.Body)
		_ = req.Body.Close()

		m.mu.Lock()
		m.kiroTokenCalls++
		m.refreshAuthHeader = append(m.refreshAuthHeader, string(body))
		m.mu.Unlock()

		payload := map[string]any{
			"accessToken":  m.kiroAccessToken,
			"refreshToken": m.kiroRefreshToken,
			"expiresIn":    3600,
			"profileArn":   m.kiroProfileARN,
		}
		return jsonResponse(http.StatusOK, payload), nil
	}

	if req.URL.String() == codexQuotaBaseURL+"/quotas" {
		m.mu.Lock()
		m.quotaCalls++
		m.quotaAuthHeaders = append(m.quotaAuthHeaders, req.Header.Get("Authorization"))
		m.mu.Unlock()

		payload := map[string]any{
			"rate_limit": map[string]any{
				"primary_window":   map[string]any{"used_percent": 10},
				"secondary_window": map[string]any{"used_percent": 20},
			},
		}
		return jsonResponse(http.StatusOK, payload), nil
	}

	if req.URL.String() == kiroQuotaBaseURL+"/getUsageLimits?origin=AI_EDITOR&resourceType=AGENTIC_REQUEST&isEmailRequired=true" {
		m.mu.Lock()
		m.kiroQuotaCalls++
		m.kiroQuotaAuth = append(m.kiroQuotaAuth, req.Header.Get("Authorization"))
		m.mu.Unlock()

		payload := map[string]any{
			"usageBreakdownList": []map[string]any{{
				"currentUsage": 12,
				"usageLimit":   100,
			}},
			"subscriptionInfo": map[string]any{
				"subscriptionTitle": "Kiro Pro",
			},
			"userInfo": map[string]any{
				"email": "",
			},
		}
		return jsonResponse(http.StatusOK, payload), nil
	}

	if req.URL.String() == kiroQuotaBaseURL+"/GetUserInfo" {
		m.mu.Lock()
		m.kiroUserInfoCalls++
		m.kiroUserInfoAuth = append(m.kiroUserInfoAuth, req.Header.Get("Authorization"))
		m.mu.Unlock()

		payload := map[string]any{
			"email": m.kiroEmail,
		}
		return jsonResponse(http.StatusOK, payload), nil
	}

	return jsonResponse(http.StatusNotFound, map[string]any{"error": "not found"}), nil
}

func jsonResponse(status int, payload map[string]any) *http.Response {
	data, _ := json.Marshal(payload)
	return &http.Response{
		StatusCode: status,
		Header:     make(http.Header),
		Body:       io.NopCloser(bytes.NewReader(data)),
	}
}

func (m *quotaRefreshMockTransport) snapshot() (oauthCalls int, kiroTokenCalls int, quotaCalls int, quotaAuthHeaders []string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.oauthCalls, m.kiroTokenCalls, m.quotaCalls, append([]string(nil), m.quotaAuthHeaders...)
}

func (m *quotaRefreshMockTransport) snapshotKiro() (kiroQuotaCalls int, kiroUserInfoCalls int, kiroQuotaAuth []string, kiroUserInfoAuth []string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.kiroQuotaCalls, m.kiroUserInfoCalls, append([]string(nil), m.kiroQuotaAuth...), append([]string(nil), m.kiroUserInfoAuth...)
}

func TestRefreshAccountWithQuota_AlwaysRefreshesCodexTokenBeforeQuota(t *testing.T) {
	store, err := config.NewManager(t.TempDir())
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}

	now := time.Now().Unix()
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

	m := NewManager(store, logger.New(10))
	transport := &quotaRefreshMockTransport{
		idToken:      buildTestIDToken(now+7200, "after@example.com", "account-after", "team"),
		accessToken:  "access-after",
		refreshToken: "refresh-after",
	}
	m.client = &http.Client{Transport: transport, Timeout: 5 * time.Second}

	updated, err := m.RefreshAccountWithQuota(account.ID)
	if err != nil {
		t.Fatalf("refresh account with quota: %v", err)
	}

	oauthCalls, kiroTokenCalls, quotaCalls, quotaAuthHeaders := transport.snapshot()
	if oauthCalls != 1 {
		t.Fatalf("expected one token refresh call, got %d", oauthCalls)
	}
	if kiroTokenCalls != 0 {
		t.Fatalf("expected no kiro token refresh call, got %d", kiroTokenCalls)
	}
	if quotaCalls != 1 {
		t.Fatalf("expected one quota call, got %d", quotaCalls)
	}
	if len(quotaAuthHeaders) != 1 || quotaAuthHeaders[0] != "Bearer access-after" {
		t.Fatalf("expected quota Authorization to use refreshed token, got %v", quotaAuthHeaders)
	}
	if updated.AccessToken != "access-after" {
		t.Fatalf("expected access token updated, got %q", updated.AccessToken)
	}
	if updated.Quota.Source != "codex/quotas" {
		t.Fatalf("expected quota source codex/quotas, got %q", updated.Quota.Source)
	}
}

func TestRefreshAccountWithQuota_RefreshesTokenWhenExpired(t *testing.T) {
	store, err := config.NewManager(t.TempDir())
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}

	now := time.Now().Unix()
	account := config.Account{
		ID:           "acct-expired",
		Provider:     "codex",
		Email:        "before@example.com",
		AccountID:    "account-before",
		PlanType:     "plus",
		AccessToken:  "access-before",
		RefreshToken: "refresh-before",
		IDToken:      buildTestIDToken(now-120, "before@example.com", "account-before", "plus"),
		ExpiresAt:    now - 120,
		Enabled:      true,
		CreatedAt:    now - 3600,
		UpdatedAt:    now - 3600,
	}
	if err := store.UpsertAccount(account); err != nil {
		t.Fatalf("upsert account: %v", err)
	}

	m := NewManager(store, logger.New(10))
	transport := &quotaRefreshMockTransport{
		idToken:      buildTestIDToken(now+7200, "after@example.com", "account-after", "team"),
		accessToken:  "access-after",
		refreshToken: "refresh-after",
	}
	m.client = &http.Client{Transport: transport, Timeout: 5 * time.Second}

	updated, err := m.RefreshAccountWithQuota(account.ID)
	if err != nil {
		t.Fatalf("refresh account with quota: %v", err)
	}

	oauthCalls, kiroTokenCalls, quotaCalls, quotaAuthHeaders := transport.snapshot()
	if oauthCalls != 1 {
		t.Fatalf("expected one token refresh call, got %d", oauthCalls)
	}
	if kiroTokenCalls != 0 {
		t.Fatalf("expected no kiro token refresh call, got %d", kiroTokenCalls)
	}
	if quotaCalls != 1 {
		t.Fatalf("expected one quota call, got %d", quotaCalls)
	}
	if len(quotaAuthHeaders) != 1 || quotaAuthHeaders[0] != "Bearer access-after" {
		t.Fatalf("expected quota Authorization to use refreshed token, got %v", quotaAuthHeaders)
	}
	if updated.AccessToken != "access-after" {
		t.Fatalf("expected updated access token, got %q", updated.AccessToken)
	}
	if updated.RefreshToken != "refresh-after" {
		t.Fatalf("expected updated refresh token, got %q", updated.RefreshToken)
	}
	if updated.Email != "after@example.com" {
		t.Fatalf("expected updated email from id token, got %q", updated.Email)
	}
	if updated.AccountID != "account-after" {
		t.Fatalf("expected updated account id from id token, got %q", updated.AccountID)
	}
}

func TestRefreshAccountWithQuota_KiroAlwaysRefreshesThenFetchesEmailAndQuotaBucket(t *testing.T) {
	store, err := config.NewManager(t.TempDir())
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}

	now := time.Now().Unix()
	account := config.Account{
		ID:           "acct-kiro",
		Provider:     "kiro",
		Email:        "kiro-fallback",
		AccessToken:  "kiro-access-old",
		RefreshToken: "kiro-refresh",
		ClientID:     "kiro-client",
		ClientSecret: "kiro-secret",
		ExpiresAt:    now + 3600,
		Enabled:      true,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := store.UpsertAccount(account); err != nil {
		t.Fatalf("upsert account: %v", err)
	}

	m := NewManager(store, logger.New(10))
	transport := &quotaRefreshMockTransport{kiroAccessToken: "kiro-access-new", kiroRefreshToken: "kiro-refresh-new", kiroProfileARN: "arn:aws:codewhisperer:us-east-1:123456789012:profile/XYZ", kiroEmail: "kiro@example.com"}
	m.client = &http.Client{Transport: transport, Timeout: 5 * time.Second}

	updated, err := m.RefreshAccountWithQuota(account.ID)
	if err != nil {
		t.Fatalf("refresh account with quota: %v", err)
	}

	oauthCalls, kiroTokenCalls, codexQuotaCalls, _ := transport.snapshot()
	if oauthCalls != 0 {
		t.Fatalf("expected no oauth refresh calls for kiro, got %d", oauthCalls)
	}
	if kiroTokenCalls != 1 {
		t.Fatalf("expected one kiro token refresh call, got %d", kiroTokenCalls)
	}
	if codexQuotaCalls != 0 {
		t.Fatalf("expected no codex quota calls for kiro, got %d", codexQuotaCalls)
	}

	kiroQuotaCalls, kiroUserInfoCalls, kiroQuotaAuth, kiroUserInfoAuth := transport.snapshotKiro()
	if kiroQuotaCalls != 1 {
		t.Fatalf("expected one kiro quota call, got %d", kiroQuotaCalls)
	}
	if kiroUserInfoCalls != 2 {
		t.Fatalf("expected two kiro user info calls (refresh + quota), got %d", kiroUserInfoCalls)
	}
	if len(kiroQuotaAuth) != 1 || kiroQuotaAuth[0] != "Bearer kiro-access-new" {
		t.Fatalf("expected kiro quota auth header with refreshed token, got %v", kiroQuotaAuth)
	}
	if len(kiroUserInfoAuth) != 2 || kiroUserInfoAuth[0] != "Bearer kiro-access-new" || kiroUserInfoAuth[1] != "Bearer kiro-access-new" {
		t.Fatalf("expected kiro user info auth header with refreshed token, got %v", kiroUserInfoAuth)
	}

	if updated.Email != "kiro@example.com" {
		t.Fatalf("expected email updated from kiro user info, got %q", updated.Email)
	}
	if updated.Quota.Source != "kiro/getUsageLimits" {
		t.Fatalf("expected quota source kiro/getUsageLimits, got %q", updated.Quota.Source)
	}
	if updated.AccessToken != "kiro-access-new" {
		t.Fatalf("expected refreshed kiro access token, got %q", updated.AccessToken)
	}
	if len(updated.Quota.Buckets) == 0 {
		t.Fatalf("expected at least one quota bucket for kiro account")
	}
}

func TestRefreshQuota_KiroSkipsCodexTokenRefreshEvenWhenExpired(t *testing.T) {
	store, err := config.NewManager(t.TempDir())
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}

	now := time.Now().Unix()
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

	m := NewManager(store, logger.New(10))
	transport := &quotaRefreshMockTransport{
		kiroAccessToken:  "kiro-access-fresh",
		kiroRefreshToken: "kiro-refresh-fresh",
		kiroProfileARN:   "arn:aws:codewhisperer:us-east-1:123456789012:profile/ABC",
		kiroEmail:        "kiro-expired@example.com",
	}
	m.client = &http.Client{Transport: transport, Timeout: 5 * time.Second}

	updated, err := m.RefreshQuota(account.ID)
	if err != nil {
		t.Fatalf("refresh quota: %v", err)
	}

	oauthCalls, kiroTokenCalls, codexQuotaCalls, _ := transport.snapshot()
	if oauthCalls != 0 {
		t.Fatalf("expected no codex oauth refresh for kiro refresh quota, got %d", oauthCalls)
	}
	if kiroTokenCalls != 1 {
		t.Fatalf("expected one kiro token refresh, got %d", kiroTokenCalls)
	}
	if codexQuotaCalls != 0 {
		t.Fatalf("expected no codex quota endpoint for kiro refresh quota, got %d", codexQuotaCalls)
	}

	kiroQuotaCalls, _, kiroQuotaAuth, _ := transport.snapshotKiro()
	if kiroQuotaCalls != 1 {
		t.Fatalf("expected one kiro quota call, got %d", kiroQuotaCalls)
	}
	if len(kiroQuotaAuth) != 1 || kiroQuotaAuth[0] != "Bearer kiro-access-fresh" {
		t.Fatalf("expected refreshed kiro auth header, got %v", kiroQuotaAuth)
	}
	if updated.Quota.Source != "kiro/getUsageLimits" {
		t.Fatalf("expected kiro quota source, got %q", updated.Quota.Source)
	}
	if updated.AccessToken != "kiro-access-fresh" {
		t.Fatalf("expected refreshed kiro access token, got %q", updated.AccessToken)
	}
}

func TestRefreshQuota_KiroSocialRefreshWithoutClientCredentials(t *testing.T) {
	store, err := config.NewManager(t.TempDir())
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}

	now := time.Now().Unix()
	account := config.Account{
		ID:           "acct-kiro-social",
		Provider:     "kiro",
		Email:        "kiro-social@example.com",
		AccessToken:  "kiro-social-old",
		RefreshToken: "aorAAAAAG-social-refresh",
		ExpiresAt:    now - 60,
		Enabled:      true,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := store.UpsertAccount(account); err != nil {
		t.Fatalf("upsert account: %v", err)
	}

	m := NewManager(store, logger.New(10))
	transport := &quotaRefreshMockTransport{
		kiroAccessToken:  "kiro-social-new",
		kiroRefreshToken: "aorAAAAAG-social-new",
		kiroProfileARN:   "arn:aws:codewhisperer:us-east-1:123456789012:profile/SOCIAL",
		kiroEmail:        "kiro-social@example.com",
	}
	m.client = &http.Client{Transport: transport, Timeout: 5 * time.Second}

	updated, err := m.RefreshQuota(account.ID)
	if err != nil {
		t.Fatalf("refresh quota: %v", err)
	}

	_, kiroTokenCalls, _, _ := transport.snapshot()
	if kiroTokenCalls != 1 {
		t.Fatalf("expected one social kiro refresh call, got %d", kiroTokenCalls)
	}
	if updated.AccessToken != "kiro-social-new" {
		t.Fatalf("expected refreshed social access token, got %q", updated.AccessToken)
	}
	if updated.RefreshToken != "aorAAAAAG-social-new" {
		t.Fatalf("expected refreshed social refresh token, got %q", updated.RefreshToken)
	}
	if updated.AccountID != "arn:aws:codewhisperer:us-east-1:123456789012:profile/SOCIAL" {
		t.Fatalf("expected profile arn persisted, got %q", updated.AccountID)
	}
}

func TestRefreshAllQuotas_SkipsQuotaCooldownDisabledAndBannedAccounts(t *testing.T) {
	store, err := config.NewManager(t.TempDir())
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}

	now := time.Now().Unix()
	accounts := []config.Account{
		{
			ID:           "ready",
			Provider:     "codex",
			Email:        "ready@example.com",
			AccessToken:  "ready-access",
			RefreshToken: "ready-refresh",
			IDToken:      buildTestIDToken(now+3600, "ready@example.com", "acct-ready", "plus"),
			ExpiresAt:    now + 3600,
			Enabled:      true,
			CreatedAt:    now,
			UpdatedAt:    now,
		},
		{
			ID:           "cooldown-future",
			Provider:     "codex",
			Email:        "cooldown@example.com",
			AccessToken:  "cooldown-access",
			RefreshToken: "cooldown-refresh",
			IDToken:      buildTestIDToken(now+3600, "cooldown@example.com", "acct-cooldown", "plus"),
			ExpiresAt:    now + 3600,
			Enabled:      true,
			Quota: config.QuotaInfo{
				Status:        "exhausted",
				LastCheckedAt: now,
				Buckets:       []config.QuotaBucket{{Name: "session", Status: "exhausted", ResetAt: now + 3600}},
			},
			CreatedAt: now,
			UpdatedAt: now,
		},
		{
			ID:           "cooldown-expired",
			Provider:     "codex",
			Email:        "expired@example.com",
			AccessToken:  "expired-access",
			RefreshToken: "expired-refresh",
			IDToken:      buildTestIDToken(now+3600, "expired@example.com", "acct-expired", "plus"),
			ExpiresAt:    now + 3600,
			Enabled:      true,
			Quota: config.QuotaInfo{
				Status:        "exhausted",
				LastCheckedAt: now,
				Buckets:       []config.QuotaBucket{{Name: "session", Status: "exhausted", ResetAt: now - 60}},
			},
			CreatedAt: now,
			UpdatedAt: now,
		},
		{
			ID:           "disabled",
			Provider:     "codex",
			Email:        "disabled@example.com",
			AccessToken:  "disabled-access",
			RefreshToken: "disabled-refresh",
			IDToken:      buildTestIDToken(now+3600, "disabled@example.com", "acct-disabled", "plus"),
			ExpiresAt:    now + 3600,
			Enabled:      false,
			CreatedAt:    now,
			UpdatedAt:    now,
		},
		{
			ID:           "banned",
			Provider:     "codex",
			Email:        "banned@example.com",
			AccessToken:  "banned-access",
			RefreshToken: "banned-refresh",
			IDToken:      buildTestIDToken(now+3600, "banned@example.com", "acct-banned", "plus"),
			ExpiresAt:    now + 3600,
			Enabled:      true,
			Banned:       true,
			HealthState:  config.AccountHealthBanned,
			CreatedAt:    now,
			UpdatedAt:    now,
		},
	}
	for _, account := range accounts {
		if err := store.UpsertAccount(account); err != nil {
			t.Fatalf("upsert account %s: %v", account.ID, err)
		}
	}

	m := NewManager(store, logger.New(10))
	transport := &quotaRefreshMockTransport{}
	m.client = &http.Client{Transport: transport, Timeout: 5 * time.Second}

	if err := m.RefreshAllQuotas(); err != nil {
		t.Fatalf("refresh all quotas: %v", err)
	}

	_, _, quotaCalls, quotaAuthHeaders := transport.snapshot()
	if quotaCalls != 2 {
		t.Fatalf("expected 2 quota calls for eligible accounts, got %d", quotaCalls)
	}
	if len(quotaAuthHeaders) != 2 {
		t.Fatalf("expected 2 authorization headers, got %v", quotaAuthHeaders)
	}
}

func TestForceRefreshAllQuotas_IncludesPreviouslySkippedAccounts(t *testing.T) {
	store, err := config.NewManager(t.TempDir())
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}

	now := time.Now().Unix()
	accounts := []config.Account{
		{
			ID:           "ready",
			Provider:     "codex",
			Email:        "ready@example.com",
			AccessToken:  "ready-access",
			RefreshToken: "ready-refresh",
			IDToken:      buildTestIDToken(now+3600, "ready@example.com", "acct-ready", "plus"),
			ExpiresAt:    now + 3600,
			Enabled:      true,
			CreatedAt:    now,
			UpdatedAt:    now,
		},
		{
			ID:           "cooldown-future",
			Provider:     "codex",
			Email:        "cooldown@example.com",
			AccessToken:  "cooldown-access",
			RefreshToken: "cooldown-refresh",
			IDToken:      buildTestIDToken(now+3600, "cooldown@example.com", "acct-cooldown", "plus"),
			ExpiresAt:    now + 3600,
			Enabled:      true,
			Quota: config.QuotaInfo{
				Status:        "exhausted",
				LastCheckedAt: now,
				Buckets:       []config.QuotaBucket{{Name: "session", Status: "exhausted", ResetAt: now + 3600}},
			},
			CreatedAt: now,
			UpdatedAt: now,
		},
		{
			ID:           "disabled",
			Provider:     "codex",
			Email:        "disabled@example.com",
			AccessToken:  "disabled-access",
			RefreshToken: "disabled-refresh",
			IDToken:      buildTestIDToken(now+3600, "disabled@example.com", "acct-disabled", "plus"),
			ExpiresAt:    now + 3600,
			Enabled:      false,
			CreatedAt:    now,
			UpdatedAt:    now,
		},
		{
			ID:           "banned",
			Provider:     "codex",
			Email:        "banned@example.com",
			AccessToken:  "banned-access",
			RefreshToken: "banned-refresh",
			IDToken:      buildTestIDToken(now+3600, "banned@example.com", "acct-banned", "plus"),
			ExpiresAt:    now + 3600,
			Enabled:      true,
			Banned:       true,
			HealthState:  config.AccountHealthBanned,
			CreatedAt:    now,
			UpdatedAt:    now,
		},
	}
	for _, account := range accounts {
		if err := store.UpsertAccount(account); err != nil {
			t.Fatalf("upsert account %s: %v", account.ID, err)
		}
	}

	m := NewManager(store, logger.New(10))
	transport := &quotaRefreshMockTransport{}
	m.client = &http.Client{Transport: transport, Timeout: 5 * time.Second}

	if err := m.ForceRefreshAllQuotas(); err != nil {
		t.Fatalf("force refresh all quotas: %v", err)
	}

	_, _, quotaCalls, quotaAuthHeaders := transport.snapshot()
	if quotaCalls != 4 {
		t.Fatalf("expected 4 quota calls for forced refresh, got %d", quotaCalls)
	}
	if len(quotaAuthHeaders) != 4 {
		t.Fatalf("expected 4 authorization headers, got %v", quotaAuthHeaders)
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
