package platform

import (
	"context"
	"strings"
)

type contextKey string

const requestIDContextKey contextKey = "gateway_request_id"

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
