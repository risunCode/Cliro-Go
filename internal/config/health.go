package config

import (
	"encoding/json"
	"strconv"
	"strings"
)

func BlockedAccountReason(message string) (string, bool) {
	signal := parseAuthErrorSignal(message)
	for _, candidate := range []string{signal.Code, signal.Reason, signal.Kind} {
		if isBlockedAuthCode(candidate) {
			return authReasonMessage(signal, "Account deactivated"), true
		}
	}

	return "", false
}

func RefreshableAuthReason(message string) (string, bool) {
	signal := parseAuthErrorSignal(message)
	for _, candidate := range []string{signal.Code, signal.Reason} {
		if isRefreshableAuthCode(candidate) {
			return authReasonMessage(signal, "Authentication required"), true
		}
	}

	if signal.Status == 401 || signal.Status == 403 {
		return authReasonMessage(signal, "Authentication required"), true
	}

	return "", false
}

type authErrorSignal struct {
	Status  int
	Message string
	Code    string
	Kind    string
	Reason  string
}

func parseAuthErrorSignal(message string) authErrorSignal {
	trimmed := strings.TrimSpace(message)
	if trimmed == "" {
		return authErrorSignal{}
	}

	signal := authErrorSignal{Status: extractStatusCode(trimmed)}

	if payloadText := extractInlineJSON(trimmed); payloadText != "" {
		var payload map[string]any
		if err := json.Unmarshal([]byte(payloadText), &payload); err == nil {
			mergeAuthSignalFromPayload(&signal, payload)
		}
	}

	if signal.Code == "" {
		signal.Code = normalizeAuthCode(extractInlineCode(trimmed))
	}
	if signal.Message == "" {
		signal.Message = compactAuthMessage(extractInlineMessage(trimmed))
	}
	if signal.Message != "" {
		signal.Message = stripLeadingAuthCode(signal.Message, signal.Code)
	}

	return signal
}

func mergeAuthSignalFromPayload(signal *authErrorSignal, payload map[string]any) {
	if signal == nil || payload == nil {
		return
	}

	if signal.Message == "" {
		signal.Message = compactAuthMessage(extractStringField(payload, "message", "error_description", "detail", "title"))
	}
	if signal.Code == "" {
		signal.Code = normalizeAuthCode(extractStringField(payload, "code", "error_code"))
	}
	if signal.Kind == "" {
		signal.Kind = normalizeAuthCode(extractStringField(payload, "type", "error_type"))
	}
	if signal.Reason == "" {
		signal.Reason = normalizeAuthCode(extractStringField(payload, "reason"))
	}

	if nested, ok := payload["error"].(map[string]any); ok {
		mergeAuthSignalFromPayload(signal, nested)
	}

	if rawErrors, ok := payload["errors"].([]any); ok {
		for _, raw := range rawErrors {
			if nested, ok := raw.(map[string]any); ok {
				mergeAuthSignalFromPayload(signal, nested)
			}
		}
	}
}

func extractStringField(payload map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := payload[key]; ok {
			if text, ok := value.(string); ok && strings.TrimSpace(text) != "" {
				return strings.TrimSpace(text)
			}
		}
	}
	return ""
}

func extractInlineJSON(value string) string {
	start := strings.Index(value, "{")
	end := strings.LastIndex(value, "}")
	if start < 0 || end <= start {
		return ""
	}
	return strings.TrimSpace(value[start : end+1])
}

func extractInlineMessage(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	if idx := strings.Index(trimmed, ":"); idx >= 0 {
		candidate := strings.TrimSpace(trimmed[idx+1:])
		if candidate != "" && !strings.HasPrefix(candidate, "{") {
			return candidate
		}
	}
	return trimmed
}

