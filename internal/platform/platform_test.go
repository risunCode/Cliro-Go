package platform

import (
	"context"
	"net/http/httptest"
	"testing"
)

func TestProxyURL_DefaultsToV1Base(t *testing.T) {
	if got := ProxyURL(8095); got != "http://127.0.0.1:8095/v1" {
		t.Fatalf("ProxyURL = %q", got)
	}
}

func TestProxyBindAddress_UsesLANFlag(t *testing.T) {
	if got := ProxyBindAddress(false, 8095); got != "127.0.0.1:8095" {
		t.Fatalf("bind address = %q", got)
	}
	if got := ProxyBindAddress(true, 8095); got != "0.0.0.0:8095" {
		t.Fatalf("bind address = %q", got)
	}
}

func TestRequestIDContextRoundTrip(t *testing.T) {
	ctx := WithRequestID(context.Background(), " req-123 ")
	if got := RequestIDFromContext(ctx); got != "req-123" {
		t.Fatalf("request id = %q", got)
	}
}

func TestApplyCommonProxyHeaders(t *testing.T) {
	recorder := httptest.NewRecorder()
	ApplyCommonProxyHeaders(recorder)
	if got := recorder.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Fatalf("allow origin = %q", got)
	}
	if got := recorder.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("cache control = %q", got)
	}
}
