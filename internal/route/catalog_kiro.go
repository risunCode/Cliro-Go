package route

import (
	"regexp"
	"strings"
)

var (
	kiroModelCatalog = []ModelDefinition{
		{ID: "auto", OwnedBy: string(ProviderKiro)},
		{ID: "claude-haiku-4.5", OwnedBy: string(ProviderKiro), SupportsThinking: true},
		{ID: "claude-sonnet-4", OwnedBy: string(ProviderKiro), SupportsThinking: true},
		{ID: "claude-sonnet-4.5", OwnedBy: string(ProviderKiro), SupportsThinking: true},
		{ID: "claude-opus-4.5", OwnedBy: string(ProviderKiro), SupportsThinking: true},
		{ID: "minimax-m2.5", OwnedBy: string(ProviderKiro), SupportsThinking: true},
		{ID: "qwen3-coder-next", OwnedBy: string(ProviderKiro), SupportsThinking: true},
	}

	kiroVisibleLookup = makeModelLookup(kiroModelCatalog)

	kiroRetiredModels = map[string]struct{}{
		"claude-3.7-sonnet": {},
	}

	kiroFamilyVersionPattern = regexp.MustCompile(`^claude-(haiku|sonnet|opus)-(\d+)-(\d+)$`)
	kiroLegacyVersionPattern = regexp.MustCompile(`^claude-(\d+)-(\d+)-(haiku|sonnet|opus)$`)
	kiroDatedPattern         = regexp.MustCompile(`^(.*?)-(\d{8})$`)
	kiroGenericVersionRegex  = regexp.MustCompile(`^(.*?)-(\d+)-(\d+)$`)
	kiroInternalIDPattern    = regexp.MustCompile(`^[A-Z0-9_]+$`)
)

func resolveKiroModel(model string) (string, bool) {
	trimmed := strings.TrimSpace(model)
	if trimmed == "" {
		return "", false
	}

	if looksLikeKiroInternalModel(trimmed) {
		return trimmed, true
	}

	normalized := normalizeKiroModel(trimmed)
	if normalized == "" {
		return "", false
	}
	if _, retired := kiroRetiredModels[normalized]; retired {
		return "", false
	}

	if visibleModel, ok := kiroVisibleLookup[normalized]; ok {
		return visibleModel, true
	}
	if looksLikeKiroPassthrough(trimmed) {
		return normalized, true
	}

	return "", false
}

func normalizeKiroModel(model string) string {
	normalized := strings.ToLower(strings.TrimSpace(model))
	if normalized == "" {
		return ""
	}

	normalized = strings.TrimPrefix(normalized, "kiro-")
	normalized = strings.TrimPrefix(normalized, "amazonq-")
	normalized = strings.TrimSuffix(normalized, "-agentic")

	if matches := kiroDatedPattern.FindStringSubmatch(normalized); len(matches) == 3 {
		normalized = matches[1]
	}

	if matches := kiroFamilyVersionPattern.FindStringSubmatch(normalized); len(matches) == 4 {
		return "claude-" + matches[1] + "-" + matches[2] + "." + matches[3]
	}
	if matches := kiroLegacyVersionPattern.FindStringSubmatch(normalized); len(matches) == 4 {
		return "claude-" + matches[1] + "." + matches[2] + "-" + matches[3]
	}
	if matches := kiroGenericVersionRegex.FindStringSubmatch(normalized); len(matches) == 4 {
		return matches[1] + "-" + matches[2] + "." + matches[3]
	}

	return normalized
}

func looksLikeKiroPassthrough(model string) bool {
	normalized := strings.ToLower(strings.TrimSpace(model))
	if normalized == "" {
		return false
	}
	return normalized == "auto" ||
		strings.HasPrefix(normalized, "claude-") ||
		strings.HasPrefix(normalized, "kiro-") ||
		strings.HasPrefix(normalized, "amazonq-")
}

func looksLikeKiroInternalModel(model string) bool {
	trimmed := strings.TrimSpace(model)
	if trimmed == "" {
		return false
	}
	return kiroInternalIDPattern.MatchString(trimmed) && strings.Contains(trimmed, "CLAUDE")
}
