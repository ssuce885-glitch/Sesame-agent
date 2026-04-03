package model

import (
	"fmt"

	"go-agent/internal/config"
)

func NewFromConfig(cfg config.Config) (StreamingClient, error) {
	switch cfg.ModelProvider {
	case "", "fake":
		return NewFakeStreaming(nil), nil
	case "anthropic":
		return NewAnthropicProvider(Config{
			APIKey:  cfg.AnthropicAPIKey,
			Model:   cfg.Model,
			BaseURL: cfg.AnthropicBaseURL,
		})
	default:
		return nil, fmt.Errorf("unsupported model provider %q", cfg.ModelProvider)
	}
}
