package config

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
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
	Addr              string
	DataDir           string
	ActiveProfile     string
	ModelProviders    map[string]ModelProviderConfig
	Profiles          map[string]ProfileConfig
	PermissionProfile string
	SystemPrompt      string
	SystemPromptFile  string
	Paths             Paths
	ConfigFingerprint string
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
		return Config{}, fmt.Errorf("legacy config fields are no longer supported; rewrite config.json using model_providers, profiles, and active_profile")
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
		return Config{}, fmt.Errorf("active_profile is required")
	}

	cfg := Config{
		Addr:              firstNonEmpty(strings.TrimSpace(overrides.Addr), envOrDefaultWithFallback("SESAME_ADDR", uc.Listen.Addr, "127.0.0.1:4317")),
		DataDir:           paths.DataDir,
		ActiveProfile:     activeProfile,
		ModelProviders:    buildModelProviders(uc.ModelProviders),
		Profiles:          buildProfiles(uc.Profiles, strings.TrimSpace(overrides.Model)),
		PermissionProfile: firstNonEmpty(strings.TrimSpace(overrides.PermissionMode), envOrDefaultWithFallback("SESAME_PERMISSION_PROFILE", uc.PermissionProfile, "read_only")),
		SystemPrompt:      envOrDefaultWithFallback("SESAME_SYSTEM_PROMPT", uc.SystemPrompt, ""),
		SystemPromptFile:  envOrDefaultWithFallback("SESAME_SYSTEM_PROMPT_FILE", uc.SystemPromptFile, ""),
		Paths:             paths,
	}
	if _, ok := cfg.Profiles[cfg.ActiveProfile]; !ok {
		return Config{}, fmt.Errorf("active_profile %q not found", cfg.ActiveProfile)
	}
	for profileID, profile := range cfg.Profiles {
		if _, ok := cfg.ModelProviders[profile.ModelProvider]; !ok {
			return Config{}, fmt.Errorf("profile %q references unknown model_provider %q", profileID, profile.ModelProvider)
		}
	}
	cfg.ConfigFingerprint = cfg.Fingerprint()
	return cfg, nil
}

func (c Config) Fingerprint() string {
	payload := struct {
		Addr              string                         `json:"addr"`
		DataDir           string                         `json:"data_dir"`
		ActiveProfile     string                         `json:"active_profile"`
		ModelProviders    map[string]ModelProviderConfig `json:"model_providers"`
		Profiles          map[string]ProfileConfig       `json:"profiles"`
		PermissionProfile string                         `json:"permission_profile"`
		SystemPrompt      string                         `json:"system_prompt"`
		SystemPromptFile  string                         `json:"system_prompt_file"`
	}{
		Addr:              c.Addr,
		DataDir:           c.DataDir,
		ActiveProfile:     c.ActiveProfile,
		ModelProviders:    c.ModelProviders,
		Profiles:          c.Profiles,
		PermissionProfile: c.PermissionProfile,
		SystemPrompt:      c.SystemPrompt,
		SystemPromptFile:  c.SystemPromptFile,
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

func buildProfiles(userProfiles map[string]UserConfigProfile, modelOverride string) map[string]ProfileConfig {
	if len(userProfiles) == 0 {
		return map[string]ProfileConfig{}
	}
	profiles := make(map[string]ProfileConfig, len(userProfiles))
	override := strings.TrimSpace(modelOverride)
	for id, profile := range userProfiles {
		trimmedID := strings.TrimSpace(id)
		if trimmedID == "" {
			continue
		}
		model := strings.TrimSpace(profile.Model)
		if override != "" {
			model = override
		}
		profiles[trimmedID] = ProfileConfig{
			ID:            trimmedID,
			Model:         model,
			ModelProvider: strings.TrimSpace(profile.ModelProvider),
			Reasoning:     strings.TrimSpace(profile.Reasoning),
			Verbosity:     strings.TrimSpace(profile.Verbosity),
			CacheProfile:  strings.TrimSpace(profile.CacheProfile),
		}
	}
	return profiles
}

func usesLegacyModelConfig(uc UserConfig) bool {
	return uc.UsesLegacyModelConfig()
}
