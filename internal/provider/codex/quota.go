package codex

import (
	"cliro-go/internal/util"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"cliro-go/internal/config"
	provider "cliro-go/internal/provider"

	"github.com/google/uuid"
)

const (
	codexQuotaBaseURL = "https://chatgpt.com/backend-api/codex"
	chatGPTBaseURL    = "https://chatgpt.com/backend-api"
)

type QuotaFetcher struct {
	httpClient *http.Client
}

func NewQuotaFetcher(httpClient *http.Client) *QuotaFetcher {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 25 * time.Second}
	}
	return &QuotaFetcher{httpClient: httpClient}
}

func (f *QuotaFetcher) FetchQuota(ctx context.Context, account config.Account) (config.QuotaInfo, error) {
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
		quota, err := f.tryQuotaEndpoint(ctx, account, endpoint.path, endpoint.source)
		if err == nil {
			return quota, nil
		}
		if blockedMsg, blocked := blockedAccountMessageFromError(err); blocked {
			quota := provider.SynthesizeQuota(account, fmt.Errorf("%s", blockedMsg))
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
		return provider.SynthesizeQuota(account, nil), nil
	}

	quota := provider.SynthesizeQuota(account, lastErr)
	return quota, lastErr
}

func (f *QuotaFetcher) tryQuotaEndpoint(ctx context.Context, account config.Account, endpoint, source string) (config.QuotaInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return config.QuotaInfo{}, err
	}
	applyCodexHeaders(req, account)
	req.Header.Set("Originator", "opencode")
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "https://chatgpt.com")
	req.Header.Set("Referer", "https://chatgpt.com/")
	req.Header.Set("Sec-Fetch-Dest", "empty")
	req.Header.Set("Sec-Fetch-Mode", "cors")
	req.Header.Set("Sec-Fetch-Site", "same-origin")

	resp, err := f.httpClient.Do(req)
	if err != nil {
		return config.QuotaInfo{}, err
	}

	body, readErr := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if readErr != nil {
		return config.QuotaInfo{}, readErr
	}

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return config.QuotaInfo{}, fmt.Errorf("quota request failed (%d): %s", resp.StatusCode, provider.CompactHTTPBody(body))
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return config.QuotaInfo{}, fmt.Errorf("quota request failed (%d): %s", resp.StatusCode, provider.CompactHTTPBody(body))
	}
	if bytesContainChallenge(body) {
		return config.QuotaInfo{}, fmt.Errorf("quota endpoint blocked by challenge")
	}

	quota, err := parseQuotaPayload(body, source)
	if err != nil {
		return config.QuotaInfo{}, err
	}

	return quota, nil
}

func applyCodexHeaders(req *http.Request, account config.Account) {
	req.Header.Set("Authorization", "Bearer "+account.AccessToken)
	req.Header.Set("Session_id", uuid.NewString())
	req.Header.Set("User-Agent", codexUserAgent)
	req.Header.Set("Originator", "opencode")
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
			bucket.Status = provider.BucketStatus(bucket)
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
		bucket.Status = provider.BucketStatus(bucket)
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
		bucket.Status = provider.BucketStatus(bucket)
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
	status := strings.ToLower(util.FirstNonEmpty(extractString(raw, "status"), extractString(raw, "state")))
	bucketName := util.FirstNonEmpty(name, extractString(raw, "name"), extractString(raw, "bucket"), extractString(raw, "kind"))
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
		Status:    provider.NormalizeQuotaStatus(status),
	}, true
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

func blockedAccountMessageFromError(err error) (string, bool) {
	if err == nil {
		return "", false
	}
	return config.BlockedAccountReason(err.Error())
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
