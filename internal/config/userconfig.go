package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type UserConfig struct {
	Listen                     UserConfigListen                   `json:"listen"`
	DataDir                    string                             `json:"data_dir"`
	PermissionProfile          string                             `json:"permission_profile"`
	SystemPrompt               string                             `json:"system_prompt"`
	SystemPromptFile           string                             `json:"system_prompt_file"`
	Skills                     UserConfigSkills                   `json:"skills"`
	ActiveProfile              string                             `json:"active_profile"`
	ModelProviders             map[string]UserConfigModelProvider `json:"model_providers"`
	Profiles                   map[string]UserConfigProfile       `json:"profiles"`
	MaxToolSteps               int                                `json:"max_tool_steps"`
	MaxRecentItems             int                                `json:"max_recent_items"`
	CompactionThreshold        int                                `json:"compaction_threshold"`
	MaxEstimatedTokens         int                                `json:"max_estimated_tokens"`
	MicrocompactBytesThreshold int                                `json:"microcompact_bytes_threshold"`
	MaxCompactionPasses        int                                `json:"max_compaction_passes"`

	Provider             string              `json:"-"`
	Model                string              `json:"-"`
	ProviderCacheProfile string              `json:"-"`
	Anthropic            UserConfigAnthropic `json:"-"`
	OpenAI               UserConfigOpenAI    `json:"-"`

	hasLegacyModelConfig bool
}

type UserConfigListen struct {
	Addr string `json:"addr"`
}

type UserConfigSkills struct {
	Enabled []string `json:"enabled"`
}

type UserConfigModelProvider struct {
	APIFamily string `json:"api_family"`
	BaseURL   string `json:"base_url"`
	APIKeyEnv string `json:"api_key_env"`
	ProfileID string `json:"provider_profile,omitempty"`
	OrgID     string `json:"org_id,omitempty"`
	ProjectID string `json:"project_id,omitempty"`
}

type UserConfigAnthropic struct {
	APIKey  string `json:"api_key"`
	BaseURL string `json:"base_url"`
	Model   string `json:"model"`
}

type UserConfigOpenAI struct {
	APIKey  string `json:"api_key"`
	BaseURL string `json:"base_url"`
	Model   string `json:"model"`
}

type UserConfigProfile struct {
	Model         string `json:"model"`
	ModelProvider string `json:"model_provider"`
	Reasoning     string `json:"reasoning,omitempty"`
	Verbosity     string `json:"verbosity,omitempty"`
	CacheProfile  string `json:"cache_profile,omitempty"`
}

type CLIConfigFile struct {
	ShowExtensionsOnStartup *bool `json:"show_extensions_on_startup,omitempty"`
}

func loadUserConfig() (UserConfig, error) {
	paths, err := ResolvePaths("", "")
	if err != nil {
		return UserConfig{}, err
	}
	data, err := os.ReadFile(paths.GlobalConfigFile)
	if os.IsNotExist(err) {
		return UserConfig{}, nil
	}
	if err != nil {
		return UserConfig{}, err
	}
	var cfg UserConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return UserConfig{}, fmt.Errorf("%s: %w", paths.GlobalConfigFile, err)
	}
	return cfg, nil
}

func LoadUserConfig() (UserConfig, error) {
	return loadUserConfig()
}

func WriteUserConfig(cfg UserConfig) error {
	paths, err := ResolvePaths("", "")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(paths.GlobalRoot, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(paths.GlobalConfigFile, data, 0o600)
}

func EnsureUserConfigFile() (string, bool, error) {
	paths, err := ResolvePaths("", "")
	if err != nil {
		return "", false, err
	}
	if err := os.MkdirAll(paths.GlobalRoot, 0o755); err != nil {
		return "", false, err
	}
	if _, err := os.Stat(paths.GlobalConfigFile); err == nil {
		return paths.GlobalConfigFile, false, nil
	} else if !os.IsNotExist(err) {
		return "", false, err
	}
	template := UserConfig{
		PermissionProfile: "trusted_local",
		ActiveProfile:     "default",
		ModelProviders: map[string]UserConfigModelProvider{
			"anthropic": {
				APIFamily: "anthropic_messages",
				BaseURL:   "https://api.anthropic.com",
				APIKeyEnv: "ANTHROPIC_API_KEY",
			},
		},
		Profiles: map[string]UserConfigProfile{
			"default": {
				Model:         "claude-sonnet-4-5",
				ModelProvider: "anthropic",
				CacheProfile:  "anthropic_default",
			},
		},
	}
	if err := WriteUserConfig(template); err != nil {
		return "", false, err
	}
	return paths.GlobalConfigFile, true, nil
}

func loadCLIConfigFile() (CLIConfigFile, error) {
	paths, err := ResolvePaths("", "")
	if err != nil {
		return CLIConfigFile{}, err
	}
	data, err := os.ReadFile(paths.GlobalCLIConfigFile)
	if os.IsNotExist(err) {
		return CLIConfigFile{}, nil
	}
	if err != nil {
		return CLIConfigFile{}, err
	}
	var cfg CLIConfigFile
	if err := json.Unmarshal(data, &cfg); err != nil {
		return CLIConfigFile{}, fmt.Errorf("%s: %w", paths.GlobalCLIConfigFile, err)
	}
	return cfg, nil
}

func WriteCLIConfigFile(cfg CLIConfigFile) error {
	paths, err := ResolvePaths("", "")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(paths.GlobalRoot, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(paths.GlobalCLIConfigFile, data, 0o644)
}

func EnsureCLIConfigFile() (string, bool, error) {
	paths, err := ResolvePaths("", "")
	if err != nil {
		return "", false, err
	}
	if err := os.MkdirAll(paths.GlobalRoot, 0o755); err != nil {
		return "", false, err
	}
	if _, err := os.Stat(paths.GlobalCLIConfigFile); err == nil {
		return paths.GlobalCLIConfigFile, false, nil
	} else if !os.IsNotExist(err) {
		return "", false, err
	}
	enabled := true
	if err := WriteCLIConfigFile(CLIConfigFile{ShowExtensionsOnStartup: &enabled}); err != nil {
		return "", false, err
	}
	return paths.GlobalCLIConfigFile, true, nil
}

func GlobalConfigPath() (string, error) {
	paths, err := ResolvePaths("", "")
	if err != nil {
		return "", err
	}
	return paths.GlobalConfigFile, nil
}

func GlobalCLIConfigPath() (string, error) {
	paths, err := ResolvePaths("", "")
	if err != nil {
		return "", err
	}
	return paths.GlobalCLIConfigFile, nil
}

func pathJoin(root string, elems ...string) string {
	parts := make([]string, 0, len(elems)+1)
	parts = append(parts, root)
	parts = append(parts, elems...)
	return filepath.Join(parts...)
}

func (c *UserConfig) UnmarshalJSON(data []byte) error {
	type alias UserConfig
	var out alias
	if err := json.Unmarshal(data, &out); err != nil {
		return err
	}
	*c = UserConfig(out)

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	c.hasLegacyModelConfig = hasAnyNonNullKey(raw,
		"provider",
		"model",
		"base_url",
		"api_key",
		"compat_mode",
		"provider_cache_profile",
		"anthropic",
		"openai",
	)
	return nil
}

func (c UserConfig) UsesLegacyModelConfig() bool {
	return c.hasLegacyModelConfig
}

func hasAnyNonNullKey(raw map[string]json.RawMessage, keys ...string) bool {
	for _, key := range keys {
		value, ok := raw[key]
		if !ok {
			continue
		}
		if strings.TrimSpace(string(value)) == "null" {
			continue
		}
		return true
	}
	return false
}
