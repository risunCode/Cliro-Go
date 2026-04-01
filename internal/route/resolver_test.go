package route

import "testing"

func TestResolveModel_Codex(t *testing.T) {
	resolved, err := ResolveModel("gpt-5.3-codex", "", nil)
	if err != nil {
		t.Fatalf("resolve model: %v", err)
	}
	if resolved.Provider != ProviderCodex {
		t.Fatalf("expected codex provider, got %q", resolved.Provider)
	}
	if resolved.ResolvedModel != "gpt-5.3-codex" {
		t.Fatalf("expected unchanged model, got %q", resolved.ResolvedModel)
	}
	if resolved.ThinkingRequested {
		t.Fatalf("expected thinking to be false")
	}
}

func TestResolveModel_CodexThinkingSuffixIsPreservedSeparately(t *testing.T) {
	resolved, err := ResolveModel("gpt-5.3-codex-thinking", "", nil)
	if err != nil {
		t.Fatalf("resolve model: %v", err)
	}
	if resolved.Provider != ProviderCodex {
		t.Fatalf("expected codex provider, got %q", resolved.Provider)
	}
	if resolved.ResolvedModel != "gpt-5.3-codex" {
		t.Fatalf("expected stripped codex model, got %q", resolved.ResolvedModel)
	}
	if !resolved.ThinkingRequested {
		t.Fatalf("expected thinking to be true")
	}
}

func TestResolveModel_KiroBaseModel(t *testing.T) {
	resolved, err := ResolveModel("claude-sonnet-4.5", "", nil)
	if err != nil {
		t.Fatalf("resolve model: %v", err)
	}
	if resolved.Provider != ProviderKiro {
		t.Fatalf("expected kiro provider, got %q", resolved.Provider)
	}
	if resolved.ResolvedModel != "claude-sonnet-4.5" {
		t.Fatalf("expected normalized kiro model, got %q", resolved.ResolvedModel)
	}
	if resolved.ThinkingRequested {
		t.Fatalf("expected thinking to be false")
	}
}

func TestResolveModel_KiroThinkingAliasPreservesThinkingIntent(t *testing.T) {
	resolved, err := ResolveModel("claude-sonnet-4-5-thinking", "", nil)
	if err != nil {
		t.Fatalf("resolve model: %v", err)
	}
	if resolved.Provider != ProviderKiro {
		t.Fatalf("expected kiro provider, got %q", resolved.Provider)
	}
	if resolved.ResolvedModel != "claude-sonnet-4.5" {
		t.Fatalf("expected normalized kiro model, got %q", resolved.ResolvedModel)
	}
	if !resolved.ThinkingRequested {
		t.Fatalf("expected thinking to be true")
	}
	if resolved.RequestedModel != "claude-sonnet-4-5-thinking" {
		t.Fatalf("expected requested model to stay unchanged, got %q", resolved.RequestedModel)
	}
}

func TestResolveModel_RequestAliasStillWorksOnlyForConfiguredAliases(t *testing.T) {
	resolved, err := ResolveModel("sonnet-thinking", "", map[string]string{"sonnet": "claude-sonnet-4.5"})
	if err != nil {
		t.Fatalf("resolve model: %v", err)
	}
	if resolved.Provider != ProviderKiro {
		t.Fatalf("expected kiro provider, got %q", resolved.Provider)
	}
	if resolved.ResolvedModel != "claude-sonnet-4.5" {
		t.Fatalf("expected aliased kiro model, got %q", resolved.ResolvedModel)
	}
	if !resolved.ThinkingRequested {
		t.Fatalf("expected thinking to be true")
	}
}

func TestResolveModel_KiroNewExposedModels(t *testing.T) {
	testCases := []string{"claude-sonnet-4", "minimax-m2.5", "qwen3-coder-next"}
	for _, model := range testCases {
		resolved, err := ResolveModel(model, "", nil)
		if err != nil {
			t.Fatalf("resolve %q: %v", model, err)
		}
		if resolved.Provider != ProviderKiro {
			t.Fatalf("expected kiro provider for %q, got %q", model, resolved.Provider)
		}
		if resolved.ResolvedModel != model {
			t.Fatalf("expected unchanged model %q, got %q", model, resolved.ResolvedModel)
		}
	}
}

func TestResolveModel_KiroSonnet37IsNoLongerSupported(t *testing.T) {
	if _, err := ResolveModel("claude-3-7-sonnet", "", nil); err == nil {
		t.Fatalf("expected sonnet 3.7 to be unsupported")
	}
}

func TestResolveModel_KiroLegacyBuiltinAliasesAreUnsupported(t *testing.T) {
	testCases := []string{"default", "latest"}
	for _, model := range testCases {
		if _, err := ResolveModel(model, "", nil); err == nil {
			t.Fatalf("expected %q to be unsupported without built-in aliases", model)
		}
	}
}

func TestResolveModel_KiroPassthrough(t *testing.T) {
	resolved, err := ResolveModel("kiro-claude-secret-9-9", "", nil)
	if err != nil {
		t.Fatalf("resolve model: %v", err)
	}
	if resolved.Provider != ProviderKiro {
		t.Fatalf("expected kiro provider, got %q", resolved.Provider)
	}
	if resolved.ResolvedModel != "claude-secret-9.9" {
		t.Fatalf("expected normalized passthrough model, got %q", resolved.ResolvedModel)
	}
}

func TestResolveModel_UnknownModelFails(t *testing.T) {
	_, err := ResolveModel("mystery-model", "", nil)
	if err == nil {
		t.Fatalf("expected unsupported model error")
	}
}

func TestResolveModel_EmptyModelFails(t *testing.T) {
	_, err := ResolveModel("   ", "", nil)
	if err == nil {
		t.Fatalf("expected empty model error")
	}
}

func TestCatalogModels_ExposeRequestedKiroModelsWithoutThinkingAliases(t *testing.T) {
	models := CatalogModels()
	seen := map[string]bool{}
	for _, model := range models {
		seen[model.ID] = true
	}
	if !seen["gpt-5.3-codex"] {
		t.Fatalf("expected codex base model, got %+v", models)
	}
	if seen["gpt-5.3-codex-thinking"] {
		t.Fatalf("did not expect thinking alias for codex model, got %+v", models)
	}
	if !seen["claude-sonnet-4.5"] {
		t.Fatalf("expected kiro base model, got %+v", models)
	}
	if !seen["claude-sonnet-4"] || !seen["minimax-m2.5"] || !seen["qwen3-coder-next"] {
		t.Fatalf("expected new kiro models, got %+v", models)
	}
	if seen["claude-sonnet-4.5-thinking"] {
		t.Fatalf("did not expect kiro thinking alias, got %+v", models)
	}
	if seen["claude-3.7-sonnet"] {
		t.Fatalf("did not expect retired sonnet 3.7 model in catalog, got %+v", models)
	}
}
