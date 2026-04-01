package kiro

import (
	"strings"
	"testing"
)

func TestNormalizeKiroSocialProvider(t *testing.T) {
	provider, err := NormalizeSocialProvider("google")
	if err != nil {
		t.Fatalf("normalize google: %v", err)
	}
	if provider != SocialProviderGoogle {
		t.Fatalf("provider = %q", provider)
	}

	provider, err = NormalizeSocialProvider("github")
	if err != nil {
		t.Fatalf("normalize github: %v", err)
	}
	if provider != SocialProviderGitHub {
		t.Fatalf("provider = %q", provider)
	}
}

func TestBuildKiroSocialLoginURL(t *testing.T) {
	url := BuildSocialLoginURL(SocialProviderGoogle, "challenge", "state123")
	if !strings.Contains(url, "/login?idp=Google") {
		t.Fatalf("unexpected url: %s", url)
	}
	if !strings.Contains(url, "redirect_uri=kiro%3A%2F%2Fkiro.kiroAgent%2Fauthenticate-success") {
		t.Fatalf("missing custom protocol redirect_uri: %s", url)
	}
	if !strings.Contains(url, "code_challenge=challenge") || !strings.Contains(url, "state=state123") {
		t.Fatalf("missing challenge/state: %s", url)
	}
}
