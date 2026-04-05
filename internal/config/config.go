package config

import (
	"errors"
	"os"
	"strconv"
	"strings"
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
	SystemPrompt               string
	SystemPromptFile           string
	MaxWorkspacePromptBytes    int
}

func Load() (Config, error) {
	uc, err := loadUserConfig()
	if err != nil {
		return Config{}, err
	}

	openAIModelFallback := uc.OpenAI.Model
	model := envOrDefaultWithFallback("AGENTD_MODEL", openAIModelFallback, "")
	if model == "" {
		model = envOrDefaultWithFallback("ANTHROPIC_MODEL", uc.Anthropic.Model, "claude-sonnet-4-5")
	}

	cfg := Config{
		Addr:                       envOrDefaultWithFallback("AGENTD_ADDR", uc.Listen.Addr, "127.0.0.1:4317"),
		DataDir:                    envOrDefault("AGENTD_DATA_DIR", ""),
		ModelProvider:              envOrDefaultWithFallback("AGENTD_MODEL_PROVIDER", uc.Provider, "anthropic"),
		Model:                      model,
		AnthropicAPIKey:            envOrDefaultWithFallback("ANTHROPIC_API_KEY", uc.Anthropic.APIKey, ""),
		AnthropicBaseURL:           envOrDefaultWithFallback("ANTHROPIC_BASE_URL", uc.Anthropic.BaseURL, "https://api.anthropic.com"),
		OpenAIAPIKey:               envOrDefaultWithFallback("OPENAI_API_KEY", uc.OpenAI.APIKey, ""),
		OpenAIBaseURL:              envOrDefaultWithFallback("OPENAI_BASE_URL", uc.OpenAI.BaseURL, ""),
		ProviderCacheProfile:       envOrDefault("AGENTD_PROVIDER_CACHE_PROFILE", "none"),
		CacheExpirySeconds:         intEnvOrDefault("AGENTD_CACHE_EXPIRY_SECONDS", 86400),
		MicrocompactBytesThreshold: intEnvOrDefault("AGENTD_MICROCOMPACT_BYTES_THRESHOLD", 4096),
		LogLevel:                   envOrDefault("AGENTD_LOG_LEVEL", "info"),
		PermissionProfile:          envOrDefault("AGENTD_PERMISSION_PROFILE", "read_only"),
		MaxToolSteps:               intEnvOrDefaultWithFallback("AGENTD_MAX_TOOL_STEPS", uc.MaxToolSteps, 8),
		MaxShellOutputBytes:        intEnvOrDefault("AGENTD_MAX_SHELL_OUTPUT_BYTES", 4096),
		ShellTimeoutSeconds:        intEnvOrDefault("AGENTD_SHELL_TIMEOUT_SECONDS", 30),
		MaxFileWriteBytes:          intEnvOrDefault("AGENTD_MAX_FILE_WRITE_BYTES", 1<<20),
		MaxRecentItems:             intEnvOrDefault("AGENTD_MAX_RECENT_ITEMS", 8),
		CompactionThreshold:        intEnvOrDefault("AGENTD_COMPACTION_THRESHOLD", 16),
		MaxEstimatedTokens:         intEnvOrDefault("AGENTD_MAX_ESTIMATED_TOKENS", 6000),
		MaxCompactionPasses:        intEnvOrDefault("AGENTD_MAX_COMPACTION_PASSES", 1),
		SystemPrompt:               envOrDefault("AGENTD_SYSTEM_PROMPT", ""),
		SystemPromptFile:           envOrDefault("AGENTD_SYSTEM_PROMPT_FILE", ""),
		MaxWorkspacePromptBytes:    intEnvOrDefault("AGENTD_MAX_WORKSPACE_PROMPT_BYTES", 32768),
	}

	if cfg.DataDir == "" {
		return Config{}, errors.New("AGENTD_DATA_DIR is required")
	}

	return cfg, nil
}

func (c Config) ResolveSystemPrompt() (string, error) {
	if strings.TrimSpace(c.SystemPrompt) != "" {
		return strings.TrimSpace(c.SystemPrompt), nil
	}
	if strings.TrimSpace(c.SystemPromptFile) == "" {
		return "", nil
	}

	data, err := os.ReadFile(c.SystemPromptFile)
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(string(data)), nil
}

func envOrDefaultWithFallback(key, fileFallback, hardDefault string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	if fileFallback != "" {
		return fileFallback
	}
	return hardDefault
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

func intEnvOrDefaultWithFallback(key string, fileFallback int, hardDefault int) int {
	value := os.Getenv(key)
	if value != "" {
		if parsed, err := strconv.Atoi(value); err == nil {
			return parsed
		}
	}
	if fileFallback != 0 {
		return fileFallback
	}
	return hardDefault
}
