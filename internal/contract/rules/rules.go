package rules

import (
	"errors"

	"cliro-go/internal/contract"
)

type ProviderCapability struct {
	SupportsTools    bool
	SupportsThinking bool
	SupportsStream   bool
}

var capabilityByProvider = map[string]ProviderCapability{
	"codex": {
		SupportsTools:    true,
		SupportsThinking: true,
		SupportsStream:   true,
	},
	"kiro": {
		SupportsTools:    true,
		SupportsThinking: true,
		SupportsStream:   true,
	},
}

var (
	ErrUnsupportedProvider = errors.New("unsupported provider")
	ErrUnsupportedTools    = errors.New("tools are not supported by provider")
	ErrUnsupportedThinking = errors.New("thinking is not supported by provider")
	ErrUnsupportedStream   = errors.New("stream is not supported by provider")
)

func CapabilityForProvider(provider string) (ProviderCapability, bool) {
	capability, ok := capabilityByProvider[provider]
	return capability, ok
}

func ValidateRequest(request contract.Request, provider string) error {
	capability, ok := CapabilityForProvider(provider)
	if !ok {
		return ErrUnsupportedProvider
	}
	if err := ValidateToolRules(request, capability); err != nil {
		return err
	}
	if err := ValidateThinkingRules(request, capability); err != nil {
		return err
	}
	if request.Stream && !capability.SupportsStream {
		return ErrUnsupportedStream
	}
	return nil
}

func ValidateToolRules(request contract.Request, capability ProviderCapability) error {
	if len(request.Tools) > 0 && !capability.SupportsTools {
		return ErrUnsupportedTools
	}
	return nil
}

func ValidateThinkingRules(request contract.Request, capability ProviderCapability) error {
	hasThinking := false
	for _, message := range request.Messages {
		if len(message.ThinkingBlocks) > 0 {
			hasThinking = true
			break
		}
		if message.Role == contract.RoleAssistant {
			continue
		}
		if _, ok := message.Content.(map[string]any); ok {
			hasThinking = true
			break
		}
	}
	if hasThinking && !capability.SupportsThinking {
		return ErrUnsupportedThinking
	}
	return nil
}
