package provider

import (
	"net/http"
	"strings"
	"time"

	"cliro-go/internal/config"
)

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
