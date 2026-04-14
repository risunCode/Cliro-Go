package kiro

import (
	"testing"

	provider "cliro/internal/provider"
)

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

func TestCollapseBlankLines_PreservesSpacingBetweenLines(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "single line",
			input: "Hello world",
			want:  "Hello world",
		},
		{
			name:  "two lines",
			input: "Line one\nLine two",
			want:  "Line one\nLine two",
		},
		{
			name:  "multiple blank lines collapsed",
			input: "Paragraph one\n\n\n\nParagraph two",
			want:  "Paragraph one\n\nParagraph two",
		},
		{
			name:  "preserves single blank line",
			input: "Paragraph one\n\nParagraph two",
			want:  "Paragraph one\n\nParagraph two",
		},
		{
			name:  "removes leading blank lines",
			input: "\n\nContent",
			want:  "Content",
		},
		{
			name:  "removes trailing blank lines",
			input: "Content\n\n",
			want:  "Content",
		},
		{
			name:  "response with multiple sentences",
			input: "Baik, saya sudah membaca dan memahami workspace CLIRO saat ini.\n\nIni ringkasan pemahaman saya:",
			want:  "Baik, saya sudah membaca dan memahami workspace CLIRO saat ini.\n\nIni ringkasan pemahaman saya:",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := collapseBlankLines(tt.input)
			if got != tt.want {
				t.Errorf("collapseBlankLines() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestMessageTextContent_PreservesSplitTextSpacing(t *testing.T) {
	content := []any{
		map[string]any{"type": "text", "text": "Baik, saya akan cek struktur"},
		map[string]any{"type": "text", "text": " modal dan layout"},
		map[string]any{"type": "text", "text": " app shell."},
	}

	got := messageTextContent(content)
	if got != "Baik, saya akan cek struktur modal dan layout app shell." {
		t.Fatalf("messageTextContent = %q", got)
	}
}

func TestNormalizeRequest_PreservesAssistantSpacingAcrossAdjacentBlocks(t *testing.T) {
	messages, _, err := normalizeRequest(provider.ChatRequest{
		Messages: []provider.Message{{
			Role: "assistant",
			Content: []any{
				map[string]any{"type": "text", "text": "Baik, saya akan cek struktur"},
				map[string]any{"type": "text", "text": " modal dan layout"},
				map[string]any{"type": "text", "text": " app shell."},
			},
		}},
	})
	if err != nil {
		t.Fatalf("normalizeRequest: %v", err)
	}
	if len(messages) < 2 {
		t.Fatalf("normalized messages = %#v", messages)
	}
	if messages[1].Content != "Baik, saya akan cek struktur modal dan layout app shell." {
		t.Fatalf("assistant content = %q", messages[1].Content)
	}
}
