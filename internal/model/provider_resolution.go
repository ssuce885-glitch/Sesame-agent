package model

import (
	"fmt"
	"net/url"
	"strings"

	"go-agent/internal/config"
)

type APIFamily string
type ProviderProfileID string

const (
	APIFamilyAnthropicMessages         APIFamily = "anthropic_messages"
	APIFamilyOpenAIResponsesCompatible APIFamily = "openai_responses_compatible"
)

const (
	ProviderProfileAnthropicDefault        ProviderProfileID = "anthropic_default"
	ProviderProfileMiniMax                 ProviderProfileID = "minimax"
	ProviderProfileOpenAICompatibleDefault ProviderProfileID = "openai_compatible_default"
	ProviderProfileVolcengineGeneral       ProviderProfileID = "volcengine_general"
	ProviderProfileVolcengineCoding        ProviderProfileID = "volcengine_coding"
)

type ProviderProfile struct {
	ID                 ProviderProfileID
	Vendor             string
	APIFamily          APIFamily
	StrictToolSequence bool
}

type ResolvedProviderConfig struct {
	Model        string
	APIKey       string
	BaseURL      string
	Provider     string
	APIFamily    APIFamily
	Profile      ProviderProfile
	CacheProfile CapabilityProfile
}

func ResolveProviderConfig(cfg config.Config) (ResolvedProviderConfig, error) {
	switch strings.TrimSpace(cfg.ModelProvider) {
	case "anthropic":
		profile := inferAnthropicProfile(cfg.Model, cfg.AnthropicBaseURL)
		return ResolvedProviderConfig{
			Model:        cfg.Model,
			APIKey:       cfg.AnthropicAPIKey,
			BaseURL:      cfg.AnthropicBaseURL,
			Provider:     cfg.ModelProvider,
			APIFamily:    APIFamilyAnthropicMessages,
			Profile:      profile,
			CacheProfile: CapabilityProfile(cfg.ProviderCacheProfile),
		}, nil
	case "openai_compatible":
		profile := inferOpenAICompatibleProfile(cfg.Model, cfg.OpenAIBaseURL)
		return ResolvedProviderConfig{
			Model:        cfg.Model,
			APIKey:       cfg.OpenAIAPIKey,
			BaseURL:      cfg.OpenAIBaseURL,
			Provider:     cfg.ModelProvider,
			APIFamily:    APIFamilyOpenAIResponsesCompatible,
			Profile:      profile,
			CacheProfile: CapabilityProfile(cfg.ProviderCacheProfile),
		}, nil
	default:
		return ResolvedProviderConfig{}, fmt.Errorf("unsupported model provider %q", cfg.ModelProvider)
	}
}

func inferAnthropicProfile(modelName, baseURL string) ProviderProfile {
	if isMiniMax(baseURL, modelName) {
		return ProviderProfile{
			ID:                 ProviderProfileMiniMax,
			Vendor:             "minimax",
			APIFamily:          APIFamilyAnthropicMessages,
			StrictToolSequence: true,
		}
	}
	return ProviderProfile{
		ID:                 ProviderProfileAnthropicDefault,
		Vendor:             "anthropic",
		APIFamily:          APIFamilyAnthropicMessages,
		StrictToolSequence: true,
	}
}

func inferOpenAICompatibleProfile(modelName, baseURL string) ProviderProfile {
	if isVolcengine(baseURL) {
		id := ProviderProfileVolcengineGeneral
		if isVolcengineCoding(baseURL) {
			id = ProviderProfileVolcengineCoding
		}
		return ProviderProfile{
			ID:                 id,
			Vendor:             "volcengine",
			APIFamily:          APIFamilyOpenAIResponsesCompatible,
			StrictToolSequence: true,
		}
	}
	return ProviderProfile{
		ID:                 ProviderProfileOpenAICompatibleDefault,
		Vendor:             "openai_compatible",
		APIFamily:          APIFamilyOpenAIResponsesCompatible,
		StrictToolSequence: true,
	}
}

func isMiniMax(baseURL, modelName string) bool {
	baseURL = strings.ToLower(strings.TrimSpace(baseURL))
	modelName = strings.ToLower(strings.TrimSpace(modelName))
	return strings.Contains(baseURL, "minimaxi") || strings.Contains(modelName, "minimax")
}

func isVolcengine(baseURL string) bool {
	host, path := normalizedProviderURL(baseURL)
	return strings.Contains(host, "volces.com") || strings.HasPrefix(path, "/api/v3") || strings.HasPrefix(path, "/api/coding/v3")
}

func isVolcengineCoding(baseURL string) bool {
	_, path := normalizedProviderURL(baseURL)
	return strings.HasPrefix(path, "/api/coding/v3")
}

func normalizedProviderURL(raw string) (string, string) {
	if strings.TrimSpace(raw) == "" {
		return "", ""
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", ""
	}
	return strings.ToLower(strings.TrimSpace(parsed.Hostname())), strings.ToLower(strings.TrimSpace(parsed.Path))
}
