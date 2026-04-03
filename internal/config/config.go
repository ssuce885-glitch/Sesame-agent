package config

import (
	"errors"
	"os"
)

type Config struct {
	Addr            string
	DataDir         string
	Model           string
	LogLevel        string
	AnthropicAPIKey string
}

func Load() (Config, error) {
	cfg := Config{
		Addr:            envOrDefault("AGENTD_ADDR", "127.0.0.1:4317"),
		DataDir:         os.Getenv("AGENTD_DATA_DIR"),
		Model:           envOrDefault("ANTHROPIC_MODEL", "claude-sonnet-4-5"),
		LogLevel:        envOrDefault("AGENTD_LOG_LEVEL", "info"),
		AnthropicAPIKey: os.Getenv("ANTHROPIC_API_KEY"),
	}

	if cfg.DataDir == "" {
		return cfg, errors.New("AGENTD_DATA_DIR is required")
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
