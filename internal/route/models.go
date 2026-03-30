package route

import (
	"fmt"
	"strings"
)

type Provider string

const (
	ProviderCodex Provider = "codex"
	ProviderKiro  Provider = "kiro"

	DefaultThinkingSuffix = "-thinking"
)

type Resolution struct {
	Provider        Provider
	RequestedModel  string
	ResolvedModel   string
	ThinkingEnabled bool
}

type ModelDefinition struct {
	ID               string
	OwnedBy          string
	SupportsThinking bool
}

var codexModelPrefixes = []string{
	"gpt-",
	"o1",
	"o3",
	"o4",
}

var kiroModelPrefixes = []string{
	"claude-",
	"minimax",
	"deepseek",
	"qwen",
}

var modelCatalog = []ModelDefinition{
	{ID: "gpt-5.1-codex-max", OwnedBy: "codex", SupportsThinking: true},
	{ID: "gpt-5.1-codex-mini", OwnedBy: "codex", SupportsThinking: true},
	{ID: "gpt-5.2", OwnedBy: "codex", SupportsThinking: true},
	{ID: "gpt-5.4", OwnedBy: "codex", SupportsThinking: true},
	{ID: "gpt-5.2-codex", OwnedBy: "codex", SupportsThinking: true},
	{ID: "gpt-5.3-codex", OwnedBy: "codex", SupportsThinking: true},
	{ID: "gpt-5.1-codex", OwnedBy: "codex", SupportsThinking: true},
	{ID: "claude-sonnet-4.5", OwnedBy: "kiro", SupportsThinking: true},
	{ID: "claude-sonnet-4", OwnedBy: "kiro", SupportsThinking: true},
	{ID: "claude-haiku-4.5", OwnedBy: "kiro", SupportsThinking: true},
	{ID: "claude-opus-4.5", OwnedBy: "kiro", SupportsThinking: true},
	{ID: "qwen3-coder-next", OwnedBy: "kiro", SupportsThinking: true},
	{ID: "minimax-m2.5", OwnedBy: "kiro", SupportsThinking: true},
}

func ResolveModel(model string, thinkingSuffix string) (Resolution, error) {
	requested := strings.TrimSpace(model)
	if requested == "" {
		return Resolution{}, fmt.Errorf("model is required")
	}

	resolved, thinkingEnabled := splitThinkingSuffix(requested, thinkingSuffix)
	provider, ok := providerForModel(resolved)
	if !ok {
		return Resolution{}, fmt.Errorf("unsupported model: %s", requested)
	}
	if provider != ProviderKiro {
		thinkingEnabled = false
	}

	return Resolution{
		Provider:        provider,
		RequestedModel:  requested,
		ResolvedModel:   resolved,
		ThinkingEnabled: thinkingEnabled,
	}, nil
}

func providerForModel(model string) (Provider, bool) {
	normalized := strings.ToLower(strings.TrimSpace(model))
	if normalized == "" {
		return "", false
	}

	for _, prefix := range codexModelPrefixes {
		if strings.HasPrefix(normalized, prefix) {
			return ProviderCodex, true
		}
	}

	for _, prefix := range kiroModelPrefixes {
		if strings.HasPrefix(normalized, prefix) {
			return ProviderKiro, true
		}
	}

	return "", false
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

func DefaultStreamingEnabled(model string, endpoint string) bool {
	resolution, err := ResolveModel(model, DefaultThinkingSuffix)
	if err != nil {
		return false
	}
	return resolution.ThinkingEnabled && endpoint != "anthropic_count_tokens"
}

func CatalogModels(thinkingSuffix string) []ModelDefinition {
	resolvedSuffix := strings.TrimSpace(thinkingSuffix)
	if resolvedSuffix == "" {
		resolvedSuffix = DefaultThinkingSuffix
	}

	models := make([]ModelDefinition, 0, len(modelCatalog)*2)
	for _, model := range modelCatalog {
		models = append(models, model)
		if model.SupportsThinking && model.OwnedBy == string(ProviderKiro) {
			models = append(models, ModelDefinition{ID: model.ID + resolvedSuffix, OwnedBy: model.OwnedBy, SupportsThinking: false})
		}
	}
	return models
}
