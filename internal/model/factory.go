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

func NewClassifierClient(cfg config.Config) (StreamingClient, error) {
	provider := strings.TrimSpace(cfg.ClassifierProvider)
	if provider == "" {
		provider = strings.TrimSpace(cfg.ModelProvider)
	}
	if strings.TrimSpace(cfg.ClassifierModel) == "" || provider == "fake" {
		return nil, nil
	}

	classifierCfg := config.Config{
		ModelProvider: provider,
		CompatMode:    cfg.CompatMode,
		Model:         strings.TrimSpace(cfg.ClassifierModel),
	}
	switch provider {
	case "anthropic":
		classifierCfg.AnthropicAPIKey = firstNonEmptyString(cfg.ClassifierAPIKey, primaryAPIKey(cfg))
		classifierCfg.AnthropicBaseURL = firstNonEmptyString(cfg.ClassifierBaseURL, primaryBaseURL(cfg))
	case "openai_compatible":
		classifierCfg.OpenAIAPIKey = firstNonEmptyString(cfg.ClassifierAPIKey, primaryAPIKey(cfg))
		classifierCfg.OpenAIBaseURL = firstNonEmptyString(cfg.ClassifierBaseURL, primaryBaseURL(cfg))
	}

	resolved, err := ResolveProviderConfig(classifierCfg)
	if err != nil {
		return nil, err
	}
	return newTransportFromResolved(resolved)
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

func primaryAPIKey(cfg config.Config) string {
	switch strings.TrimSpace(cfg.ModelProvider) {
	case "openai_compatible":
		return cfg.OpenAIAPIKey
	case "anthropic":
		return cfg.AnthropicAPIKey
	default:
		return ""
	}
}

func primaryBaseURL(cfg config.Config) string {
	switch strings.TrimSpace(cfg.ModelProvider) {
	case "openai_compatible":
		return cfg.OpenAIBaseURL
	case "anthropic":
		return cfg.AnthropicBaseURL
	default:
		return ""
	}
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
