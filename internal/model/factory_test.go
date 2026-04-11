package model

import (
	"testing"

	"go-agent/internal/config"
)

func TestNewFromConfigRejectsEmptyProvider(t *testing.T) {
	_, err := NewFromConfig(config.Config{})
	if err == nil {
		t.Fatal("NewFromConfig() error = nil, want error")
	}
}

func TestNewFromConfigSelectsOpenAICompatibleProvider(t *testing.T) {
	got, err := NewFromConfig(config.Config{
		ModelProvider: "openai_compatible",
		Model:         "gpt-4.1-mini",
		OpenAIAPIKey:  "test-key",
		OpenAIBaseURL: "https://example.com",
	})
	if err != nil {
		t.Fatalf("NewFromConfig() error = %v", err)
	}

	adaptive, ok := got.(*AdaptiveProvider)
	if !ok {
		t.Fatalf("NewFromConfig() type = %T, want *AdaptiveProvider", got)
	}
	if adaptive.ResolvedConfig().APIFamily != APIFamilyOpenAIResponsesCompatible {
		t.Fatalf("ResolvedConfig().APIFamily = %q, want %q", adaptive.ResolvedConfig().APIFamily, APIFamilyOpenAIResponsesCompatible)
	}
}

func TestNewFromConfigPassesCacheProfileToOpenAICompatible(t *testing.T) {
	got, err := NewFromConfig(config.Config{
		ModelProvider:        "openai_compatible",
		Model:                "gpt-4.1-mini",
		OpenAIAPIKey:         "test-key",
		OpenAIBaseURL:        "https://example.com",
		ProviderCacheProfile: "ark_responses",
	})
	if err != nil {
		t.Fatalf("NewFromConfig() error = %v", err)
	}

	provider, ok := got.(*AdaptiveProvider)
	if !ok {
		t.Fatalf("NewFromConfig() type = %T, want *AdaptiveProvider", got)
	}
	if provider.ResolvedConfig().CacheProfile != CapabilityProfileArkResponses {
		t.Fatalf("cacheProfile = %q, want %q", provider.ResolvedConfig().CacheProfile, CapabilityProfileArkResponses)
	}
}

func TestResolveProviderConfigInfersMiniMaxProfile(t *testing.T) {
	resolved, err := ResolveProviderConfig(config.Config{
		ModelProvider:    "anthropic",
		Model:            "MiniMax-M2.7",
		AnthropicAPIKey:  "test-key",
		AnthropicBaseURL: "https://api.minimaxi.com/anthropic",
	})
	if err != nil {
		t.Fatalf("ResolveProviderConfig() error = %v", err)
	}

	if resolved.Profile.ID != ProviderProfileMiniMax {
		t.Fatalf("Profile.ID = %q, want %q", resolved.Profile.ID, ProviderProfileMiniMax)
	}
	if resolved.Profile.Vendor != "minimax" {
		t.Fatalf("Profile.Vendor = %q, want minimax", resolved.Profile.Vendor)
	}
}

func TestResolveProviderConfigInfersVolcengineCodingProfile(t *testing.T) {
	resolved, err := ResolveProviderConfig(config.Config{
		ModelProvider: "openai_compatible",
		Model:         "ark-code-latest",
		OpenAIAPIKey:  "test-key",
		OpenAIBaseURL: "https://ark.cn-beijing.volces.com/api/coding/v3",
	})
	if err != nil {
		t.Fatalf("ResolveProviderConfig() error = %v", err)
	}

	if resolved.Profile.ID != ProviderProfileVolcengineCoding {
		t.Fatalf("Profile.ID = %q, want %q", resolved.Profile.ID, ProviderProfileVolcengineCoding)
	}
	if resolved.Profile.Vendor != "volcengine" {
		t.Fatalf("Profile.Vendor = %q, want volcengine", resolved.Profile.Vendor)
	}
}
