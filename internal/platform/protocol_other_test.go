//go:build !windows

package platform

import "testing"

func TestEnsureProtocolRegistered_NoOpOnNonWindows(t *testing.T) {
	registered, err := EnsureProtocolRegistered()
	if err != nil {
		t.Fatalf("EnsureProtocolRegistered: %v", err)
	}
	if registered {
		t.Fatalf("registered = %t", registered)
	}
}
