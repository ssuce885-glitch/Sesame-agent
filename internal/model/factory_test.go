package model

import (
	"strings"
	"testing"

	"go-agent/internal/config"
)

func TestResolveProviderConfigUsesExplicitProviderEntry(t *testing.T) {
	cfg := config.Config{
		ActiveProfile: "coding",
		ModelProviders: map[string]config.ModelProviderConfig{
			"anthropic-prod": {
				ID:        "anthropic-prod",
				APIFamily: "anthropic_messages",
				BaseURL:   "https://api.anthropic.com",
				APIKeyEnv: "ANTHROPIC_API_KEY",
			},
		},
		Profiles: map[string]config.ProfileConfig{
			"coding": {
				ID:            "coding",
				Model:         "claude-sonnet-4-5",
				ModelProvider: "anthropic-prod",
				CacheProfile:  "anthropic_default",
			},
		},
	}
	t.Setenv("ANTHROPIC_API_KEY", "test-key")

	resolved, err := ResolveProviderConfig(cfg)
	if err != nil {
		t.Fatalf("ResolveProviderConfig() error = %v", err)
	}
	if resolved.ProviderID != "anthropic-prod" {
		t.Fatalf("ProviderID = %q, want anthropic-prod", resolved.ProviderID)
	}
	if resolved.APIFamily != APIFamilyAnthropicMessages {
		t.Fatalf("APIFamily = %q, want %q", resolved.APIFamily, APIFamilyAnthropicMessages)
	}
}

func TestResolveProviderConfigRejectsMissingProviderReference(t *testing.T) {
	cfg := config.Config{
		ActiveProfile:  "coding",
		ModelProviders: map[string]config.ModelProviderConfig{},
		Profiles: map[string]config.ProfileConfig{
			"coding": {ID: "coding", Model: "claude-sonnet-4-5", ModelProvider: "missing"},
		},
	}

	_, err := ResolveProviderConfig(cfg)
	if err == nil {
		t.Fatal("ResolveProviderConfig() error = nil, want missing provider failure")
	}
	if !strings.Contains(err.Error(), `profile "coding" references unknown model_provider "missing"`) {
		t.Fatalf("error = %v", err)
	}
}
