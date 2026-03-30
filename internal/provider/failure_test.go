package provider

import (
	"net/http"
	"testing"
	"time"
)

func TestClassifyHTTPFailure_InvalidBearerTokenIsRefreshable(t *testing.T) {
	decision := ClassifyHTTPFailure(http.StatusUnauthorized, `{"message":"The bearer token included in the request is invalid.","reason":null}`)
	if decision.Class != FailureAuthRefreshable {
		t.Fatalf("class = %q", decision.Class)
	}
	if !decision.RetryAllowed {
		t.Fatalf("expected retryable auth refresh decision")
	}
}

func TestClassifyHTTPFailure_InvalidValueIsRequestShape(t *testing.T) {
	decision := ClassifyHTTPFailure(http.StatusBadRequest, "Invalid value: 'input_text'. Supported values are: 'output_text' and 'refusal'.")
	if decision.Class != FailureRequestShape {
		t.Fatalf("class = %q", decision.Class)
	}
	if decision.Status != http.StatusBadGateway {
		t.Fatalf("status = %d", decision.Status)
	}
}

func TestCircuitCooldown_UsesConfiguredSteps(t *testing.T) {
	if got := CircuitCooldown([]int{10, 30, 60}, 1); got != 10*time.Second {
		t.Fatalf("first cooldown = %s", got)
	}
	if got := CircuitCooldown([]int{10, 30, 60}, 2); got != 30*time.Second {
		t.Fatalf("second cooldown = %s", got)
	}
	if got := CircuitCooldown([]int{10, 30, 60}, 5); got != 60*time.Second {
		t.Fatalf("overflow cooldown = %s", got)
	}
}
