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

type ModelResolution struct {
	Provider          Provider
	RequestedModel    string
	ResolvedModel     string
	ThinkingRequested bool
}

type ModelDefinition struct {
	ID               string
	OwnedBy          string
	SupportsThinking bool
	Hidden           bool
}

func ResolveModel(model string, thinkingSuffix string, aliases map[string]string) (ModelResolution, error) {
	requested := strings.TrimSpace(model)
	if requested == "" {
		return ModelResolution{}, fmt.Errorf("model is required")
	}

	resolvedBase, thinkingRequested := splitThinkingSuffix(requested, thinkingSuffix)

	// Check alias first
	if aliasTarget, ok := aliases[resolvedBase]; ok && strings.TrimSpace(aliasTarget) != "" {
		resolvedBase = strings.TrimSpace(aliasTarget)
	}

	if resolvedModel, ok := resolveCodexModel(resolvedBase); ok {
		return ModelResolution{
			Provider:          ProviderCodex,
			RequestedModel:    requested,
			ResolvedModel:     resolvedModel,
			ThinkingRequested: thinkingRequested,
		}, nil
	}

	if resolvedModel, ok := resolveKiroModel(resolvedBase); ok {
		return ModelResolution{
			Provider:          ProviderKiro,
			RequestedModel:    requested,
			ResolvedModel:     resolvedModel,
			ThinkingRequested: thinkingRequested,
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
