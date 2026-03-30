package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"cliro-go/internal/config"

	"github.com/google/uuid"
)

const (
	codexQuotaBaseURL = "https://chatgpt.com/backend-api/codex"
	chatGPTBaseURL    = "https://chatgpt.com/backend-api"
	kiroQuotaBaseURL  = "https://codewhisperer.us-east-1.amazonaws.com"
	quotaFetchTimeout = 25 * time.Second
)

func (m *Manager) quotaRequestTimeout() time.Duration {
	return quotaFetchTimeout
}

func (m *Manager) applyQuotaSnapshot(accountID string, quota config.QuotaInfo) error {
	return m.store.UpdateAccount(accountID, func(a *config.Account) {
		a.Quota = quota
		if blockedMsg, blocked := blockedAccountMessageFromQuota(quota); blocked {
			a.Enabled = false
			a.Banned = true
			a.BannedReason = blockedMsg
			a.HealthState = config.AccountHealthBanned
			a.HealthReason = blockedMsg
			a.LastError = blockedMsg
			return
		}
		if shouldApplyQuotaCooldown(quota) {
			cooldownUntil := quotaResetAt(quota)
			if cooldownUntil <= time.Now().Unix() {
				cooldownUntil = time.Now().Add(time.Hour).Unix()
			}
			a.CooldownUntil = cooldownUntil
			a.HealthState = config.AccountHealthCooldownQuota
			a.HealthReason = firstNonEmpty(strings.TrimSpace(quota.Summary), "Quota exhausted")
			a.LastFailureAt = time.Now().Unix()
			if strings.TrimSpace(a.LastError) == "" {
				a.LastError = firstNonEmpty(strings.TrimSpace(quota.Summary), "Quota exhausted")
			}
		} else if a.HealthState == config.AccountHealthCooldownQuota {
			a.HealthState = config.AccountHealthReady
			a.HealthReason = ""
			a.CooldownUntil = 0
			a.ConsecutiveFailures = 0
		}
	})
}

func (m *Manager) RefreshAccountWithQuota(accountID string) (config.Account, error) {
	account, ok := m.store.GetAccount(accountID)
	if !ok {
		return config.Account{}, fmt.Errorf("account not found")
	}
	if err := validateQuotaProvider(account); err != nil {
		return account, err
	}

	refreshed, err := m.RefreshAccount(accountID)
	if err != nil {
		return refreshed, err
	}
	account = refreshed

	ctx, cancel := context.WithTimeout(context.Background(), m.quotaRequestTimeout())
	defer cancel()

	quota, quotaErr := m.fetchQuotaForAccount(ctx, account)
	if err := m.applyQuotaSnapshot(accountID, quota); err != nil {
		return account, err
	}

	updated, _ := m.store.GetAccount(accountID)
	return updated, quotaErr
}

func (m *Manager) RefreshQuota(accountID string) (config.Account, error) {
	account, ok := m.store.GetAccount(accountID)
	if !ok {
		return config.Account{}, fmt.Errorf("account not found")
	}
	if err := validateQuotaProvider(account); err != nil {
		return account, err
	}

	fresh, err := m.EnsureFreshAccount(accountID)
	if err != nil {
		quota := synthesizeQuota(account, err)
		blockedMsg, blocked := blockedAccountMessageFromError(err)
		_ = m.store.UpdateAccount(accountID, func(a *config.Account) {
			a.Quota = quota
			if blocked {
				a.Enabled = false
				a.Banned = true
				a.BannedReason = blockedMsg
				a.LastError = blockedMsg
			}
		})
		return account, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), m.quotaRequestTimeout())
	defer cancel()

	quota, quotaErr := m.fetchQuotaForAccount(ctx, fresh)
	if err := m.applyQuotaSnapshot(accountID, quota); err != nil {
		return fresh, err
	}
	updated, _ := m.store.GetAccount(accountID)
	return updated, quotaErr
}

func (m *Manager) fetchQuotaForAccount(ctx context.Context, account config.Account) (config.QuotaInfo, error) {
	if isKiroAccount(account) {
		return m.fetchKiroQuota(ctx, account)
	}
	if !isCodexAccount(account) {
		return config.QuotaInfo{}, fmt.Errorf("unsupported provider for quota refresh: %s", strings.TrimSpace(account.Provider))
	}
	return m.fetchQuota(ctx, account)
}

