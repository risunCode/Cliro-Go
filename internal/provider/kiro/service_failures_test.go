package kiro

import (
	"net/http"
	"testing"

	provider "cliro-go/internal/provider"
)

func TestClassifyHTTPFailure_ImproperlyFormedRequestIsRequestShape(t *testing.T) {
	decision := classifyHTTPFailure(http.StatusBadRequest, []byte(`{"message":"Improperly formed request"}`))
	if decision.Class != provider.FailureRequestShape {
		t.Fatalf("class = %q", decision.Class)
	}
	if decision.Status != http.StatusBadRequest {
		t.Fatalf("status = %d", decision.Status)
	}
}

func TestClassifyHTTPFailure_QuotaMessageTriggersCooldown(t *testing.T) {
	decision := classifyHTTPFailure(http.StatusTooManyRequests, []byte(`{"message":"Usage limit reached"}`))
	if decision.Class != provider.FailureQuotaCooldown {
		t.Fatalf("class = %q", decision.Class)
	}
	if decision.Status != http.StatusTooManyRequests {
		t.Fatalf("status = %d", decision.Status)
	}
}

func TestClassifyTransportFailure_FirstTokenTimeoutIsRetryable(t *testing.T) {
	decision := classifyTransportFailure(ErrFirstTokenTimeout)
	if decision.Class != provider.FailureRetryableTransport {
		t.Fatalf("class = %q", decision.Class)
	}
	if !decision.RetryAllowed {
		t.Fatalf("expected retryable timeout decision")
	}
	if decision.Status != http.StatusGatewayTimeout {
		t.Fatalf("status = %d", decision.Status)
	}
}
