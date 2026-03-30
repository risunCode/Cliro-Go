package route

import "fmt"

func ValidateEndpointProvider(endpoint string, provider Provider) error {
	switch endpoint {
	case "openai_responses", "openai_chat", "openai_completions", "anthropic_messages", "anthropic_count_tokens":
		if provider == ProviderCodex || provider == ProviderKiro {
			return nil
		}
		return fmt.Errorf("unsupported provider: %s", provider)
	default:
		return fmt.Errorf("unsupported endpoint: %s", endpoint)
	}
}
