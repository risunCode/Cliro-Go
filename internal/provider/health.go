package provider

import (
	"cliro-go/internal/util"
	"net/http"
	"strings"
	"time"

	"cliro-go/internal/config"
)

var transientBackoffSeconds = []int{10, 30, 60}

func TransientCooldown(failureCount int) time.Duration {
	if failureCount <= 0 || len(transientBackoffSeconds) == 0 {
		return 0
	}
	index := failureCount - 1
	if index >= len(transientBackoffSeconds) {
		index = len(transientBackoffSeconds) - 1
	}
	seconds := transientBackoffSeconds[index]
	if seconds <= 0 {
		return 0
	}
	return time.Duration(seconds) * time.Second
}

type FailureClass string

const (
	FailureRetryableTransport FailureClass = "retryable_transport"
	FailureAuthRefreshable    FailureClass = "auth_refreshable"
	FailureQuotaCooldown      FailureClass = "quota_cooldown"
	FailureDurableDisabled    FailureClass = "durable_disabled"
	FailureRequestShape       FailureClass = "request_shape"
	FailureProviderFatal      FailureClass = "provider_fatal"
)

type FailureDecision struct {
	Class        FailureClass
	Message      string
	Cooldown     time.Duration
	RetryAllowed bool
	BanAccount   bool
	Disable      bool
	Status       int
}

func ClassifyHTTPFailure(status int, message string) FailureDecision {
	trimmedMessage := strings.TrimSpace(message)
	if trimmedMessage == "" {
		trimmedMessage = http.StatusText(status)
	}

	if blockedMessage, blocked := config.BlockedAccountReason(trimmedMessage); blocked {
		return FailureDecision{Class: FailureDurableDisabled, Message: blockedMessage, BanAccount: true, Disable: true, Status: http.StatusUnauthorized}
	}
	if refreshableMessage, refreshable := config.RefreshableAuthReason(trimmedMessage); refreshable && (status == http.StatusUnauthorized || status == http.StatusForbidden) {
		return FailureDecision{Class: FailureAuthRefreshable, Message: refreshableMessage, RetryAllowed: true, Status: http.StatusUnauthorized}
	}

	lowerMessage := strings.ToLower(trimmedMessage)
	if status == http.StatusTooManyRequests || strings.Contains(lowerMessage, "usage_limit_reached") || strings.Contains(lowerMessage, "rate limit") || strings.Contains(lowerMessage, "quota") {
		return FailureDecision{Class: FailureQuotaCooldown, Message: trimmedMessage, Cooldown: time.Hour, Status: http.StatusTooManyRequests}
	}

	if status == http.StatusUnauthorized || status == http.StatusForbidden {
		return FailureDecision{Class: FailureAuthRefreshable, Message: trimmedMessage, Cooldown: 30 * time.Second, Status: http.StatusUnauthorized}
	}

	if isRequestShapeFailure(status, lowerMessage) {
		return FailureDecision{Class: FailureRequestShape, Message: trimmedMessage, Status: http.StatusBadGateway}
	}

	if isRetryableHTTPStatus(status) {
		return FailureDecision{Class: FailureRetryableTransport, Message: trimmedMessage, Cooldown: 15 * time.Second, RetryAllowed: true, Status: http.StatusBadGateway}
	}

	return FailureDecision{Class: FailureProviderFatal, Message: trimmedMessage, Cooldown: 30 * time.Second, Status: http.StatusBadGateway}
}

func ClassifyTransportFailure(err error) FailureDecision {
	message := "transport error"
	if err != nil {
		message = strings.TrimSpace(err.Error())
	}
	return FailureDecision{Class: FailureRetryableTransport, Message: message, Cooldown: 15 * time.Second, RetryAllowed: true, Status: http.StatusBadGateway}
}

// SynthesizeQuota derives a quota snapshot from runtime account state when
// the upstream quota endpoint is unavailable or inconclusive.
func SynthesizeQuota(account config.Account, err error) config.QuotaInfo {
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
		info.Summary = util.FirstNonEmpty(strings.TrimSpace(account.LastError), "Quota cooldown is active.")
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

// NormalizeQuotaStatus maps provider-specific quota labels into canonical local statuses.
func NormalizeQuotaStatus(status string) string {
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

// BucketStatus derives a canonical status for a quota bucket when the source status is missing or inconsistent.
func BucketStatus(bucket config.QuotaBucket) string {
	if bucket.Status != "" {
		status := NormalizeQuotaStatus(bucket.Status)
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

// CompactHTTPBody trims and truncates upstream HTTP bodies for safe error messages.
func CompactHTTPBody(body []byte) string {
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		return "empty response"
	}
	if len(trimmed) > 180 {
		return trimmed[:180] + "..."
	}
	return trimmed
}

func isRetryableHTTPStatus(status int) bool {
	switch status {
	case http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true
	default:
		return false
	}
}

func isRequestShapeFailure(status int, lowerMessage string) bool {
	if status == http.StatusBadRequest || status == http.StatusUnprocessableEntity {
		return true
	}
	requestShapeIndicators := []string{
		"invalid value",
		"unsupported values",
		"unsupported value",
		"unsupported field",
		"invalid request",
		"malformed",
		"does not support",
		"unsupported parameter",
		"invalid type",
	}
	for _, indicator := range requestShapeIndicators {
		if strings.Contains(lowerMessage, indicator) {
			return true
		}
	}
	return false
}


func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
