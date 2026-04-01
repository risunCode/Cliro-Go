package gateway

import (
	"crypto/subtle"
	"fmt"
	"net/http"
	"strings"
)

type APIError struct {
	Status  int
	Type    string
	Message string
}

func (e APIError) Error() string {
	return e.Message
}

func InvalidRequest(message string) APIError {
	return APIError{Status: http.StatusBadRequest, Type: "invalid_request_error", Message: message}
}

func ServerError(message string) APIError {
	return APIError{Status: http.StatusInternalServerError, Type: "server_error", Message: message}
}

func Unauthorized(message string) APIError {
	return APIError{Status: http.StatusUnauthorized, Type: "authentication_error", Message: message}
}

func Forbidden(message string) APIError {
	return APIError{Status: http.StatusForbidden, Type: "permission_error", Message: message}
}

func (s *Server) validateSecurityHeaders(r *http.Request) APIError {
	if r == nil {
		return InvalidRequest("request is required")
	}
	if s.store == nil {
		return ServerError("store unavailable")
	}

	configuredKey := strings.TrimSpace(s.store.ProxyAPIKey())
	providedKey, err := resolveProxyCredential(r)
	if err != nil {
		return InvalidRequest(err.Error())
	}

	if !s.store.AuthorizationMode() {
		return APIError{}
	}
	if configuredKey == "" {
		return Forbidden("authorization mode enabled but proxy API key is not configured")
	}
	if providedKey == "" {
		return Unauthorized("missing proxy API key")
	}
	if subtle.ConstantTimeCompare([]byte(providedKey), []byte(configuredKey)) != 1 {
		return Unauthorized("invalid proxy API key")
	}
	return APIError{}
}

func resolveProxyCredential(r *http.Request) (string, error) {
	if r == nil {
		return "", nil
	}
	authorizationHeader := strings.TrimSpace(r.Header.Get("Authorization"))
	xAPIKey := strings.TrimSpace(r.Header.Get("X-API-Key"))

	resolvedBearer := ""
	if authorizationHeader != "" {
		parts := strings.Fields(authorizationHeader)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || strings.TrimSpace(parts[1]) == "" {
			return "", fmt.Errorf("malformed Authorization header")
		}
		resolvedBearer = strings.TrimSpace(parts[1])
	}

	if resolvedBearer != "" && xAPIKey != "" && subtle.ConstantTimeCompare([]byte(resolvedBearer), []byte(xAPIKey)) != 1 {
		return "", fmt.Errorf("conflicting Authorization and X-API-Key headers")
	}
	if resolvedBearer != "" {
		return resolvedBearer, nil
	}
	return xAPIKey, nil
}
