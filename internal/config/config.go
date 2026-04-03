package config

import (
	"errors"
	"os"
)

type Config struct {
	Addr             string
	DataDir          string
	ModelProvider    string
	Model            string
	AnthropicAPIKey  string
	AnthropicBaseURL string
	OpenAIAPIKey     string
	OpenAIBaseURL    string
	LogLevel         string
}

func Load() (Config, error) {
	model := os.Getenv("AGENTD_MODEL")
	if model == "" {
		model = envOrDefault("ANTHROPIC_MODEL", "claude-sonnet-4-5")
	}

	cfg := Config{
		Addr:             envOrDefault("AGENTD_ADDR", "127.0.0.1:4317"),
		DataDir:          envOrDefault("AGENTD_DATA_DIR", ""),
		ModelProvider:    envOrDefault("AGENTD_MODEL_PROVIDER", "anthropic"),
		Model:            model,
		AnthropicAPIKey:  envOrDefault("ANTHROPIC_API_KEY", ""),
		AnthropicBaseURL: envOrDefault("ANTHROPIC_BASE_URL", "https://api.anthropic.com"),
		OpenAIAPIKey:     envOrDefault("OPENAI_API_KEY", ""),
		OpenAIBaseURL:    envOrDefault("OPENAI_BASE_URL", ""),
		LogLevel:         envOrDefault("AGENTD_LOG_LEVEL", "info"),
	}

	if cfg.DataDir == "" {
		return Config{}, errors.New("AGENTD_DATA_DIR is required")
	}

	return cfg, nil
}

func envOrDefault(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	return value
}
