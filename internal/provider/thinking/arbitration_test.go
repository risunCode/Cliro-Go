package thinking

import (
	contract "cliro-go/internal/contract"
	"testing"
)

func TestSelect_PrefersNativeOverParsedAndForced(t *testing.T) {
	selection := Select(Inputs{
		Request:      contract.ThinkingConfig{Requested: true, Mode: contract.ThinkingModeForce},
		ForceAllowed: true,
		Native:       Candidate{Thinking: "native plan", Signature: "sig-native"},
		Parsed:       Candidate{Thinking: "parsed plan", Signature: "sig-parsed"},
		Forced:       Candidate{Thinking: "forced plan", Signature: "sig-forced"},
	})

	if selection.Source != SourceNative {
		t.Fatalf("source = %q", selection.Source)
	}
	if selection.Thinking != "native plan" {
		t.Fatalf("thinking = %q", selection.Thinking)
	}
	if selection.Signature != "sig-native" {
		t.Fatalf("signature = %q", selection.Signature)
	}
}

func TestSelect_PrefersParsedOverForcedWhenNativeAbsent(t *testing.T) {
	selection := Select(Inputs{
		Request:      contract.ThinkingConfig{Requested: true, Mode: contract.ThinkingModeForce},
		ForceAllowed: true,
		Parsed:       Candidate{Thinking: "parsed plan", Signature: "sig-parsed"},
		Forced:       Candidate{Thinking: "forced plan", Signature: "sig-forced"},
	})

	if selection.Source != SourceParsed {
		t.Fatalf("source = %q", selection.Source)
	}
	if selection.Thinking != "parsed plan" {
		t.Fatalf("thinking = %q", selection.Thinking)
	}
	if selection.Signature != "sig-parsed" {
		t.Fatalf("signature = %q", selection.Signature)
	}
}

func TestSelect_UsesForcedOnlyWhenRequestedAndAllowed(t *testing.T) {
	forced := Candidate{Thinking: "forced plan", Signature: "sig-forced"}

	t.Run("requested and allowed", func(t *testing.T) {
		selection := Select(Inputs{
			Request:      contract.ThinkingConfig{Requested: true, Mode: contract.ThinkingModeForce},
			ForceAllowed: true,
			Forced:       forced,
		})

		if selection.Source != SourceForced {
			t.Fatalf("source = %q", selection.Source)
		}
	})

	t.Run("not requested", func(t *testing.T) {
		selection := Select(Inputs{ForceAllowed: true, Forced: forced})
		if selection.Source != SourceNone {
			t.Fatalf("source = %q", selection.Source)
		}
	})

	t.Run("not allowed", func(t *testing.T) {
		selection := Select(Inputs{
			Request: contract.ThinkingConfig{Requested: true, Mode: contract.ThinkingModeForce},
			Forced:  forced,
		})

		if selection.Source != SourceNone {
			t.Fatalf("source = %q", selection.Source)
		}
	})

	t.Run("blank forced thinking", func(t *testing.T) {
		selection := Select(Inputs{
			Request:      contract.ThinkingConfig{Requested: true, Mode: contract.ThinkingModeForce},
			ForceAllowed: true,
			Forced:       Candidate{Thinking: " \n\t "},
		})
		if selection.Source != SourceNone {
			t.Fatalf("source = %q", selection.Source)
		}
	})
}

func TestForceEligible(t *testing.T) {
	tests := []struct {
		name            string
		request         contract.ThinkingConfig
		forceConfigured bool
		want            bool
	}{
		{name: "disabled when not requested", request: contract.ThinkingConfig{Mode: contract.ThinkingModeForce}, forceConfigured: true, want: false},
		{name: "disabled when force not configured", request: contract.ThinkingConfig{Requested: true, Mode: contract.ThinkingModeForce}, want: false},
		{name: "enabled for auto", request: contract.ThinkingConfig{Requested: true, Mode: contract.ThinkingModeAuto}, forceConfigured: true, want: true},
		{name: "enabled for force", request: contract.ThinkingConfig{Requested: true, Mode: contract.ThinkingModeForce}, forceConfigured: true, want: true},
		{name: "disabled for off", request: contract.ThinkingConfig{Requested: true, Mode: contract.ThinkingModeOff}, forceConfigured: true, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ForceEligible(tt.request, tt.forceConfigured); got != tt.want {
				t.Fatalf("ForceEligible() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestArbiter_AllowsOnlyOneSource(t *testing.T) {
	var arbiter Arbiter

	if !arbiter.Allow(SourceNative) {
		t.Fatal("expected initial native source to be accepted")
	}
	if !arbiter.Allow(SourceNative) {
		t.Fatal("expected duplicate native source to stay accepted")
	}
	if arbiter.Allow(SourceParsed) {
		t.Fatal("expected parsed source to be rejected after native selection")
	}
	if arbiter.Allow(SourceForced) {
		t.Fatal("expected forced source to be rejected after native selection")
	}
	if arbiter.Selected() != SourceNative {
		t.Fatalf("selected = %q", arbiter.Selected())
	}
}

func TestArbiter_RejectsNoneSource(t *testing.T) {
	var arbiter Arbiter

	if arbiter.Allow(SourceNone) {
		t.Fatal("expected none source to be rejected")
	}
	if arbiter.Selected() != SourceNone {
		t.Fatalf("selected = %q", arbiter.Selected())
	}
}
