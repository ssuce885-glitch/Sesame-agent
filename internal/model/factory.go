package model

import (
	"fmt"

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
