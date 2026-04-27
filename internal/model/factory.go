package model

import (
	"fmt"
	"strings"

	"go-agent/internal/config"
)

func NewFromConfig(cfg config.Config) (StreamingClient, error) {
	if cfg.ModelProvider == "fake" {
		return NewFakeStreaming(nil), nil
	}

	resolved, err := ResolveProviderConfig(cfg)
	if err != nil {
		return nil, err
	}

	transport, err := newTransportFromResolved(resolved)
	if err != nil {
		return nil, err
	}
	return NewAdaptiveProvider(resolved, transport), nil
}

func NewVisionProviderFromConfig(cfg config.Config) (StreamingClient, error) {
	provider := strings.TrimSpace(cfg.VisionProvider)
	apiKey := strings.TrimSpace(cfg.VisionAPIKey)
	model := strings.TrimSpace(cfg.VisionModel)
	baseURL := strings.TrimSpace(cfg.VisionBaseURL)
	if baseURL == "" {
		baseURL = defaultVisionBaseURL(provider)
	}

	switch provider {
	case "anthropic":
		return NewAnthropicProvider(Config{
			APIKey:  apiKey,
			Model:   model,
			BaseURL: baseURL,
		})
	case "openai_compatible":
		return NewOpenAICompatibleProvider(Config{
			APIKey:  apiKey,
			Model:   model,
			BaseURL: baseURL,
		})
	case "":
		return nil, fmt.Errorf("vision model is not configured")
	default:
		return nil, fmt.Errorf("unsupported vision model provider %q", provider)
	}
}

func newTransportFromResolved(resolved ResolvedProviderConfig) (StreamingClient, error) {
	switch resolved.APIFamily {
	case APIFamilyAnthropicMessages:
		return NewAnthropicProvider(Config{
			APIKey:  resolved.APIKey,
			Model:   resolved.Model,
			BaseURL: resolved.BaseURL,
		})
	case APIFamilyOpenAIResponsesCompatible:
		return NewOpenAICompatibleProvider(Config{
			APIKey:       resolved.APIKey,
			Model:        resolved.Model,
			BaseURL:      resolved.BaseURL,
			CacheProfile: resolved.CacheProfile,
		})
	default:
		return nil, fmt.Errorf("unsupported api family %q", resolved.APIFamily)
	}
}

func defaultVisionBaseURL(provider string) string {
	switch strings.TrimSpace(provider) {
	case "anthropic":
		return "https://api.anthropic.com"
	case "openai_compatible":
		return "https://api.openai.com/v1"
	default:
		return ""
	}
}
