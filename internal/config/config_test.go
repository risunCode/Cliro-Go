package config

import "testing"

func TestNewManager_DefaultsProxyRoutingSettings(t *testing.T) {
	manager, err := NewManager(t.TempDir())
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	if manager.SchedulingMode() != SchedulingModeBalance {
		t.Fatalf("scheduling mode = %q", manager.SchedulingMode())
	}
	cloudflared := manager.Cloudflared()
	if cloudflared.Mode != CloudflaredModeQuick || !cloudflared.UseHTTP2 || cloudflared.Enabled {
		t.Fatalf("unexpected cloudflared defaults: %+v", cloudflared)
	}
	thinking := manager.Snapshot().Thinking
	if thinking.Suffix != defaultThinkingSuffix || thinking.Mode != ThinkingModeAuto || !thinking.RequireAnthropicSignature || thinking.ForceForAnthropic || thinking.MaxForcedThinkingTokens != defaultMaxForcedThinkingTokens {
		t.Fatalf("unexpected thinking defaults: %+v", thinking)
	}
	if len(thinking.FallbackTags) != 2 || thinking.FallbackTags[0] != "<thinking>" || thinking.FallbackTags[1] != "<think>" {
		t.Fatalf("unexpected fallback tag defaults: %+v", thinking.FallbackTags)
	}
}

func TestSetCloudflaredConfig_PersistsNormalizedSettings(t *testing.T) {
	manager, err := NewManager(t.TempDir())
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	if err := manager.SetCloudflaredConfig(CloudflaredModeAuth, "  secret-token  ", false); err != nil {
		t.Fatalf("set cloudflared config: %v", err)
	}
	if err := manager.SetCloudflaredEnabled(true); err != nil {
		t.Fatalf("set cloudflared enabled: %v", err)
	}
	cloudflared := manager.Cloudflared()
	if cloudflared.Mode != CloudflaredModeAuth || cloudflared.Token != "secret-token" || cloudflared.UseHTTP2 || !cloudflared.Enabled {
		t.Fatalf("unexpected cloudflared config: %+v", cloudflared)
	}
}

func TestBlockedAccountReason_DetectsTokenInvalidated(t *testing.T) {
	reason, blocked := BlockedAccountReason("access token invalidated by provider")
	if !blocked {
		t.Fatalf("expected blocked account classification")
	}
	if reason == "" {
		t.Fatalf("expected non-empty blocked reason")
	}
}

func TestBlockedAccountReason_DoesNotTreatInvalidBearerTokenAsDurableBan(t *testing.T) {
	reason, blocked := BlockedAccountReason(`{"message":"The bearer token included in the request is invalid.","reason":null}`)
	if blocked {
		t.Fatalf("expected invalid bearer token to stay refreshable, got reason %q", reason)
	}
	if reason != "" {
		t.Fatalf("reason = %q", reason)
	}
}

func TestRefreshableAuthReason_DetectsInvalidBearerTokenJSON(t *testing.T) {
	reason, refreshable := RefreshableAuthReason(`{"message":"The bearer token included in the request is invalid.","reason":null}`)
	if !refreshable {
		t.Fatalf("expected refreshable auth classification")
	}
	if reason != "The bearer token included in the request is invalid." {
		t.Fatalf("reason = %q", reason)
	}
}
