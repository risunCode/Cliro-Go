package anthropic

import (
	"testing"

	contract "cliro-go/internal/contract"
)

func TestIRToMessages_ThinkingFirstStableSignatureAndToolRemap(t *testing.T) {
	response := contract.Response{
		Model:      "claude-sonnet-4.5",
		Thinking:   "plan",
		Text:       "done",
		ToolCalls:  []contract.ToolCall{{ID: "toolu_1", Name: "Glob", Arguments: `{"query":"*.go","paths":["internal"]}`}},
		StopReason: "tool_calls",
		Usage:      contract.Usage{InputTokens: 4, OutputTokens: 6},
	}

	first := IRToMessages(response)
	second := IRToMessages(response)

	content, ok := first.Content.([]map[string]any)
	if !ok {
		t.Fatalf("content type = %T", first.Content)
	}
	if len(content) != 3 {
		t.Fatalf("content count = %d", len(content))
	}
	if content[0]["type"] != "thinking" || content[1]["type"] != "text" || content[2]["type"] != "tool_use" {
		t.Fatalf("unexpected block order: %#v", content)
	}
	if content[0]["signature"] != StableThinkingSignature("plan") {
		t.Fatalf("thinking signature = %#v", content[0]["signature"])
	}
	secondContent := second.Content.([]map[string]any)
	if content[0]["signature"] != secondContent[0]["signature"] {
		t.Fatalf("signature not stable: %#v vs %#v", content[0]["signature"], secondContent[0]["signature"])
	}

	input, ok := content[2]["input"].(map[string]any)
	if !ok {
		t.Fatalf("tool input = %#v", content[2]["input"])
	}
	if input["pattern"] != "*.go" || input["path"] != "internal" {
		t.Fatalf("remapped input = %#v", input)
	}
	if _, exists := input["query"]; exists {
		t.Fatalf("unexpected query key in %#v", input)
	}
	if first.StopReason != "tool_use" {
		t.Fatalf("stop reason = %q", first.StopReason)
	}
}

func TestIRToMessages_PrefersNativeThinkingSignature(t *testing.T) {
	response := contract.Response{
		Model:             "claude-sonnet-4.5",
		Thinking:          "plan",
		ThinkingSignature: "sig_native",
		Text:              "done",
	}

	encoded := IRToMessages(response)
	content, ok := encoded.Content.([]map[string]any)
	if !ok {
		t.Fatalf("content type = %T", encoded.Content)
	}
	if len(content) != 2 {
		t.Fatalf("content count = %d", len(content))
	}
	if content[0]["type"] != "thinking" {
		t.Fatalf("first block type = %#v", content[0]["type"])
	}
	if content[0]["signature"] != "sig_native" {
		t.Fatalf("signature = %#v", content[0]["signature"])
	}
}

func TestIRToMessages_OmitsEmptyThinkingTextAndToolBlocks(t *testing.T) {
	response := contract.Response{
		Model:    "claude-sonnet-4.5",
		Thinking: " \n\t ",
		Text:     "  ",
		ToolCalls: []contract.ToolCall{
			{ID: "toolu_1", Name: "", Arguments: `{"query":"*.go"}`},
			{ID: "toolu_2", Name: "   ", Arguments: `{"query":"*.md"}`},
		},
	}

	encoded := IRToMessages(response)
	content, ok := encoded.Content.([]map[string]any)
	if !ok {
		t.Fatalf("content type = %T", encoded.Content)
	}
	if len(content) != 0 {
		t.Fatalf("content = %#v", content)
	}
}

func TestResolveThinkingSignature_FallsBackDeterministically(t *testing.T) {
	const thinking = "  plan the next steps  "

	first := resolveThinkingSignature(thinking, "")
	second := resolveThinkingSignature(thinking, "   ")
	stable := StableThinkingSignature(thinking)

	if first == "" {
		t.Fatal("expected fallback signature")
	}
	if first != second {
		t.Fatalf("fallback signature mismatch: %q vs %q", first, second)
	}
	if first != stable {
		t.Fatalf("fallback signature = %q, want %q", first, stable)
	}
	other := resolveThinkingSignature("different", "")
	if first == other {
		t.Fatalf("fallback signatures should differ: %q vs %q", first, other)
	}
}
