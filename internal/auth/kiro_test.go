package auth

import (
	"strings"
	"testing"
)

func TestNormalizeKiroSocialProvider(t *testing.T) {
	provider, err := normalizeKiroSocialProvider("google")
	if err != nil {
		t.Fatalf("normalize google: %v", err)
	}
	if provider != kiroSocialProviderGoogle {
		t.Fatalf("provider = %q", provider)
	}

	provider, err = normalizeKiroSocialProvider("github")
	if err != nil {
		t.Fatalf("normalize github: %v", err)
	}
	if provider != kiroSocialProviderGitHub {
		t.Fatalf("provider = %q", provider)
	}
}

func TestBuildKiroSocialLoginURL(t *testing.T) {
	url := buildKiroSocialLoginURL(kiroSocialProviderGoogle, "http://localhost:9876/oauth/callback", "challenge", "state123")
	if !strings.Contains(url, "/login?idp=Google") {
		t.Fatalf("unexpected url: %s", url)
	}
	if !strings.Contains(url, "redirect_uri=http%3A%2F%2Flocalhost%3A9876%2Foauth%2Fcallback") {
		t.Fatalf("missing redirect_uri: %s", url)
	}
	if !strings.Contains(url, "code_challenge=challenge") || !strings.Contains(url, "state=state123") {
		t.Fatalf("missing challenge/state: %s", url)
	}
}
