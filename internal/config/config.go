package config

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/url"
	"os"
	"strconv"
	"strings"
)

type CLIStartupOverrides struct {
	DataDir        string
	Addr           string
	Model          string
	PermissionMode string
	WorkspaceRoot  string
}

type Config struct {
	Addr                       string
	DataDir                    string
	ModelProvider              string
	CompatMode                 string
	Model                      string
	AnthropicAPIKey            string
	AnthropicBaseURL           string
	OpenAIAPIKey               string
	OpenAIBaseURL              string
	VisionProvider             string
	VisionAPIKey               string
	VisionBaseURL              string
	VisionModel                string
	ProviderCacheProfile       string
	CacheExpirySeconds         int
	MicrocompactBytesThreshold int
	MaxCompactionBatchItems    int
	MaxToolResultStoreBytes    int
	LogLevel                   string
	PermissionProfile          string
	MaxToolSteps               int
	// MaxShellOutputBytes defaults to 65536 and can be tuned with SESAME_MAX_SHELL_OUTPUT_BYTES for larger shell_command reports.
	MaxShellOutputBytes          int
	ShellTimeoutSeconds          int
	MaxFileWriteBytes            int
	MaxRecentItems               int
	CompactionThreshold          int
	MaxEstimatedTokens           int
	ModelContextWindow           int
	MaxCompactionPasses          int
	SystemPrompt                 string
	SystemPromptFile             string
	MaxWorkspacePromptBytes      int
	MaxConcurrentTasks           int
	TaskOutputMaxBytes           int
	RemoteExecutorShimCommand    string
	RemoteExecutorTimeoutSeconds int
	DaemonID                     string
	Paths                        Paths
	ConfigFingerprint            string
}

type CLIConfig struct {
	ShowExtensionsOnStartup bool
}

func Load() (Config, error) {
	return loadConfig(CLIStartupOverrides{})
}

func ResolveCLIStartupConfig(overrides CLIStartupOverrides) (Config, error) {
	return loadConfig(overrides)
}

func LoadCLIConfig() (CLIConfig, error) {
	fileCfg, err := loadCLIConfigFile()
	if err != nil {
		return CLIConfig{}, err
	}
	cfg := CLIConfig{
		ShowExtensionsOnStartup: true,
	}
	if fileCfg.ShowExtensionsOnStartup != nil {
		cfg.ShowExtensionsOnStartup = *fileCfg.ShowExtensionsOnStartup
	}
	return cfg, nil
}