func isKiroAccount(account config.Account) bool {
	return strings.EqualFold(strings.TrimSpace(account.Provider), "kiro")
}

func isCodexAccount(account config.Account) bool {
	return strings.EqualFold(strings.TrimSpace(account.Provider), "codex")
}

func validateQuotaProvider(account config.Account) error {
	if isKiroAccount(account) || isCodexAccount(account) {
		return nil
	}
	provider := strings.TrimSpace(account.Provider)
	if provider == "" {
		return fmt.Errorf("account provider is required")
	}
	return fmt.Errorf("unsupported provider for quota refresh: %s", provider)
}

func (m *Manager) fetchKiroQuota(ctx context.Context, account config.Account) (config.QuotaInfo, error) {
	currentAccount := account
	for attempt := 0; attempt < 2; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, kiroQuotaBaseURL+"/getUsageLimits?origin=AI_EDITOR&resourceType=AGENTIC_REQUEST&isEmailRequired=true", nil)
		if err != nil {
			return synthesizeQuota(currentAccount, err), err
		}
		applyKiroRuntimeHeaders(req, currentAccount.AccessToken)

		resp, err := m.httpClient().Do(req)
		if err != nil {
			return synthesizeQuota(currentAccount, err), err
		}

		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			data, _ := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			if attempt == 0 {
				refreshedAccount, refreshErr := m.RefreshAccount(currentAccount.ID)
				if refreshErr == nil {
					currentAccount = refreshedAccount
					m.log.Info("quota", "Kiro quota token refreshed for "+firstNonEmpty(currentAccount.Email, currentAccount.ID))
					continue
				}
				m.log.Warn("quota", "Kiro quota refresh failed for "+firstNonEmpty(currentAccount.Email, currentAccount.ID)+": "+refreshErr.Error())
			}
			err = fmt.Errorf("kiro quota request failed (%d): %s", resp.StatusCode, compactHTTPBody(data))
			return synthesizeQuota(currentAccount, err), err
		}

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			data, _ := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			err = fmt.Errorf("kiro quota request failed (%d): %s", resp.StatusCode, compactHTTPBody(data))
			return synthesizeQuota(currentAccount, err), err
		}

		var payload struct {
			UsageBreakdownList []struct {
				CurrentUsage float64 `json:"currentUsage"`
				UsageLimit   float64 `json:"usageLimit"`
			} `json:"usageBreakdownList"`
			SubscriptionInfo struct {
				SubscriptionName  string `json:"subscriptionName"`
				SubscriptionTitle string `json:"subscriptionTitle"`
			} `json:"subscriptionInfo"`
			UserInfo struct {
				Email string `json:"email"`
			} `json:"userInfo"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
			_ = resp.Body.Close()
			return synthesizeQuota(currentAccount, err), err
		}
		_ = resp.Body.Close()

		used := 0
		total := 0
		if len(payload.UsageBreakdownList) > 0 {
			used = int(payload.UsageBreakdownList[0].CurrentUsage)
			total = int(payload.UsageBreakdownList[0].UsageLimit)
		}
		remaining := 0
		if total > 0 {
			remaining = maxInt(total-used, 0)
		}
		percent := 0
		if total > 0 {
			percent = int(float64(remaining) / float64(total) * 100)
		}

		bucket := config.QuotaBucket{
			Name:      "credits",
			Used:      used,
			Total:     total,
			Remaining: remaining,
			Percent:   percent,
			Status:    bucketStatus(config.QuotaBucket{Used: used, Total: total, Remaining: remaining}),
		}
		status := bucket.Status
		if status == "" {
			status = "healthy"
		}
		summary := firstNonEmpty(
			strings.TrimSpace(payload.SubscriptionInfo.SubscriptionTitle),
			strings.TrimSpace(payload.SubscriptionInfo.SubscriptionName),
		)
		if summary == "" && total > 0 {
			summary = fmt.Sprintf("%d/%d credits remaining", remaining, total)
		}
		if summary == "" {
			summary = "Kiro usage data loaded"
		}

		resolvedEmail := strings.TrimSpace(payload.UserInfo.Email)
		if resolvedEmail == "" {
			email, err := m.fetchKiroUserEmail(ctx, currentAccount)
			if err != nil {
				m.log.Warn("quota", "kiro user info lookup failed: "+err.Error())
			} else {
				resolvedEmail = strings.TrimSpace(email)
			}
		}

		if resolvedEmail != "" {
			_ = m.store.UpdateAccount(currentAccount.ID, func(a *config.Account) {
				a.Email = resolvedEmail
			})
		}

		return config.QuotaInfo{
			Status:        status,
			Summary:       summary,
			Source:        "kiro/getUsageLimits",
			LastCheckedAt: time.Now().Unix(),
			Buckets:       []config.QuotaBucket{bucket},
		}, nil
	}

	return synthesizeQuota(currentAccount, fmt.Errorf("kiro quota request failed")), fmt.Errorf("kiro quota request failed")
}

func (m *Manager) fetchKiroUserEmail(ctx context.Context, account config.Account) (string, error) {
	currentAccount := account
	for attempt := 0; attempt < 2; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, kiroQuotaBaseURL+"/GetUserInfo", strings.NewReader(`{"origin":"KIRO_IDE"}`))
		if err != nil {
			return "", err
		}
		req.Header.Set("Content-Type", "application/json")
		applyKiroRuntimeHeaders(req, currentAccount.AccessToken)

		resp, err := m.httpClient().Do(req)
		if err != nil {
			return "", err
		}

		data, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if err != nil {
			return "", err
		}
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			if attempt == 0 {
				refreshedAccount, refreshErr := m.RefreshAccount(currentAccount.ID)
				if refreshErr == nil {
					currentAccount = refreshedAccount
					continue
				}
			}
			return "", fmt.Errorf("kiro user info request failed (%d): %s", resp.StatusCode, compactHTTPBody(data))
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return "", fmt.Errorf("kiro user info request failed (%d): %s", resp.StatusCode, compactHTTPBody(data))
		}

		var payload struct {
			Email    string `json:"email"`
			UserInfo struct {
				Email string `json:"email"`
			} `json:"userInfo"`
		}
		if err := json.Unmarshal(data, &payload); err != nil {
			return "", err
		}

		resolved := strings.TrimSpace(firstNonEmpty(payload.Email, payload.UserInfo.Email))
		if resolved == "" {
			return "", fmt.Errorf("kiro user info response missing email")
		}
		return resolved, nil
	}

	return "", fmt.Errorf("kiro user info request failed")
}

func (m *Manager) RefreshAllQuotas() error {
	return m.refreshAllQuotas(false)
}

func (m *Manager) ForceRefreshAllQuotas() error {
	return m.refreshAllQuotas(true)
}

func (m *Manager) refreshAllQuotas(force bool) error {
	accounts := m.store.Accounts()
	if len(accounts) == 0 {
		return nil
	}

	now := time.Now().Unix()
	eligible := make([]config.Account, 0, len(accounts))
	skipped := map[string]int{}
	for _, account := range accounts {
		if !force {
			if skip, reason := shouldSkipBatchQuotaRefresh(account, now); skip {
				skipped[reason]++
				continue
			}
		}
		eligible = append(eligible, account)
	}

	if len(eligible) == 0 {
		m.logQuotaRefreshBatch(force, len(accounts), 0, skipped)
		return nil
	}

	workerCount := 4
	if workerCount <= 0 {
		workerCount = 1
	}
	if workerCount > len(eligible) {
		workerCount = len(eligible)
	}

	jobs := make(chan config.Account)
	failures := make([]string, 0)
	var failuresMu sync.Mutex
	var wg sync.WaitGroup

	for worker := 0; worker < workerCount; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for account := range jobs {
				if _, err := m.RefreshQuota(account.ID); err != nil {
					failuresMu.Lock()
					failures = append(failures, firstNonEmpty(account.Email, account.ID)+": "+err.Error())
					failuresMu.Unlock()
				}
			}
		}()
	}

	for _, account := range eligible {
		jobs <- account
	}
	close(jobs)
	wg.Wait()

	m.logQuotaRefreshBatch(force, len(accounts), len(eligible), skipped)

	if len(failures) > 0 {
		return fmt.Errorf(strings.Join(failures, "; "))
	}
	return nil
}

func shouldSkipBatchQuotaRefresh(account config.Account, now int64) (bool, string) {
	if account.Banned || account.HealthState == config.AccountHealthBanned {
		return true, "banned"
	}
	if !account.Enabled || account.HealthState == config.AccountHealthDisabledDurable {
		return true, "disabled"
	}
	if shouldApplyQuotaCooldown(account.Quota) {
		if resetAt := quotaResetAt(account.Quota); resetAt > now {
			return true, "quota_cooldown"
		}
	}
	return false, ""
}

func (m *Manager) logQuotaRefreshBatch(force bool, total int, eligible int, skipped map[string]int) {
	if m == nil || m.log == nil {
		return
	}
	mode := "smart"
	if force {
		mode = "force"
	}
	parts := []string{fmt.Sprintf("quota batch refresh mode=%s total=%d eligible=%d", mode, total, eligible)}
	if skipped["quota_cooldown"] > 0 {
		parts = append(parts, fmt.Sprintf("skipped_quota_cooldown=%d", skipped["quota_cooldown"]))
	}
	if skipped["disabled"] > 0 {
		parts = append(parts, fmt.Sprintf("skipped_disabled=%d", skipped["disabled"]))
	}
	if skipped["banned"] > 0 {
		parts = append(parts, fmt.Sprintf("skipped_banned=%d", skipped["banned"]))
	}
	m.log.Info("quota", strings.Join(parts, " "))
}

func (m *Manager) fetchQuota(ctx context.Context, account config.Account) (config.QuotaInfo, error) {
	endpoints := []struct {
		path   string
		source string
	}{
		{path: codexQuotaBaseURL + "/quotas", source: "codex/quotas"},
		{path: codexQuotaBaseURL + "/quota", source: "codex/quota"},
		{path: codexQuotaBaseURL + "/usage", source: "codex/usage"},
		{path: codexQuotaBaseURL + "/limits", source: "codex/limits"},
		{path: codexQuotaBaseURL + "/me", source: "codex/me"},
		{path: chatGPTBaseURL + "/me", source: "chatgpt/me"},
		{path: chatGPTBaseURL + "/accounts/check/v4-2023-04-27", source: "chatgpt/accounts/check"},
	}
	var lastErr error
	softFailuresOnly := true
	for _, endpoint := range endpoints {
		quota, err := m.tryQuotaEndpoint(ctx, account, endpoint.path, endpoint.source)
		if err == nil {
			return quota, nil
		}
		if blockedMsg, blocked := blockedAccountMessageFromError(err); blocked {
			quota := synthesizeQuota(account, fmt.Errorf("%s", blockedMsg))
			quota.Status = "deactivated"
			quota.Summary = blockedMsg
			quota.Error = blockedMsg
			return quota, err
		}
		lastErr = err
		if !isSoftQuotaDiscoveryErr(err) {
			softFailuresOnly = false
		}
	}

	if softFailuresOnly {
		if lastErr != nil {
			m.log.Warn("quota", "quota endpoint unavailable; using runtime snapshot: "+lastErr.Error())
		}
		return synthesizeQuota(account, nil), nil
	}

	quota := synthesizeQuota(account, lastErr)
	return quota, lastErr
}

func (m *Manager) tryQuotaEndpoint(ctx context.Context, account config.Account, endpoint, source string) (config.QuotaInfo, error) {
	originators := []string{"codex_cli_rs", "codex-cli", "openai_native"}
	var lastErr error

	for _, originator := range originators {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return config.QuotaInfo{}, err
		}
		applyCodexHeaders(req, account)
		req.Header.Set("Originator", originator)
		req.Header.Set("originator", originator)
		req.Header.Set("Accept", "application/json, text/plain, */*")
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", "https://chatgpt.com")
		req.Header.Set("Referer", "https://chatgpt.com/")
		req.Header.Set("Sec-Fetch-Dest", "empty")
		req.Header.Set("Sec-Fetch-Mode", "cors")
		req.Header.Set("Sec-Fetch-Site", "same-origin")

		resp, err := m.httpClient().Do(req)
		if err != nil {
			lastErr = err
			continue
		}

		body, readErr := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if readErr != nil {
			lastErr = readErr
			continue
		}

		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			lastErr = fmt.Errorf("quota request failed (%d): %s", resp.StatusCode, compactHTTPBody(body))
			continue
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			lastErr = fmt.Errorf("quota request failed (%d): %s", resp.StatusCode, compactHTTPBody(body))
			continue
		}
		if bytesContainChallenge(body) {
			lastErr = fmt.Errorf("quota endpoint blocked by challenge")
			continue
		}

		quota, err := parseQuotaPayload(body, source)
		if err != nil {
			lastErr = err
			continue
		}

		return quota, nil
	}

	if lastErr != nil {
		return config.QuotaInfo{}, lastErr
	}

	return config.QuotaInfo{}, fmt.Errorf("quota request failed: no response")
}

func applyCodexHeaders(req *http.Request, account config.Account) {
	req.Header.Set("Authorization", "Bearer "+account.AccessToken)
	req.Header.Set("Version", codexVersion)
	req.Header.Set("Session_id", uuid.NewString())
	req.Header.Set("User-Agent", codexUserAgent)
	req.Header.Set("Originator", "codex_cli_rs")
	req.Header.Set("originator", "codex_cli_rs")
	if strings.TrimSpace(account.AccountID) != "" {
		req.Header.Set("Chatgpt-Account-Id", account.AccountID)
	}
}

func parseQuotaPayload(body []byte, source string) (config.QuotaInfo, error) {
	var raw any
	if err := json.Unmarshal(body, &raw); err != nil {
		return config.QuotaInfo{}, err
	}

	if root, ok := raw.(map[string]any); ok {
		if parsed, ok := parseCodexUsagePayload(root, source); ok {
			return parsed, nil
		}
	}

	buckets := map[string]config.QuotaBucket{}
	collectQuotaBuckets("", raw, buckets)
	if len(buckets) == 0 {
		return config.QuotaInfo{}, fmt.Errorf("quota payload did not contain recognizable quota buckets")
	}

	out := make([]config.QuotaBucket, 0, len(buckets))
	for _, bucket := range buckets {
		bucket.Remaining = maxInt(bucket.Remaining, 0)
		if bucket.Total > 0 {
			if bucket.Remaining == 0 && bucket.Used > 0 {
				bucket.Remaining = maxInt(bucket.Total-bucket.Used, 0)
			}
			if bucket.Percent == 0 {
				bucket.Percent = int(float64(maxInt(bucket.Remaining, 0)) / float64(bucket.Total) * 100)
			}
			if bucket.Remaining <= 0 {
				bucket.Status = "exhausted"
			}
		}
		if bucket.Status == "" {
			bucket.Status = bucketStatus(bucket)
		}
		out = append(out, bucket)
	}

	status := aggregateQuotaStatus(out)

	return config.QuotaInfo{
		Status:        status,
		Summary:       fmt.Sprintf("%d quota bucket(s) detected", len(out)),
		Source:        source,
		LastCheckedAt: time.Now().Unix(),
		Buckets:       out,
	}, nil
}

func parseCodexUsagePayload(root map[string]any, source string) (config.QuotaInfo, bool) {
	rateLimitRaw, ok := root["rate_limit"]
	if !ok {
		return config.QuotaInfo{}, false
	}
	rateLimit, ok := rateLimitRaw.(map[string]any)
	if !ok {
		return config.QuotaInfo{}, false
	}

	buckets := make([]config.QuotaBucket, 0, 2)
	if primaryRaw, ok := rateLimit["primary_window"].(map[string]any); ok {
		usedPercent := maxInt(0, extractInt(primaryRaw, "used_percent", "usedPercent", "usage_percent"))
		if usedPercent > 100 {
			usedPercent = 100
		}
		bucket := config.QuotaBucket{
			Name:      "session",
			Used:      usedPercent,
			Total:     100,
			Remaining: maxInt(0, 100-usedPercent),
			Percent:   maxInt(0, 100-usedPercent),
			ResetAt:   extractTime(primaryRaw, "reset_at", "resets_at", "next_reset_at"),
		}
		bucket.Status = bucketStatus(bucket)
		buckets = append(buckets, bucket)
	}

	if secondaryRaw, ok := rateLimit["secondary_window"].(map[string]any); ok {
		usedPercent := maxInt(0, extractInt(secondaryRaw, "used_percent", "usedPercent", "usage_percent"))
		if usedPercent > 100 {
			usedPercent = 100
		}
		bucket := config.QuotaBucket{
			Name:      "weekly",
			Used:      usedPercent,
			Total:     100,
			Remaining: maxInt(0, 100-usedPercent),
			Percent:   maxInt(0, 100-usedPercent),
			ResetAt:   extractTime(secondaryRaw, "reset_at", "resets_at", "next_reset_at"),
		}
		bucket.Status = bucketStatus(bucket)
		buckets = append(buckets, bucket)
	}

	if len(buckets) == 0 {
		return config.QuotaInfo{}, false
	}

	summary := "Codex usage windows loaded"
	if limitReached, ok := rateLimit["limit_reached"].(bool); ok && limitReached {
		summary = "One or more Codex windows reached their limit"
	}

	return config.QuotaInfo{
		Status:        aggregateQuotaStatus(buckets),
		Summary:       summary,
		Source:        source,
		LastCheckedAt: time.Now().Unix(),
		Buckets:       buckets,
	}, true
}

func aggregateQuotaStatus(buckets []config.QuotaBucket) string {
	status := "healthy"
	for _, bucket := range buckets {
		switch strings.ToLower(strings.TrimSpace(bucket.Status)) {
		case "exhausted", "empty":
			return "exhausted"
		case "low":
			status = "low"
		}
	}
	return status
}

func collectQuotaBuckets(name string, value any, buckets map[string]config.QuotaBucket) {
	switch typed := value.(type) {
	case map[string]any:
		if bucket, ok := quotaBucketFromMap(name, typed); ok {
			buckets[bucket.Name] = bucket
		}
		for key, nested := range typed {
			nestedName := key
			if name != "" && !isQuotaKey(key) {
				nestedName = name
			}
			collectQuotaBuckets(nestedName, nested, buckets)
		}
	case []any:
		for _, item := range typed {
			collectQuotaBuckets(name, item, buckets)
		}
	}
}

func quotaBucketFromMap(name string, raw map[string]any) (config.QuotaBucket, bool) {
	total := extractInt(raw, "total", "limit", "max", "quota")
	remaining := extractInt(raw, "remaining", "left", "available")
	used := extractInt(raw, "used", "consumed", "current")
	percent := extractInt(raw, "percent", "percentage")
	resetAt := extractTime(raw, "reset_at", "resets_at", "next_reset_at")
	status := strings.ToLower(firstNonEmpty(extractString(raw, "status"), extractString(raw, "state")))
	bucketName := firstNonEmpty(name, extractString(raw, "name"), extractString(raw, "bucket"), extractString(raw, "kind"))
	if bucketName == "" {
		bucketName = "quota"
	}
	if total == 0 && remaining == 0 && used == 0 && percent == 0 && resetAt == 0 && status == "" {
		return config.QuotaBucket{}, false
	}
	return config.QuotaBucket{
		Name:      bucketName,
		Used:      used,
		Total:     total,
		Remaining: remaining,
		Percent:   percent,
		ResetAt:   resetAt,
		Status:    normalizeQuotaStatus(status),
	}, true
}

func synthesizeQuota(account config.Account, err error) config.QuotaInfo {
	now := time.Now().Unix()
	info := config.QuotaInfo{
		Status:        "healthy",
		Summary:       "Quota endpoint not resolved yet; using local runtime state.",
		Source:        "runtime",
		LastCheckedAt: now,
	}
	if err != nil {
		info.Error = err.Error()
		info.Status = "unknown"
	}
	if account.CooldownUntil > now {
		info.Status = "exhausted"
		info.Summary = firstNonEmpty(account.LastError, "Quota cooldown is active.")
		info.Buckets = []config.QuotaBucket{{
			Name:    "session",
			ResetAt: account.CooldownUntil,
			Status:  "exhausted",
		}, {
			Name:   "weekly",
			Status: "unknown",
		}}
		return info
	}
	if account.LastError != "" {
		info.Status = "degraded"
		info.Summary = account.LastError
	}
	if len(account.Quota.Buckets) > 0 {
		info.Buckets = append([]config.QuotaBucket(nil), account.Quota.Buckets...)
	}
	return info
}

func compactHTTPBody(body []byte) string {
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		return "empty response"
	}
	if len(trimmed) > 180 {
		return trimmed[:180] + "..."
	}
	return trimmed
}

func isSoftQuotaDiscoveryErr(err error) bool {
	if err == nil {
		return false
	}

	message := strings.ToLower(err.Error())
	if strings.Contains(message, "quota payload did not contain recognizable quota buckets") {
		return true
	}

	statusFragments := []string{
		"quota request failed (400)",
		"quota request failed (404)",
		"quota request failed (405)",
		"quota request failed (410)",
		"quota request failed (501)",
	}
	for _, fragment := range statusFragments {
		if strings.Contains(message, fragment) {
			return true
		}
	}

	return false
}

func bytesContainChallenge(body []byte) bool {
	text := strings.ToLower(string(body))
	return strings.Contains(text, "enable javascript and cookies to continue") || strings.Contains(text, "__cf_chl_opt")
}

func extractString(raw map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := raw[key]; ok {
			switch typed := value.(type) {
			case string:
				if strings.TrimSpace(typed) != "" {
					return typed
				}
			}
		}
	}
	return ""
}

func extractInt(raw map[string]any, keys ...string) int {
	for _, key := range keys {
		if value, ok := raw[key]; ok {
			switch typed := value.(type) {
			case float64:
				return int(typed)
			case int:
				return typed
			case int64:
				return int(typed)
			case string:
				if parsed, err := strconv.Atoi(strings.TrimSpace(typed)); err == nil {
					return parsed
				}
			}
		}
	}
	return 0
}

func extractTime(raw map[string]any, keys ...string) int64 {
	for _, key := range keys {
		if value, ok := raw[key]; ok {
			switch typed := value.(type) {
			case float64:
				return int64(typed)
			case int64:
				return typed
			case string:
				trimmed := strings.TrimSpace(typed)
				if parsed, err := strconv.ParseInt(trimmed, 10, 64); err == nil {
					return parsed
				}
				if parsed, err := time.Parse(time.RFC3339, trimmed); err == nil {
					return parsed.Unix()
				}
			}
		}
	}
	return 0
}

func normalizeQuotaStatus(status string) string {
	status = strings.ToLower(strings.TrimSpace(status))
	switch status {
	case "ready", "healthy", "ok":
		return "healthy"
	case "expiring", "warning":
		return "low"
	case "expired", "exhausted", "quota_exceeded", "insufficient_quota":
		return "exhausted"
	case "", "unknown":
		return ""
	default:
		return status
	}
}

func bucketStatus(bucket config.QuotaBucket) string {
	if bucket.Status != "" {
		status := normalizeQuotaStatus(bucket.Status)
		if status != "" {
			return status
		}
		return strings.ToLower(strings.TrimSpace(bucket.Status))
	}
	now := time.Now().Unix()
	if bucket.Total > 0 {
		remaining := bucket.Remaining
		if remaining == 0 && bucket.Used > 0 && bucket.Used <= bucket.Total {
			remaining = maxInt(bucket.Total-bucket.Used, 0)
		}
		if remaining <= 0 {
			return "exhausted"
		}
		remainingPercent := int(float64(remaining) / float64(bucket.Total) * 100)
		if remainingPercent <= 15 {
			return "low"
		}
		return "healthy"
	}
	if bucket.ResetAt > now {
		if bucket.Remaining <= 0 {
			return "exhausted"
		}
		return "low"
	}
	return "unknown"
}

func blockedAccountMessageFromQuota(quota config.QuotaInfo) (string, bool) {
	sourceMessage := firstNonEmpty(strings.TrimSpace(quota.Error), strings.TrimSpace(quota.Summary))
	if sourceMessage == "" {
		return "", false
	}
	return blockedAccountMessageFromString(sourceMessage)
}

func blockedAccountMessageFromError(err error) (string, bool) {
	if err == nil {
		return "", false
	}
	return blockedAccountMessageFromString(err.Error())
}

func blockedAccountMessageFromString(message string) (string, bool) {
	return config.BlockedAccountReason(message)
}

func shouldApplyQuotaCooldown(quota config.QuotaInfo) bool {
	status := strings.ToLower(strings.TrimSpace(quota.Status))
	if status == "exhausted" || status == "empty" {
		return true
	}

	for _, bucket := range quota.Buckets {
		bucketStatus := strings.ToLower(strings.TrimSpace(bucket.Status))
		if bucketStatus == "exhausted" || bucketStatus == "empty" {
			return true
		}
		if bucket.Total > 0 {
			remaining := bucket.Remaining
			if remaining == 0 && bucket.Used > 0 && bucket.Used <= bucket.Total {
				remaining = maxInt(bucket.Total-bucket.Used, 0)
			}
			if remaining <= 0 {
				return true
			}
		}
	}

	return false
}

func quotaResetAt(quota config.QuotaInfo) int64 {
	var latest int64
	for _, bucket := range quota.Buckets {
		if bucket.ResetAt > latest {
			latest = bucket.ResetAt
		}
	}
	return latest
}

func isQuotaKey(key string) bool {
	key = strings.ToLower(key)
	return key == "session" || key == "weekly" || key == "daily" || key == "monthly" || key == "quota" || key == "quotas" || key == "limits"
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
