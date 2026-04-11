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

func TestResolveProviderConfigPrefersRuntimeModelOverride(t *testing.T) {
	cfg := config.Config{
		ActiveProfile: "coding",
		Model:         "runtime-override-model",
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
				Model:         "profile-model",
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
	if resolved.Model != "runtime-override-model" {
		t.Fatalf("Model = %q, want runtime-override-model", resolved.Model)
	}
}

func TestResolveProviderConfigRejectsOpenAIChatCompletionsFamily(t *testing.T) {
	cfg := config.Config{
		ActiveProfile: "coding",
		ModelProviders: map[string]config.ModelProviderConfig{
			"openai-prod": {
				ID:        "openai-prod",
				APIFamily: "openai_chat_completions",
				BaseURL:   "https://api.openai.com/v1",
				APIKeyEnv: "OPENAI_API_KEY",
			},
		},
		Profiles: map[string]config.ProfileConfig{
			"coding": {
				ID:            "coding",
				Model:         "gpt-5.4",
				ModelProvider: "openai-prod",
				CacheProfile:  "openai_responses",
			},
		},
	}
	t.Setenv("OPENAI_API_KEY", "test-key")

	_, err := ResolveProviderConfig(cfg)
	if err == nil {
		t.Fatal("ResolveProviderConfig() error = nil, want unsupported openai_chat_completions")
	}
	if !strings.Contains(err.Error(), `uses unsupported api_family "openai_chat_completions"`) {
		t.Fatalf("error = %v", err)
	}
}