func MissingSetupFields(cfg Config) []string {
	missing := make([]string, 0, 4)
	if strings.TrimSpace(cfg.ModelProvider) == "" {
		missing = append(missing, "provider")
	}
	if strings.TrimSpace(cfg.Model) == "" {
		missing = append(missing, "model")
	}

	switch strings.TrimSpace(cfg.ModelProvider) {
	case "fake":
		return missing
	case "openai_compatible":
		if strings.TrimSpace(cfg.OpenAIBaseURL) == "" {
			missing = append(missing, "openai.base_url")
		}
		if strings.TrimSpace(cfg.OpenAIAPIKey) == "" {
			missing = append(missing, "openai.api_key")
		}
	default:
		if strings.TrimSpace(cfg.AnthropicAPIKey) == "" {
			missing = append(missing, "anthropic.api_key")
		}
	}

	return missing
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

func loadConfig(overrides CLIStartupOverrides) (Config, error) {
	uc, err := loadUserConfig()
	if err != nil {
		return Config{}, err
	}

	workspaceRoot := strings.TrimSpace(overrides.WorkspaceRoot)
	if workspaceRoot == "" {
		if cwd, cwdErr := os.Getwd(); cwdErr == nil {
			workspaceRoot = cwd
		}
	}

	pathDataDir := firstNonEmpty(strings.TrimSpace(overrides.DataDir), envOrDefault("SESAME_DATA_DIR", ""), uc.DataDir)
	paths, err := ResolvePaths(workspaceRoot, pathDataDir)
	if err != nil {
		return Config{}, err
	}

	modelProvider := firstNonEmpty(
		envOrDefault("SESAME_MODEL_PROVIDER", ""),
	)
	compatMode := firstNonEmpty(
		envOrDefault("SESAME_COMPAT_MODE", ""),
		uc.CompatMode,
	)
	genericBaseURL := firstNonEmpty(
		envOrDefault("SESAME_BASE_URL", ""),
		uc.BaseURL,
	)
	genericAPIKey := firstNonEmpty(
		envOrDefault("SESAME_API_KEY", ""),
		uc.APIKey,
	)
	modelProvider = resolveModelProvider(modelProvider, compatMode, genericBaseURL)
	modelProvider = firstNonEmpty(
		modelProvider,
		uc.Provider,
		"anthropic",
	)
	model := firstNonEmpty(
		strings.TrimSpace(overrides.Model),
		envOrDefault("SESAME_MODEL", ""),
		uc.Model,
		providerModelFallback(modelProvider, uc),
	)
	primaryAnthropicAPIKey := selectedProviderAPIKey(modelProvider == "anthropic", genericAPIKey, envOrDefaultWithFallback("ANTHROPIC_API_KEY", uc.Anthropic.APIKey, ""))
	primaryAnthropicBaseURL := selectedProviderBaseURL(modelProvider == "anthropic", genericBaseURL, envOrDefaultWithFallback("ANTHROPIC_BASE_URL", uc.Anthropic.BaseURL, ""), "https://api.anthropic.com")
	primaryOpenAIAPIKey := selectedProviderAPIKey(modelProvider == "openai_compatible", genericAPIKey, envOrDefaultWithFallback("OPENAI_API_KEY", uc.OpenAI.APIKey, ""))
	primaryOpenAIBaseURL := selectedProviderBaseURL(modelProvider == "openai_compatible", genericBaseURL, envOrDefaultWithFallback("OPENAI_BASE_URL", uc.OpenAI.BaseURL, ""), "")
	visionProvider := firstNonEmpty(envOrDefault("SESAME_VISION_PROVIDER", ""), uc.Vision.Provider)
	visionAPIKey := firstNonEmpty(envOrDefault("SESAME_VISION_API_KEY", ""), uc.Vision.APIKey)
	visionBaseURL := firstNonEmpty(envOrDefault("SESAME_VISION_BASE_URL", ""), uc.Vision.BaseURL, visionDefaultURL(visionProvider))
	visionModel := firstNonEmpty(envOrDefault("SESAME_VISION_MODEL", ""), uc.Vision.Model)
	cfg := Config{
		Addr:                         firstNonEmpty(strings.TrimSpace(overrides.Addr), envOrDefaultWithFallback("SESAME_ADDR", uc.Listen.Addr, "127.0.0.1:4317")),
		DataDir:                      paths.DataDir,
		ModelProvider:                modelProvider,
		CompatMode:                   compatMode,
		Model:                        model,
		AnthropicAPIKey:              primaryAnthropicAPIKey,
		AnthropicBaseURL:             primaryAnthropicBaseURL,
		OpenAIAPIKey:                 primaryOpenAIAPIKey,
		OpenAIBaseURL:                primaryOpenAIBaseURL,
		VisionProvider:               visionProvider,
		VisionAPIKey:                 visionAPIKey,
		VisionBaseURL:                visionBaseURL,
		VisionModel:                  visionModel,
		ProviderCacheProfile:         firstNonEmpty(envOrDefault("SESAME_PROVIDER_CACHE_PROFILE", ""), defaultProviderCacheProfile(modelProvider, uc, genericBaseURL), "none"),
		CacheExpirySeconds:           intEnvOrDefault("SESAME_CACHE_EXPIRY_SECONDS", 86400),
		MicrocompactBytesThreshold:   intEnvOrDefaultWithFallback("SESAME_MICROCOMPACT_BYTES_THRESHOLD", uc.MicrocompactBytesThreshold, 4096),
		MaxCompactionBatchItems:      intEnvOrDefaultWithFallback("SESAME_MAX_COMPACTION_BATCH_ITEMS", uc.MaxCompactionBatchItems, 500),
		MaxToolResultStoreBytes:      intEnvOrDefault("SESAME_MAX_TOOL_RESULT_STORE_BYTES", 16384),
		LogLevel:                     envOrDefault("SESAME_LOG_LEVEL", "info"),
		PermissionProfile:            firstNonEmpty(strings.TrimSpace(overrides.PermissionMode), envOrDefaultWithFallback("SESAME_PERMISSION_PROFILE", uc.PermissionProfile, "trusted_local")),
		MaxToolSteps:                 intEnvOrDefaultWithFallback("SESAME_MAX_TOOL_STEPS", uc.MaxToolSteps, 100),
		MaxShellOutputBytes:          intEnvOrDefault("SESAME_MAX_SHELL_OUTPUT_BYTES", 65536),
		ShellTimeoutSeconds:          intEnvOrDefault("SESAME_SHELL_TIMEOUT_SECONDS", 30),
		MaxFileWriteBytes:            intEnvOrDefault("SESAME_MAX_FILE_WRITE_BYTES", 1<<20),
		MaxRecentItems:               intEnvOrDefaultWithFallback("SESAME_MAX_RECENT_ITEMS", uc.MaxRecentItems, 12),
		CompactionThreshold:          intEnvOrDefaultWithFallback("SESAME_COMPACTION_THRESHOLD", uc.CompactionThreshold, 32),
		MaxEstimatedTokens:           intEnvOrDefaultWithFallback("SESAME_MAX_ESTIMATED_TOKENS", uc.MaxEstimatedTokens, 16000),
		ModelContextWindow:           intEnvOrDefaultWithFallback("SESAME_MODEL_CONTEXT_WINDOW", uc.ModelContextWindow, 200000),
		MaxCompactionPasses:          intEnvOrDefaultWithFallback("SESAME_MAX_COMPACTION_PASSES", uc.MaxCompactionPasses, 1),
		SystemPrompt:                 envOrDefaultWithFallback("SESAME_SYSTEM_PROMPT", uc.SystemPrompt, ""),
		SystemPromptFile:             envOrDefaultWithFallback("SESAME_SYSTEM_PROMPT_FILE", uc.SystemPromptFile, ""),
		MaxWorkspacePromptBytes:      intEnvOrDefault("SESAME_MAX_WORKSPACE_PROMPT_BYTES", 32768),
		MaxConcurrentTasks:           intEnvOrDefault("SESAME_MAX_CONCURRENT_TASKS", 8),
		TaskOutputMaxBytes:           intEnvOrDefault("SESAME_TASK_OUTPUT_MAX_BYTES", 1<<20),
		RemoteExecutorShimCommand:    envOrDefault("SESAME_REMOTE_EXECUTOR_SHIM_COMMAND", ""),
		RemoteExecutorTimeoutSeconds: intEnvOrDefault("SESAME_REMOTE_EXECUTOR_TIMEOUT_SECONDS", 300),
		DaemonID:                     envOrDefault("SESAME_DAEMON_ID", ""),
		Paths:                        paths,
	}
	cfg.ConfigFingerprint = cfg.Fingerprint()
	return cfg, nil
}

func (c Config) Fingerprint() string {
	payload := struct {
		Addr                         string `json:"addr"`
		DataDir                      string `json:"data_dir"`
		ModelProvider                string `json:"provider"`
		CompatMode                   string `json:"compat_mode"`
		Model                        string `json:"model"`
		AnthropicAPIKey              string `json:"anthropic_api_key"`
		AnthropicBaseURL             string `json:"anthropic_base_url"`
		OpenAIAPIKey                 string `json:"openai_api_key"`
		OpenAIBaseURL                string `json:"openai_base_url"`
		VisionProvider               string `json:"vision_provider"`
		VisionAPIKey                 string `json:"vision_api_key"`
		VisionBaseURL                string `json:"vision_base_url"`
		VisionModel                  string `json:"vision_model"`
		ProviderCacheProfile         string `json:"provider_cache_profile"`
		LogLevel                     string `json:"log_level"`
		PermissionProfile            string `json:"permission_profile"`
		MaxToolSteps                 int    `json:"max_tool_steps"`
		MaxShellOutputBytes          int    `json:"max_shell_output_bytes"`
		ShellTimeoutSeconds          int    `json:"shell_timeout_seconds"`
		MaxFileWriteBytes            int    `json:"max_file_write_bytes"`
		MaxRecentItems               int    `json:"max_recent_items"`
		CompactionThreshold          int    `json:"compaction_threshold"`
		MaxEstimatedTokens           int    `json:"max_estimated_tokens"`
		ModelContextWindow           int    `json:"model_context_window"`
		MaxCompactionPasses          int    `json:"max_compaction_passes"`
		MaxCompactionBatchItems      int    `json:"max_compaction_batch_items"`
		MaxToolResultStoreBytes      int    `json:"max_tool_result_store_bytes"`
		SystemPrompt                 string `json:"system_prompt"`
		SystemPromptFile             string `json:"system_prompt_file"`
		MaxWorkspacePromptBytes      int    `json:"max_workspace_prompt_bytes"`
		MaxConcurrentTasks           int    `json:"max_concurrent_tasks"`
		TaskOutputMaxBytes           int    `json:"task_output_max_bytes"`
		RemoteExecutorShimCommand    string `json:"remote_executor_shim_command"`
		RemoteExecutorTimeoutSeconds int    `json:"remote_executor_timeout_seconds"`
	}{
		Addr:                         c.Addr,
		DataDir:                      c.DataDir,
		ModelProvider:                c.ModelProvider,
		CompatMode:                   c.CompatMode,
		Model:                        c.Model,
		AnthropicAPIKey:              c.AnthropicAPIKey,
		AnthropicBaseURL:             c.AnthropicBaseURL,
		OpenAIAPIKey:                 c.OpenAIAPIKey,
		OpenAIBaseURL:                c.OpenAIBaseURL,
		VisionProvider:               c.VisionProvider,
		VisionAPIKey:                 c.VisionAPIKey,
		VisionBaseURL:                c.VisionBaseURL,
		VisionModel:                  c.VisionModel,
		ProviderCacheProfile:         c.ProviderCacheProfile,
		LogLevel:                     c.LogLevel,
		PermissionProfile:            c.PermissionProfile,
		MaxToolSteps:                 c.MaxToolSteps,
		MaxShellOutputBytes:          c.MaxShellOutputBytes,
		ShellTimeoutSeconds:          c.ShellTimeoutSeconds,
		MaxFileWriteBytes:            c.MaxFileWriteBytes,
		MaxRecentItems:               c.MaxRecentItems,
		CompactionThreshold:          c.CompactionThreshold,
		MaxEstimatedTokens:           c.MaxEstimatedTokens,
		ModelContextWindow:           c.ModelContextWindow,
		MaxCompactionPasses:          c.MaxCompactionPasses,
		MaxCompactionBatchItems:      c.MaxCompactionBatchItems,
		MaxToolResultStoreBytes:      c.MaxToolResultStoreBytes,
		SystemPrompt:                 c.SystemPrompt,
		SystemPromptFile:             c.SystemPromptFile,
		MaxWorkspacePromptBytes:      c.MaxWorkspacePromptBytes,
		MaxConcurrentTasks:           c.MaxConcurrentTasks,
		TaskOutputMaxBytes:           c.TaskOutputMaxBytes,
		RemoteExecutorShimCommand:    c.RemoteExecutorShimCommand,
		RemoteExecutorTimeoutSeconds: c.RemoteExecutorTimeoutSeconds,
	}
	raw, _ := json.Marshal(payload)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
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

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func visionDefaultURL(provider string) string {
	switch strings.TrimSpace(provider) {
	case "anthropic":
		return "https://api.anthropic.com"
	case "openai_compatible":
		return "https://api.openai.com/v1"
	default:
		return ""
	}
}

func providerModelFallback(modelProvider string, uc UserConfig) string {
	switch strings.TrimSpace(modelProvider) {
	case "openai_compatible":
		return firstNonEmpty(uc.OpenAI.Model, "gpt-4.1-mini")
	case "fake":
		return "fake-smoke"
	default:
		return firstNonEmpty(uc.Anthropic.Model, "claude-sonnet-4-5")
	}
}

func defaultProviderCacheProfile(modelProvider string, uc UserConfig, genericBaseURL string) string {
	return firstNonEmpty(
		uc.ProviderCacheProfile,
		inferProviderCacheProfile(modelProvider, firstNonEmpty(genericBaseURL, uc.OpenAI.BaseURL)),
	)
}

func inferProviderCacheProfile(modelProvider, baseURL string) string {
	if strings.TrimSpace(modelProvider) != "openai_compatible" {
		return ""
	}

	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil {
		return ""
	}
	host := strings.ToLower(parsed.Hostname())
	path := strings.ToLower(parsed.Path)
	if strings.HasPrefix(host, "ark.") && strings.Contains(host, "volces.com") {
		return "ark_responses"
	}
	if strings.Contains(host, "volces.com") && strings.HasPrefix(path, "/api/") {
		return "ark_responses"
	}
	return ""
}
