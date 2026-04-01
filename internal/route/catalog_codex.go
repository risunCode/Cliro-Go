package route

import "strings"

var codexModelPrefixes = []string{
	"gpt-",
	"o1",
	"o3",
	"o4",
}

var codexModelCatalog = []ModelDefinition{
	{ID: "gpt-5.1-codex-max", OwnedBy: string(ProviderCodex), SupportsThinking: true},
	{ID: "gpt-5.1-codex-mini", OwnedBy: string(ProviderCodex), SupportsThinking: true},
	{ID: "gpt-5.2", OwnedBy: string(ProviderCodex), SupportsThinking: true},
	{ID: "gpt-5.4", OwnedBy: string(ProviderCodex), SupportsThinking: true},
	{ID: "gpt-5.2-codex", OwnedBy: string(ProviderCodex), SupportsThinking: true},
	{ID: "gpt-5.3-codex", OwnedBy: string(ProviderCodex), SupportsThinking: true},
	{ID: "gpt-5.1-codex", OwnedBy: string(ProviderCodex), SupportsThinking: true},
}

var codexModelLookup = makeModelLookup(codexModelCatalog)

func resolveCodexModel(model string) (string, bool) {
	trimmed := strings.TrimSpace(model)
	if trimmed == "" {
		return "", false
	}

	if resolved, ok := codexModelLookup[strings.ToLower(trimmed)]; ok {
		return resolved, true
	}

	normalized := strings.ToLower(trimmed)
	for _, prefix := range codexModelPrefixes {
		if strings.HasPrefix(normalized, prefix) {
			return trimmed, true
		}
	}

	return "", false
}

func makeModelLookup(models []ModelDefinition) map[string]string {
	lookup := make(map[string]string, len(models))
	for _, model := range models {
		lookup[strings.ToLower(strings.TrimSpace(model.ID))] = model.ID
	}
	return lookup
}
