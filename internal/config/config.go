package config

import (
	"errors"
	"os"
	"strconv"
)

type Config struct {
	Addr                       string
	DataDir                    string
	ModelProvider              string
	Model                      string
	AnthropicAPIKey            string
	AnthropicBaseURL           string
	OpenAIAPIKey               string
	OpenAIBaseURL              string
	ProviderCacheProfile       string
	CacheExpirySeconds         int
	MicrocompactBytesThreshold int
	LogLevel                   string
	PermissionProfile          string
	MaxToolSteps               int
	MaxShellOutputBytes        int
	ShellTimeoutSeconds        int
	MaxFileWriteBytes          int
	MaxRecentItems             int
	CompactionThreshold        int
	MaxEstimatedTokens         int
	MaxCompactionPasses        int
}

func Load() (Config, error) {
	model := os.Getenv("AGENTD_MODEL")
	if model == "" {
		model = envOrDefault("ANTHROPIC_MODEL", "claude-sonnet-4-5")
	}

	cfg := Config{
		Addr:                       envOrDefault("AGENTD_ADDR", "127.0.0.1:4317"),
		DataDir:                    envOrDefault("AGENTD_DATA_DIR", ""),
		ModelProvider:              envOrDefault("AGENTD_MODEL_PROVIDER", "anthropic"),
		Model:                      model,
		AnthropicAPIKey:            envOrDefault("ANTHROPIC_API_KEY", ""),
		AnthropicBaseURL:           envOrDefault("ANTHROPIC_BASE_URL", "https://api.anthropic.com"),
		OpenAIAPIKey:               envOrDefault("OPENAI_API_KEY", ""),
		OpenAIBaseURL:              envOrDefault("OPENAI_BASE_URL", ""),
		ProviderCacheProfile:       envOrDefault("AGENTD_PROVIDER_CACHE_PROFILE", "none"),
		CacheExpirySeconds:         intEnvOrDefault("AGENTD_CACHE_EXPIRY_SECONDS", 86400),
		MicrocompactBytesThreshold: intEnvOrDefault("AGENTD_MICROCOMPACT_BYTES_THRESHOLD", 4096),
		LogLevel:                   envOrDefault("AGENTD_LOG_LEVEL", "info"),
		PermissionProfile:          envOrDefault("AGENTD_PERMISSION_PROFILE", "read_only"),
		MaxToolSteps:               intEnvOrDefault("AGENTD_MAX_TOOL_STEPS", 8),
		MaxShellOutputBytes:        intEnvOrDefault("AGENTD_MAX_SHELL_OUTPUT_BYTES", 4096),
		ShellTimeoutSeconds:        intEnvOrDefault("AGENTD_SHELL_TIMEOUT_SECONDS", 30),
		MaxFileWriteBytes:          intEnvOrDefault("AGENTD_MAX_FILE_WRITE_BYTES", 1<<20),
		MaxRecentItems:             intEnvOrDefault("AGENTD_MAX_RECENT_ITEMS", 8),
		CompactionThreshold:        intEnvOrDefault("AGENTD_COMPACTION_THRESHOLD", 16),
		MaxEstimatedTokens:         intEnvOrDefault("AGENTD_MAX_ESTIMATED_TOKENS", 6000),
		MaxCompactionPasses:        intEnvOrDefault("AGENTD_MAX_COMPACTION_PASSES", 1),
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

func intEnvOrDefault(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}

	return parsed
}
