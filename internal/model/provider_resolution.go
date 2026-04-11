package model

import (
	"fmt"
	"os"
	"strings"

	"go-agent/internal/config"
)

type APIFamily string

const (
	APIFamilyAnthropicMessages     APIFamily = "anthropic_messages"
	APIFamilyOpenAIChatCompletions APIFamily = "openai_chat_completions"
	APIFamilyOpenAIResponses       APIFamily = "openai_responses"
	APIFamilyAnthropic             APIFamily = "anthropic"
	APIFamilyOpenAICompatible      APIFamily = "openai_compatible"
	APIFamilyOpenAI                APIFamily = "openai"
)

type ProviderProfile string

const (
	ProviderProfileAnthropicDefault ProviderProfile = "anthropic_default"
	ProviderProfileOpenAIResponses  ProviderProfile = "openai_responses"
)

type ResolvedProviderConfig struct {
	ProfileID    string
	ProviderID   string
	Model        string
	APIKey       string
	BaseURL      string
	APIFamily    APIFamily
	Profile      ProviderProfile
	CacheProfile CapabilityProfile
	Reasoning    string
	Verbosity    string
}

func ResolveProviderConfig(cfg config.Config) (ResolvedProviderConfig, error) {
	profile, ok := cfg.Profiles[cfg.ActiveProfile]
	if !ok {
		return ResolvedProviderConfig{}, fmt.Errorf("active profile %q not found", cfg.ActiveProfile)
	}

	profileID := strings.TrimSpace(profile.ID)
	if profileID == "" {
		profileID = strings.TrimSpace(cfg.ActiveProfile)
	}

	providerRef := strings.TrimSpace(profile.ModelProvider)
	provider, ok := cfg.ModelProviders[providerRef]
	if !ok {
		return ResolvedProviderConfig{}, fmt.Errorf("profile %q references unknown model_provider %q", profileID, providerRef)
	}

	providerID := strings.TrimSpace(provider.ID)
	if providerID == "" {
		providerID = providerRef
	}

	apiKeyEnv := strings.TrimSpace(provider.APIKeyEnv)
	apiKey := strings.TrimSpace(os.Getenv(apiKeyEnv))
	if apiKey == "" {
		return ResolvedProviderConfig{}, fmt.Errorf("provider %q requires env %q", providerID, apiKeyEnv)
	}

	apiFamily, err := normalizeAPIFamily(provider.APIFamily)
	if err != nil {
		return ResolvedProviderConfig{}, fmt.Errorf("provider %q %w", providerID, err)
	}

	return ResolvedProviderConfig{
		ProfileID:    profileID,
		ProviderID:   providerID,
		Model:        strings.TrimSpace(profile.Model),
		APIKey:       apiKey,
		BaseURL:      strings.TrimSpace(provider.BaseURL),
		APIFamily:    apiFamily,
		Profile:      providerProfileFor(provider),
		CacheProfile: CapabilityProfile(strings.TrimSpace(profile.CacheProfile)),
		Reasoning:    strings.TrimSpace(profile.Reasoning),
		Verbosity:    strings.TrimSpace(profile.Verbosity),
	}, nil
}

func providerProfileFor(provider config.ModelProviderConfig) ProviderProfile {
	profile := strings.TrimSpace(provider.ProfileID)
	if profile != "" {
		return ProviderProfile(profile)
	}

	apiFamily, err := normalizeAPIFamily(provider.APIFamily)
	if err != nil {
		return ""
	}
	switch apiFamily {
	case APIFamilyAnthropicMessages:
		return ProviderProfileAnthropicDefault
	case APIFamilyOpenAIChatCompletions, APIFamilyOpenAIResponses:
		return ProviderProfileOpenAIResponses
	default:
		return ""
	}
}

func normalizeAPIFamily(raw string) (APIFamily, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case string(APIFamilyAnthropic), string(APIFamilyAnthropicMessages):
		return APIFamilyAnthropicMessages, nil
	case string(APIFamilyOpenAI), string(APIFamilyOpenAICompatible), string(APIFamilyOpenAIChatCompletions):
		return APIFamilyOpenAIChatCompletions, nil
	case string(APIFamilyOpenAIResponses):
		return APIFamilyOpenAIResponses, nil
	default:
		return "", fmt.Errorf("uses unsupported api_family %q", strings.TrimSpace(raw))
	}
}