func extractInlineCode(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}

	lower := strings.ToLower(trimmed)
	index := strings.Index(lower, "\"code\"")
	if index < 0 {
		index = strings.Index(lower, "code:")
	}
	if index >= 0 {
		segment := trimmed[index:]
		for _, separator := range []string{":", "="} {
			if sep := strings.Index(segment, separator); sep >= 0 {
				candidate := strings.TrimSpace(segment[sep+1:])
				candidate = strings.Trim(candidate, `"'{}[] ,`)
				parts := strings.Fields(candidate)
				if len(parts) == 0 {
					continue
				}
				normalized := normalizeAuthCode(parts[0])
				if isKnownAuthCode(normalized) {
					return normalized
				}
			}
		}
	}

	segments := strings.Split(trimmed, ":")
	if len(segments) > 6 {
		segments = segments[:6]
	}
	for _, segment := range segments {
		candidate := strings.TrimSpace(segment)
		if candidate == "" {
			continue
		}
		candidate = strings.Trim(candidate, `"'{}[]() ,`)
		normalized := normalizeAuthCode(candidate)
		if isKnownAuthCode(normalized) {
			return normalized
		}
		fields := strings.Fields(normalized)
		if len(fields) > 0 {
			tail := fields[len(fields)-1]
			if isKnownAuthCode(tail) {
				return tail
			}
		}
	}

	return ""
}

func stripLeadingAuthCode(message string, code string) string {
	trimmed := strings.TrimSpace(message)
	if trimmed == "" {
		return ""
	}
	normalizedCode := normalizeAuthCode(code)
	if normalizedCode == "" {
		return trimmed
	}

	parts := strings.SplitN(trimmed, ":", 2)
	if len(parts) != 2 {
		return trimmed
	}
	prefix := normalizeAuthCode(strings.TrimSpace(parts[0]))
	if prefix != normalizedCode {
		return trimmed
	}
	cleaned := strings.TrimSpace(parts[1])
	if cleaned == "" {
		return trimmed
	}
	return cleaned
}

func isKnownAuthCode(code string) bool {
	normalized := normalizeAuthCode(code)
	if normalized == "" {
		return false
	}
	return isBlockedAuthCode(normalized) || isRefreshableAuthCode(normalized)
}

func extractStatusCode(value string) int {
	open := strings.LastIndex(value, "(")
	close := strings.LastIndex(value, ")")
	if open < 0 || close <= open {
		return 0
	}
	code := strings.TrimSpace(value[open+1 : close])
	if len(code) != 3 {
		return 0
	}
	parsed, err := strconv.Atoi(code)
	if err != nil {
		return 0
	}
	return parsed
}

func normalizeAuthCode(code string) string {
	normalized := strings.TrimSpace(strings.ToLower(code))
	if normalized == "" {
		return ""
	}
	normalized = strings.NewReplacer("-", "_", " ", "_").Replace(normalized)
	for strings.Contains(normalized, "__") {
		normalized = strings.ReplaceAll(normalized, "__", "_")
	}
	return normalized
}

func isBlockedAuthCode(code string) bool {
	switch normalizeAuthCode(code) {
	case "account_deactivated", "account_disabled", "account_suspended", "account_banned", "user_deactivated", "user_disabled", "organization_deactivated", "org_deactivated", "account_terminated":
		return true
	default:
		return false
	}
}

func isRefreshableAuthCode(code string) bool {
	switch normalizeAuthCode(code) {
	case "refresh_token_reused", "refresh_token_invalid", "invalid_grant", "invalid_token", "expired_token", "token_expired", "invalid_api_key", "authentication_required", "auth_required":
		return true
	default:
		return false
	}
}

func humanizeAuthCode(code string) string {
	normalized := normalizeAuthCode(code)
	if normalized == "" {
		return ""
	}
	return strings.ReplaceAll(normalized, "_", " ")
}

func authReasonMessage(signal authErrorSignal, fallback string) string {
	if signal.Message != "" {
		return signal.Message
	}
	if humanized := humanizeAuthCode(signal.Code); humanized != "" {
		return humanized
	}
	if humanized := humanizeAuthCode(signal.Reason); humanized != "" {
		return humanized
	}
	if humanized := humanizeAuthCode(signal.Kind); humanized != "" {
		return humanized
	}
	if trimmed := strings.TrimSpace(fallback); trimmed != "" {
		return trimmed
	}
	return "Authentication required"
}

func compactAuthMessage(message string) string {
	normalized := strings.TrimSpace(message)
	if normalized == "" {
		return ""
	}
	if len(normalized) > 180 {
		return normalized[:180] + "..."
	}
	return normalized
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
