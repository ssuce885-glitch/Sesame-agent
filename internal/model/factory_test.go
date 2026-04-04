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

	if _, ok := got.(*OpenAICompatibleProvider); !ok {
		t.Fatalf("NewFromConfig() type = %T, want *OpenAICompatibleProvider", got)
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

	provider, ok := got.(*OpenAICompatibleProvider)
	if !ok {
		t.Fatalf("NewFromConfig() type = %T, want *OpenAICompatibleProvider", got)
	}
	if provider.cacheProfile != CapabilityProfileArkResponses {
		t.Fatalf("cacheProfile = %q, want %q", provider.cacheProfile, CapabilityProfileArkResponses)
	}
}
