package provider

import (
	"errors"
	"net/http"
	"testing"
	"time"
)

func TestClassifyHTTPFailure(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		message string
		class   FailureClass
		resp    int
	}{
		{
			name:    "refreshable auth",
			status:  http.StatusUnauthorized,
			message: `{"message":"The bearer token included in the request is invalid.","reason":null}`,
			class:   FailureAuthRefreshable,
			resp:    http.StatusUnauthorized,
		},
		{
			name:    "request shape",
			status:  http.StatusBadRequest,
			message: "Invalid value: 'input_text'. Supported values are: 'output_text' and 'refusal'.",
			class:   FailureRequestShape,
			resp:    http.StatusBadGateway,
		},
		{
			name:    "quota cooldown",
			status:  http.StatusTooManyRequests,
			message: "rate limit reached",
			class:   FailureQuotaCooldown,
			resp:    http.StatusTooManyRequests,
		},
		{
			name:    "retryable upstream",
			status:  http.StatusServiceUnavailable,
			message: "upstream unavailable",
			class:   FailureRetryableTransport,
			resp:    http.StatusBadGateway,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			decision := ClassifyHTTPFailure(tc.status, tc.message)
			if decision.Class != tc.class {
				t.Fatalf("class = %q", decision.Class)
			}
			if decision.Status != tc.resp {
				t.Fatalf("status = %d", decision.Status)
			}
		})
	}
}

func TestClassifyTransportFailure(t *testing.T) {
	tests := []struct {
		name  string
		err   error
		class FailureClass
		resp  int
	}{
		{name: "nil error", class: FailureRetryableTransport, resp: http.StatusBadGateway},
		{name: "transport timeout", err: errors.New("timeout contacting upstream"), class: FailureRetryableTransport, resp: http.StatusBadGateway},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			decision := ClassifyTransportFailure(tc.err)
			if decision.Class != tc.class {
				t.Fatalf("class = %q", decision.Class)
			}
			if decision.Status != tc.resp {
				t.Fatalf("status = %d", decision.Status)
			}
		})
	}
}

func TestTransientCooldown_UsesFixedPolicy(t *testing.T) {
	if got := TransientCooldown(1); got != 10*time.Second {
		t.Fatalf("first cooldown = %s", got)
	}
	if got := TransientCooldown(2); got != 30*time.Second {
		t.Fatalf("second cooldown = %s", got)
	}
	if got := TransientCooldown(5); got != 60*time.Second {
		t.Fatalf("overflow cooldown = %s", got)
	}
}
