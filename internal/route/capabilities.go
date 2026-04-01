package route

import "fmt"

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
