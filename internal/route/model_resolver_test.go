package route

import "testing"

func TestResolveModel_Codex(t *testing.T) {
	resolved, err := ResolveModel("gpt-5.3-codex", "")
	if err != nil {
		t.Fatalf("resolve model: %v", err)
	}
	if resolved.Provider != ProviderCodex {
		t.Fatalf("expected codex provider, got %q", resolved.Provider)
	}
	if resolved.ResolvedModel != "gpt-5.3-codex" {
		t.Fatalf("expected unchanged model, got %q", resolved.ResolvedModel)
	}
	if resolved.ThinkingEnabled {
		t.Fatalf("did not expect thinking suffix for codex model")
	}
}

func TestResolveModel_KiroWithThinkingSuffix(t *testing.T) {
	resolved, err := ResolveModel("claude-haiku-4.5-thinking", "")
	if err != nil {
		t.Fatalf("resolve model: %v", err)
	}
	if resolved.Provider != ProviderKiro {
		t.Fatalf("expected kiro provider, got %q", resolved.Provider)
	}
	if resolved.ResolvedModel != "claude-haiku-4.5" {
		t.Fatalf("expected stripped thinking suffix, got %q", resolved.ResolvedModel)
	}
	if !resolved.ThinkingEnabled {
		t.Fatalf("expected thinking suffix to be enabled")
	}
}

func TestResolveModel_CodexThinkingSuffixIsIgnored(t *testing.T) {
	resolved, err := ResolveModel("gpt-5.3-codex-thinking", "")
	if err != nil {
		t.Fatalf("resolve model: %v", err)
	}
	if resolved.Provider != ProviderCodex {
		t.Fatalf("expected codex provider, got %q", resolved.Provider)
	}
	if resolved.ResolvedModel != "gpt-5.3-codex" {
		t.Fatalf("expected stripped codex model, got %q", resolved.ResolvedModel)
	}
	if resolved.ThinkingEnabled {
		t.Fatalf("did not expect thinking suffix to remain enabled for codex")
	}
}

func TestResolveModel_UnknownModelFails(t *testing.T) {
	_, err := ResolveModel("mystery-model", "")
	if err == nil {
		t.Fatalf("expected unsupported model error")
	}
}

func TestResolveModel_EmptyModelFails(t *testing.T) {
	_, err := ResolveModel("   ", "")
	if err == nil {
		t.Fatalf("expected empty model error")
	}
}

func TestCatalogModels_IncludesThinkingAliases(t *testing.T) {
	models := CatalogModels(DefaultThinkingSuffix)
	seen := map[string]bool{}
	for _, model := range models {
		seen[model.ID] = true
	}
	if !seen["claude-sonnet-4.5"] || !seen["claude-sonnet-4.5-thinking"] {
		t.Fatalf("expected thinking alias for kiro model, got %+v", models)
	}
	if !seen["gpt-5.3-codex"] {
		t.Fatalf("expected codex base model, got %+v", models)
	}
	if !seen["qwen3-coder-next"] || !seen["qwen3-coder-next-thinking"] {
		t.Fatalf("expected thinking alias for qwen model, got %+v", models)
	}
	if !seen["minimax-m2.5"] || !seen["minimax-m2.5-thinking"] {
		t.Fatalf("expected thinking alias for minimax model, got %+v", models)
	}
	if seen["gpt-5.3-codex-thinking"] {
		t.Fatalf("did not expect thinking alias for codex model, got %+v", models)
	}
}

func TestResolveModel_NewKiroModelsSupportThinking(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{input: "qwen3-coder-next-thinking", want: "qwen3-coder-next"},
		{input: "minimax-m2.5-thinking", want: "minimax-m2.5"},
	}

	for _, tc := range cases {
		resolved, err := ResolveModel(tc.input, "")
		if err != nil {
			t.Fatalf("resolve %s: %v", tc.input, err)
		}
		if resolved.Provider != ProviderKiro {
			t.Fatalf("expected kiro provider for %s, got %q", tc.input, resolved.Provider)
		}
		if resolved.ResolvedModel != tc.want {
			t.Fatalf("expected resolved model %q, got %q", tc.want, resolved.ResolvedModel)
		}
		if !resolved.ThinkingEnabled {
			t.Fatalf("expected thinking enabled for %s", tc.input)
		}
	}
}

func TestDefaultStreamingEnabled_FollowsKiroThinkingSuffixOnly(t *testing.T) {
	if DefaultStreamingEnabled("claude-sonnet-4.5", "anthropic_messages") {
		t.Fatalf("expected non-thinking kiro model to default to non-stream")
	}
	if !DefaultStreamingEnabled("claude-sonnet-4.5-thinking", "anthropic_messages") {
		t.Fatalf("expected thinking kiro model to default to stream")
	}
	if DefaultStreamingEnabled("gpt-5.3-codex-thinking", "openai_chat") {
		t.Fatalf("did not expect codex thinking suffix to change default stream behavior")
	}
	if DefaultStreamingEnabled("gpt-5.3-codex", "openai_chat") {
		t.Fatalf("expected non-thinking codex model to default to non-stream")
	}
}
