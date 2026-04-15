package models

import (
	"fmt"
	"regexp"
	"strings"
)

const (
	ProviderCodex Provider = "codex"
	ProviderKiro  Provider = "kiro"

	DefaultThinkingSuffix = "-thinking"
)

type ModelResolution struct {
	Provider          Provider
	RequestedModel    string
	ResolvedModel     string
	ThinkingRequested bool
	ThinkingEffort    string
}

type ModelDefinition struct {
	ID               string
	OwnedBy          string
	SupportsThinking bool
	Hidden           bool
}

var codexModelPrefixes = []string{
	"gpt-",
	"o1",
	"o3",
	"o4",
}

var codexModelCatalog = []ModelDefinition{
	{ID: "gpt-5", OwnedBy: string(ProviderCodex), SupportsThinking: true},
	{ID: "gpt-5-codex", OwnedBy: string(ProviderCodex), SupportsThinking: true},
	{ID: "gpt-5-codex-mini", OwnedBy: string(ProviderCodex), SupportsThinking: true},
	{ID: "gpt-5.1", OwnedBy: string(ProviderCodex), SupportsThinking: true},
	{ID: "gpt-5.1-codex", OwnedBy: string(ProviderCodex), SupportsThinking: true},
	{ID: "gpt-5.1-codex-mini", OwnedBy: string(ProviderCodex), SupportsThinking: true},
	{ID: "gpt-5.1-codex-max", OwnedBy: string(ProviderCodex), SupportsThinking: true},
	{ID: "gpt-5.2", OwnedBy: string(ProviderCodex), SupportsThinking: true},
	{ID: "gpt-5.2-codex", OwnedBy: string(ProviderCodex), SupportsThinking: true},
	{ID: "gpt-5.3-codex", OwnedBy: string(ProviderCodex), SupportsThinking: true},
	{ID: "gpt-5.4", OwnedBy: string(ProviderCodex), SupportsThinking: true},
	{ID: "gpt-5.4-mini", OwnedBy: string(ProviderCodex), SupportsThinking: true},
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

func ResolveModel(model string, thinkingSuffix string, aliases map[string]string) (ModelResolution, error) {
	requested := strings.TrimSpace(model)
	if requested == "" {
		return ModelResolution{}, fmt.Errorf("model is required")
	}

	resolvedBase, thinkingRequested := splitThinkingSuffix(requested, thinkingSuffix)
	resolvedBase, effortSuffix := splitEffortSuffix(resolvedBase)

	// Check alias first
	if aliasTarget, ok := aliases[resolvedBase]; ok && strings.TrimSpace(aliasTarget) != "" {
		resolvedBase = strings.TrimSpace(aliasTarget)
	}

	if resolvedModel, ok := resolveCodexModel(resolvedBase); ok {
		return ModelResolution{
			Provider:          ProviderCodex,
			RequestedModel:    requested,
			ResolvedModel:     resolvedModel,
			ThinkingRequested: thinkingRequested || effortSuffix != "",
			ThinkingEffort:    effortSuffix,
		}, nil
	}
	if resolvedModel, ok := resolveKiroModel(resolvedBase); ok {
		return ModelResolution{
			Provider:          ProviderKiro,
			RequestedModel:    requested,
			ResolvedModel:     resolvedModel,
			ThinkingRequested: true,
			ThinkingEffort:    effortSuffix,
		}, nil
	}

	return ModelResolution{}, fmt.Errorf("unsupported model: %s", requested)
}

func splitThinkingSuffix(model string, suffix string) (string, bool) {
	trimmed := strings.TrimSpace(model)
	if trimmed == "" {
		return "", false
	}

	resolvedSuffix := strings.TrimSpace(suffix)
	if resolvedSuffix == "" {
		resolvedSuffix = DefaultThinkingSuffix
	}

	lowerModel := strings.ToLower(trimmed)
	lowerSuffix := strings.ToLower(resolvedSuffix)
	if strings.HasSuffix(lowerModel, lowerSuffix) {
		base := strings.TrimSpace(trimmed[:len(trimmed)-len(lowerSuffix)])
		if base != "" {
			return base, true
		}
	}

	return trimmed, false
}

func splitEffortSuffix(model string) (string, string) {
	trimmed := strings.TrimSpace(model)
	if trimmed == "" {
		return "", ""
	}

	lowerModel := strings.ToLower(trimmed)
	efforts := []string{"-xhigh", "-high", "-medium", "-low", "-minimal", "-auto", "-max", "-none"}
	for _, effort := range efforts {
		if strings.HasSuffix(lowerModel, effort) {
			base := strings.TrimSpace(trimmed[:len(trimmed)-len(effort)])
			if base != "" {
				return base, strings.TrimPrefix(effort, "-")
			}
		}
	}

	return trimmed, ""
}
func CatalogModels() []ModelDefinition {
	models := make([]ModelDefinition, 0, len(codexModelCatalog)+len(kiroModelCatalog))
	models = append(models, catalogModelsForProvider(codexModelCatalog)...)
	models = append(models, catalogModelsForProvider(kiroModelCatalog)...)
	return models
}

func catalogModelsForProvider(catalog []ModelDefinition) []ModelDefinition {
	out := make([]ModelDefinition, 0, len(catalog))
	for _, model := range catalog {
		if model.Hidden {
			continue
		}
		out = append(out, ModelDefinition{
			ID:               model.ID,
			OwnedBy:          model.OwnedBy,
			SupportsThinking: model.SupportsThinking,
		})
	}
	return out
}

var endpointProviders = map[string]map[Provider]bool{
	"openai_responses": {
		ProviderCodex: true,
		ProviderKiro:  true,
	},
	"openai_chat": {
		ProviderCodex: true,
		ProviderKiro:  true,
	},
	"openai_completions": {
		ProviderCodex: true,
		ProviderKiro:  true,
	},
	"anthropic_messages": {
		ProviderCodex: true,
		ProviderKiro:  true,
	},
	"anthropic_count_tokens": {
		ProviderCodex: true,
		ProviderKiro:  true,
	},
}

var kiroModelCatalog = []ModelDefinition{
	{ID: "auto", OwnedBy: string(ProviderKiro), SupportsThinking: true},
	{ID: "claude-opus-4.6", OwnedBy: string(ProviderKiro), SupportsThinking: true},
	{ID: "claude-sonnet-4.6", OwnedBy: string(ProviderKiro), SupportsThinking: true},
	{ID: "claude-haiku-4.6", OwnedBy: string(ProviderKiro), SupportsThinking: true},
	{ID: "claude-opus-4.5", OwnedBy: string(ProviderKiro), SupportsThinking: true},
	{ID: "claude-sonnet-4.5", OwnedBy: string(ProviderKiro), SupportsThinking: true},
	{ID: "claude-sonnet-4", OwnedBy: string(ProviderKiro), SupportsThinking: true},
	{ID: "claude-haiku-4.5", OwnedBy: string(ProviderKiro), SupportsThinking: true},
	{ID: "glm-5", OwnedBy: string(ProviderKiro), SupportsThinking: true},
	{ID: "qwen3-coder-next", OwnedBy: string(ProviderKiro), SupportsThinking: true},
	{ID: "minimax-m2.5", OwnedBy: string(ProviderKiro), SupportsThinking: true},
	{ID: "deepseek-3.2", OwnedBy: string(ProviderKiro), SupportsThinking: true},
}

var kiroModelLookup = makeModelLookup(kiroModelCatalog)

func resolveKiroModel(model string) (string, bool) {
	normalized := normalizeKiroModelName(strings.TrimSpace(model))
	if normalized == "" {
		return "", false
	}
	if resolved, ok := kiroModelLookup[strings.ToLower(normalized)]; ok {
		return resolved, true
	}
	if strings.HasPrefix(strings.ToLower(normalized), "claude-") {
		return normalized, true
	}
	return "", false
}

func normalizeKiroModelName(name string) string {
	if name == "" {
		return ""
	}
	nameLower := strings.ToLower(strings.TrimSpace(name))
	pattern1 := regexp.MustCompile(`^(claude-(?:haiku|sonnet|opus)-\d+)-(\d{1,2})(?:-(?:\d{8}|latest|\d+))?$`)
	if matches := pattern1.FindStringSubmatch(nameLower); matches != nil {
		return matches[1] + "." + matches[2]
	}
	pattern2 := regexp.MustCompile(`^(claude-(?:haiku|sonnet|opus)-\d+)(?:-\d{8})?$`)
	if matches := pattern2.FindStringSubmatch(nameLower); matches != nil {
		return matches[1]
	}
	pattern3 := regexp.MustCompile(`^(claude)-(\d+)-(\d+)-(haiku|sonnet|opus)(?:-(?:\d{8}|latest|\d+))?$`)
	if matches := pattern3.FindStringSubmatch(nameLower); matches != nil {
		return matches[1] + "-" + matches[2] + "." + matches[3] + "-" + matches[4]
	}
	pattern4 := regexp.MustCompile(`^(claude-(?:\d+\.\d+-)?(?:haiku|sonnet|opus)(?:-\d+\.\d+)?)-\d{8}$`)
	if matches := pattern4.FindStringSubmatch(nameLower); matches != nil {
		return matches[1]
	}
	pattern5 := regexp.MustCompile(`^claude-(\d+)\.(\d+)-(haiku|sonnet|opus)(?:-.*)?$`)
	if matches := pattern5.FindStringSubmatch(nameLower); matches != nil {
		return "claude-" + matches[3] + "-" + matches[1] + "." + matches[2]
	}
	return nameLower
}

func ValidateEndpointProvider(endpoint string, provider Provider) error {
	supportedProviders, ok := endpointProviders[endpoint]
	if !ok {
		return fmt.Errorf("unsupported endpoint: %s", endpoint)
	}
	if supportedProviders[provider] {
		return nil
	}
	return fmt.Errorf("unsupported provider: %s", provider)
}
