package kiro

import "testing"

func TestSanitizePromptText_StripsEnvironmentDetailsBlock(t *testing.T) {
	input := "before\n<environment_details>\nCurrent time: 2026-03-30T20:43:49+07:00\n</environment_details>\nafter"
	got := sanitizePromptText(input)
	if got != "before\n\nafter" {
		t.Fatalf("sanitizePromptText = %q", got)
	}
}

func TestSanitizeModelOutputText_StripsStandaloneEnvironmentDetails(t *testing.T) {
	input := "<environment_details>\nCurrent time: 2026-03-30T20:43:49+07:00\n</environment_details>"
	got := sanitizeModelOutputText(input)
	if got != "" {
		t.Fatalf("sanitizeModelOutputText = %q", got)
	}
}
