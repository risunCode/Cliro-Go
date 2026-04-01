package platform

import (
	"context"
	"fmt"
	"net/http"
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
	return fmt.Sprintf("http://127.0.0.1:%d/v1", port)
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

func ApplyCommonProxyHeaders(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-API-Key, X-Request-ID, Anthropic-Version")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Expose-Headers", "Content-Type, X-Request-ID")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
}

func EnsureProtocolRegistered() (bool, error) {
	return ensureProtocolRegistered()
}
