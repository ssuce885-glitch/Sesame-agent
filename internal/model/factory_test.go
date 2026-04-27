package model

import (
	"strings"
	"testing"

	"go-agent/internal/config"
)

func TestNewVisionProviderFromConfigOpenAICompatible(t *testing.T) {
	client, err := NewVisionProviderFromConfig(config.Config{
		VisionProvider: "openai_compatible",
		VisionAPIKey:   "vision-key",
		VisionModel:    "vision-model",
	})
	if err != nil {
		t.Fatalf("NewVisionProviderFromConfig returned error: %v", err)
	}

	provider, ok := client.(*OpenAICompatibleProvider)
	if !ok {
		t.Fatalf("client type = %T, want *OpenAICompatibleProvider", client)
	}
	if provider.baseURL != "https://api.openai.com/v1" {
		t.Fatalf("baseURL = %q, want https://api.openai.com/v1", provider.baseURL)
	}
}

func TestNewVisionProviderFromConfigAnthropic(t *testing.T) {
	client, err := NewVisionProviderFromConfig(config.Config{
		VisionProvider: "anthropic",
		VisionAPIKey:   "vision-key",
		VisionModel:    "vision-model",
	})
	if err != nil {
		t.Fatalf("NewVisionProviderFromConfig returned error: %v", err)
	}

	provider, ok := client.(*AnthropicProvider)
	if !ok {
		t.Fatalf("client type = %T, want *AnthropicProvider", client)
	}
	if provider.baseURL != "https://api.anthropic.com" {
		t.Fatalf("baseURL = %q, want https://api.anthropic.com", provider.baseURL)
	}
}

func TestNewVisionProviderFromConfigRequiresConfiguration(t *testing.T) {
	_, err := NewVisionProviderFromConfig(config.Config{})
	if err == nil {
		t.Fatal("NewVisionProviderFromConfig returned nil error, want configuration error")
	}
	if !strings.Contains(err.Error(), "vision model is not configured") {
		t.Fatalf("error = %q, want not configured guidance", err.Error())
	}
}
