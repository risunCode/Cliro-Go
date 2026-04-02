package config

import (
	"testing"
)

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

func TestBlockedAccountReason_DetectsDeactivatedCode(t *testing.T) {
	reason, blocked := BlockedAccountReason(`quota request failed (401): {"error":{"message":"Your OpenAI account has been deactivated.","code":"account_deactivated"}}`)
	if !blocked {
		t.Fatalf("expected blocked account classification")
	}
	if reason != "Your OpenAI account has been deactivated." {
		t.Fatalf("reason = %q", reason)
	}
}

func TestBlockedAccountReason_DetectsCompactCodePrefix(t *testing.T) {
	reason, blocked := BlockedAccountReason("quota request failed (401): account_deactivated: Your OpenAI account has been deactivated.")
	if !blocked {
		t.Fatalf("expected blocked account classification for compact code prefix")
	}
	if reason != "Your OpenAI account has been deactivated." {
		t.Fatalf("reason = %q", reason)
	}
}

func TestBlockedAccountReason_DoesNotTreatInvalidBearerTokenAsDurableBan(t *testing.T) {
	reason, blocked := BlockedAccountReason(`refresh token failed (401): {"error":{"message":"Your refresh token has already been used to generate a new access token.","code":"refresh_token_reused"}}`)
	if blocked {
		t.Fatalf("expected refresh_token_reused to stay refreshable, got reason %q", reason)
	}
	if reason != "" {
		t.Fatalf("reason = %q", reason)
	}
}

func TestRefreshableAuthReason_DetectsRefreshTokenReuseByCode(t *testing.T) {
	reason, refreshable := RefreshableAuthReason(`refresh token failed (401): {"error":{"message":"Your refresh token has already been used to generate a new access token.","code":"refresh_token_reused"}}`)
	if !refreshable {
		t.Fatalf("expected refreshable auth classification")
	}
	if reason != "Your refresh token has already been used to generate a new access token." {
		t.Fatalf("reason = %q", reason)
	}
}

func TestRefreshableAuthReason_DetectsCompactRefreshCodePrefix(t *testing.T) {
	reason, refreshable := RefreshableAuthReason("refresh token failed (401): refresh_token_reused: Your refresh token has already been used to generate a new access token.")
	if !refreshable {
		t.Fatalf("expected refreshable auth classification for compact code prefix")
	}
	if reason != "Your refresh token has already been used to generate a new access token." {
		t.Fatalf("reason = %q", reason)
	}
}
