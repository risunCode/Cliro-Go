package config

import (
	"encoding/json"
	"strings"
)

func BlockedAccountReason(message string) (string, bool) {
	normalizedMessage := normalizeBlockedAccountMessage(message)
	if normalizedMessage == "" {
		return "", false
	}

	value := strings.ToLower(normalizedMessage)
	blockIndicators := []string{
		"deactivated",
		"banned",
		"suspended",
		"disabled by",
		"terminated",
		"closed",
		"token invalidated",
		"invalidated token",
		"token was invalidated",
		"token has been invalidated",
		"auth revoked",
		"access revoked",
		"refresh token revoked",
		"revoked",
	}

	for _, indicator := range blockIndicators {
		if strings.Contains(value, indicator) {
			return normalizedMessage, true
		}
	}

	return "", false
}

func RefreshableAuthReason(message string) (string, bool) {
	normalizedMessage := normalizeBlockedAccountMessage(message)
	if normalizedMessage == "" {
		return "", false
	}

	value := strings.ToLower(normalizedMessage)
	refreshableIndicators := []string{
		"bearer token included in the request is invalid",
		"bearer token is invalid",
		"token included in the request is invalid",
		"invalid bearer token",
		"token expired",
		"session expired",
		"expired access token",
		"access token expired",
		"unauthorized",
	}

	for _, indicator := range refreshableIndicators {
		if strings.Contains(value, indicator) {
			return normalizedMessage, true
		}
	}

	return "", false
}

func normalizeBlockedAccountMessage(message string) string {
	trimmed := strings.TrimSpace(message)
	if trimmed == "" {
		return ""
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(trimmed), &payload); err == nil {
		if extracted := strings.TrimSpace(extractBlockedMessageField(payload)); extracted != "" {
			return extracted
		}
	}

	return trimmed
}

func extractBlockedMessageField(payload map[string]any) string {
	if payload == nil {
		return ""
	}
	if msg, ok := payload["message"].(string); ok && strings.TrimSpace(msg) != "" {
		return msg
	}
	if reason, ok := payload["reason"].(string); ok && strings.TrimSpace(reason) != "" {
		return reason
	}
	if nested, ok := payload["error"].(map[string]any); ok {
		if msg := extractBlockedMessageField(nested); msg != "" {
			return msg
		}
	}
	return ""
}

func AccountLabel(account Account) string {
	if email := strings.TrimSpace(account.Email); email != "" {
		return email
	}
	if accountID := strings.TrimSpace(account.AccountID); accountID != "" {
		return accountID
	}
	return strings.TrimSpace(account.ID)
}

func QuotaResetAt(quota QuotaInfo) int64 {
	var latest int64
	for _, bucket := range quota.Buckets {
		if bucket.ResetAt > latest {
			latest = bucket.ResetAt
		}
	}
	return latest
}
