package model

import "errors"

type Config struct {
	APIKey string
	Model  string
}

type AnthropicProvider struct {
	apiKey string
	model  string
}

func NewAnthropicProvider(cfg Config) (*AnthropicProvider, error) {
	if cfg.APIKey == "" {
		return nil, errors.New("anthropic api key is required")
	}
	if cfg.Model == "" {
		return nil, errors.New("anthropic model is required")
	}

	return &AnthropicProvider{
		apiKey: cfg.APIKey,
		model:  cfg.Model,
	}, nil
}
