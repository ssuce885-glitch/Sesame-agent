package config

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
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

type ModelProviderConfig struct {
	ID        string
	APIFamily string
	BaseURL   string
	APIKeyEnv string
	ProfileID string
	OrgID     string
	ProjectID string
}

type ProfileConfig struct {
	ID            string
	Model         string
	ModelProvider string
	Reasoning     string
	Verbosity     string
	CacheProfile  string
}

type Config struct {
	Addr           string
	DataDir        string
	ActiveProfile  string
	ModelProviders map[string]ModelProviderConfig
	Profiles       map[string]ProfileConfig

	ModelProvider        string
	Model                string
	AnthropicAPIKey      string
	AnthropicBaseURL     string
	OpenAIAPIKey         string
	OpenAIBaseURL        string
	ProviderCacheProfile string

	CacheExpirySeconds           int
	MicrocompactBytesThreshold   int
	LogLevel                     string
	PermissionProfile            string
	MaxToolSteps                 int
	MaxShellOutputBytes          int
	ShellTimeoutSeconds          int
	MaxFileWriteBytes            int
	MaxRecentItems               int
	CompactionThreshold          int
	MaxEstimatedTokens           int
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

var (
	ErrLegacyConfigFieldsUnsupported = errors.New("legacy config fields are no longer supported")
	ErrActiveProfileRequired         = errors.New("active_profile is required")
	ErrActiveProfileNotFound         = errors.New("active_profile not found")
	ErrUnknownModelProvider          = errors.New("unknown model_provider")
	ErrUnsupportedAPIFamily          = errors.New("unsupported api_family")
)

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
	if strings.TrimSpace(cfg.ActiveProfile) == "" {
		missing = append(missing, "active_profile")
		return missing
	}
	if len(cfg.Profiles) == 0 {
		missing = append(missing, "profiles")
		return missing
	}
	profile, ok := cfg.Profiles[strings.TrimSpace(cfg.ActiveProfile)]
	if !ok {
		missing = append(missing, "active_profile")
		return missing
	}
	if strings.TrimSpace(profile.Model) == "" {
		missing = append(missing, "profiles.<active>.model")
	}
	if strings.TrimSpace(profile.ModelProvider) == "" {
		missing = append(missing, "profiles.<active>.model_provider")
		return missing
	}
	provider, ok := cfg.ModelProviders[strings.TrimSpace(profile.ModelProvider)]
	if !ok {
		missing = append(missing, "profiles.<active>.model_provider")
		return missing
	}
	if strings.TrimSpace(provider.APIFamily) == "" {
		missing = append(missing, "model_providers.<active>.api_family")
	}
	if runtimeModelProviderFromAPIFamily(provider.APIFamily) == "fake" {
		return missing
	}
	if strings.TrimSpace(provider.APIKeyEnv) == "" {
		missing = append(missing, "model_providers.<active>.api_key_env")
	}
	if strings.TrimSpace(provider.BaseURL) == "" {
		missing = append(missing, "model_providers.<active>.base_url")
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
	if usesLegacyModelConfig(uc) {
		return Config{}, fmt.Errorf("%w; rewrite config.json using model_providers, profiles, and active_profile", ErrLegacyConfigFieldsUnsupported)
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

	activeProfile := strings.TrimSpace(uc.ActiveProfile)
	if activeProfile == "" {
		return Config{}, ErrActiveProfileRequired
	}

	modelProviders := buildModelProviders(uc.ModelProviders)
	profiles := buildProfiles(uc.Profiles)
	activeRuntimeProfile, ok := profiles[activeProfile]
	if !ok {
		return Config{}, fmt.Errorf("%w: %q", ErrActiveProfileNotFound, activeProfile)
	}
	activeRuntimeProvider, ok := modelProviders[activeRuntimeProfile.ModelProvider]
	if !ok {
		return Config{}, fmt.Errorf("%w: active_profile %q references %q", ErrUnknownModelProvider, activeProfile, activeRuntimeProfile.ModelProvider)
	}
	for profileID, profile := range profiles {
		if _, ok := modelProviders[profile.ModelProvider]; !ok {
			return Config{}, fmt.Errorf("%w: profile %q references %q", ErrUnknownModelProvider, profileID, profile.ModelProvider)
		}
	}
	for providerID, provider := range modelProviders {
		if !isSupportedAPIFamily(provider.APIFamily) {
			return Config{}, fmt.Errorf("%w: model_provider %q uses %q", ErrUnsupportedAPIFamily, providerID, provider.APIFamily)
		}
	}

	compatProvider := runtimeModelProviderFromAPIFamily(activeRuntimeProvider.APIFamily)
	providerAPIKey := envOrDefault(strings.TrimSpace(activeRuntimeProvider.APIKeyEnv), "")
	anthropicAPIKey := ""
	anthropicBaseURL := ""
	openAIAPIKey := ""
	openAIBaseURL := ""
	switch compatProvider {
	case "anthropic":
		anthropicAPIKey = providerAPIKey
		anthropicBaseURL = strings.TrimSpace(activeRuntimeProvider.BaseURL)
	case "openai_compatible":
		openAIAPIKey = providerAPIKey
		openAIBaseURL = strings.TrimSpace(activeRuntimeProvider.BaseURL)
	}

	cfg := Config{
		Addr:                         firstNonEmpty(strings.TrimSpace(overrides.Addr), envOrDefaultWithFallback("SESAME_ADDR", uc.Listen.Addr, "127.0.0.1:4317")),
		DataDir:                      paths.DataDir,
		ActiveProfile:                activeProfile,
		ModelProviders:               modelProviders,
		Profiles:                     profiles,
		ModelProvider:                compatProvider,
		Model:                        firstNonEmpty(strings.TrimSpace(overrides.Model), activeRuntimeProfile.Model),
		AnthropicAPIKey:              anthropicAPIKey,
		AnthropicBaseURL:             anthropicBaseURL,
		OpenAIAPIKey:                 openAIAPIKey,
		OpenAIBaseURL:                openAIBaseURL,
		ProviderCacheProfile:         firstNonEmpty(envOrDefault("SESAME_PROVIDER_CACHE_PROFILE", ""), activeRuntimeProfile.CacheProfile, "none"),
		CacheExpirySeconds:           intEnvOrDefault("SESAME_CACHE_EXPIRY_SECONDS", 86400),
		MicrocompactBytesThreshold:   intEnvOrDefaultWithFallback("SESAME_MICROCOMPACT_BYTES_THRESHOLD", uc.MicrocompactBytesThreshold, 8192),
		LogLevel:                     envOrDefault("SESAME_LOG_LEVEL", "info"),
		PermissionProfile:            firstNonEmpty(strings.TrimSpace(overrides.PermissionMode), envOrDefaultWithFallback("SESAME_PERMISSION_PROFILE", uc.PermissionProfile, "read_only")),
		MaxToolSteps:                 intEnvOrDefaultWithFallback("SESAME_MAX_TOOL_STEPS", uc.MaxToolSteps, 8),
		MaxShellOutputBytes:          intEnvOrDefault("SESAME_MAX_SHELL_OUTPUT_BYTES", 4096),
		ShellTimeoutSeconds:          intEnvOrDefault("SESAME_SHELL_TIMEOUT_SECONDS", 30),
		MaxFileWriteBytes:            intEnvOrDefault("SESAME_MAX_FILE_WRITE_BYTES", 1<<20),
		MaxRecentItems:               intEnvOrDefaultWithFallback("SESAME_MAX_RECENT_ITEMS", uc.MaxRecentItems, 12),
		CompactionThreshold:          intEnvOrDefaultWithFallback("SESAME_COMPACTION_THRESHOLD", uc.CompactionThreshold, 32),
		MaxEstimatedTokens:           intEnvOrDefaultWithFallback("SESAME_MAX_ESTIMATED_TOKENS", uc.MaxEstimatedTokens, 16000),
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
		Addr                         string                         `json:"addr"`
		DataDir                      string                         `json:"data_dir"`
		ActiveProfile                string                         `json:"active_profile"`
		ModelProviders               map[string]ModelProviderConfig `json:"model_providers"`
		Profiles                     map[string]ProfileConfig       `json:"profiles"`
		ModelProvider                string                         `json:"provider"`
		Model                        string                         `json:"model"`
		AnthropicAPIKey              string                         `json:"anthropic_api_key"`
		AnthropicBaseURL             string                         `json:"anthropic_base_url"`
		OpenAIAPIKey                 string                         `json:"openai_api_key"`
		OpenAIBaseURL                string                         `json:"openai_base_url"`
		ProviderCacheProfile         string                         `json:"provider_cache_profile"`
		LogLevel                     string                         `json:"log_level"`
		PermissionProfile            string                         `json:"permission_profile"`
		MaxToolSteps                 int                            `json:"max_tool_steps"`
		MaxShellOutputBytes          int                            `json:"max_shell_output_bytes"`
		ShellTimeoutSeconds          int                            `json:"shell_timeout_seconds"`
		MaxFileWriteBytes            int                            `json:"max_file_write_bytes"`
		MaxRecentItems               int                            `json:"max_recent_items"`
		CompactionThreshold          int                            `json:"compaction_threshold"`
		MaxEstimatedTokens           int                            `json:"max_estimated_tokens"`
		MaxCompactionPasses          int                            `json:"max_compaction_passes"`
		SystemPrompt                 string                         `json:"system_prompt"`
		SystemPromptFile             string                         `json:"system_prompt_file"`
		MaxWorkspacePromptBytes      int                            `json:"max_workspace_prompt_bytes"`
		MaxConcurrentTasks           int                            `json:"max_concurrent_tasks"`
		TaskOutputMaxBytes           int                            `json:"task_output_max_bytes"`
		RemoteExecutorShimCommand    string                         `json:"remote_executor_shim_command"`
		RemoteExecutorTimeoutSeconds int                            `json:"remote_executor_timeout_seconds"`
	}{
		Addr:                         c.Addr,
		DataDir:                      c.DataDir,
		ActiveProfile:                c.ActiveProfile,
		ModelProviders:               c.ModelProviders,
		Profiles:                     c.Profiles,
		ModelProvider:                c.ModelProvider,
		Model:                        c.Model,
		AnthropicAPIKey:              c.AnthropicAPIKey,
		AnthropicBaseURL:             c.AnthropicBaseURL,
		OpenAIAPIKey:                 c.OpenAIAPIKey,
		OpenAIBaseURL:                c.OpenAIBaseURL,
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
		MaxCompactionPasses:          c.MaxCompactionPasses,
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

func buildModelProviders(userProviders map[string]UserConfigModelProvider) map[string]ModelProviderConfig {
	if len(userProviders) == 0 {
		return map[string]ModelProviderConfig{}
	}
	providers := make(map[string]ModelProviderConfig, len(userProviders))
	for id, provider := range userProviders {
		trimmedID := strings.TrimSpace(id)
		if trimmedID == "" {
			continue
		}
		providers[trimmedID] = ModelProviderConfig{
			ID:        trimmedID,
			APIFamily: strings.TrimSpace(provider.APIFamily),
			BaseURL:   strings.TrimSpace(provider.BaseURL),
			APIKeyEnv: strings.TrimSpace(provider.APIKeyEnv),
			ProfileID: strings.TrimSpace(provider.ProfileID),
			OrgID:     strings.TrimSpace(provider.OrgID),
			ProjectID: strings.TrimSpace(provider.ProjectID),
		}
	}
	return providers
}

func buildProfiles(userProfiles map[string]UserConfigProfile) map[string]ProfileConfig {
	if len(userProfiles) == 0 {
		return map[string]ProfileConfig{}
	}
	profiles := make(map[string]ProfileConfig, len(userProfiles))
	for id, profile := range userProfiles {
		trimmedID := strings.TrimSpace(id)
		if trimmedID == "" {
			continue
		}
		profiles[trimmedID] = ProfileConfig{
			ID:            trimmedID,
			Model:         strings.TrimSpace(profile.Model),
			ModelProvider: strings.TrimSpace(profile.ModelProvider),
			Reasoning:     strings.TrimSpace(profile.Reasoning),
			Verbosity:     strings.TrimSpace(profile.Verbosity),
			CacheProfile:  strings.TrimSpace(profile.CacheProfile),
		}
	}
	return profiles
}

func runtimeModelProviderFromAPIFamily(apiFamily string) string {
	normalized := strings.ToLower(strings.TrimSpace(apiFamily))
	switch normalized {
	case "anthropic", "anthropic_messages":
		return "anthropic"
	case "openai", "openai_compatible", "openai_chat_completions", "openai_responses":
		return "openai_compatible"
	case "fake":
		return "fake"
	}
	return ""
}

func isSupportedAPIFamily(apiFamily string) bool {
	return runtimeModelProviderFromAPIFamily(apiFamily) != ""
}

func usesLegacyModelConfig(uc UserConfig) bool {
	return uc.UsesLegacyModelConfig()
}
