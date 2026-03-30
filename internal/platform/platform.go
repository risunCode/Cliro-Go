package platform

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

type contextKey string

const requestIDContextKey contextKey = "gateway_request_id"

func ProxyBindHost(allowLAN bool) string {
	if allowLAN {
		return "0.0.0.0"
	}
	return "127.0.0.1"
}

func ProxyURL(port int) string {
	return fmt.Sprintf("http://127.0.0.1:%d", port)
}

func ProxyBindAddress(allowLAN bool, port int) string {
	return fmt.Sprintf("%s:%d", ProxyBindHost(allowLAN), port)
}

func WithRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, requestIDContextKey, strings.TrimSpace(requestID))
}

func RequestIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	value, _ := ctx.Value(requestIDContextKey).(string)
	return strings.TrimSpace(value)
}

func JoinProxyBaseURL(baseURL string, endpointPath string) string {
	trimmedBase := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	trimmedPath := strings.TrimSpace(endpointPath)
	if trimmedPath == "" {
		return trimmedBase
	}
	if !strings.HasPrefix(trimmedPath, "/") {
		trimmedPath = "/" + trimmedPath
	}
	if strings.HasSuffix(strings.ToLower(trimmedBase), "/v1") && strings.HasPrefix(strings.ToLower(trimmedPath), "/v1") {
		trimmedBase = trimmedBase[:len(trimmedBase)-3]
	}
	return trimmedBase + trimmedPath
}

var ErrNotImplemented = errors.New("not implemented")

func InvalidRequest(message string) error {
	return fmt.Errorf("invalid request: %s", message)
}

type Logger interface {
	Info(module, message string)
	Error(module, message string)
	Debug(module, message string)
}
