package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type UserConfig struct {
	Listen                     UserConfigListen    `json:"listen"`
	Provider                   string              `json:"provider"`
	Model                      string              `json:"model"`
	DataDir                    string              `json:"data_dir"`
	PermissionProfile          string              `json:"permission_profile"`
	ProviderCacheProfile       string              `json:"provider_cache_profile"`
	SystemPrompt               string              `json:"system_prompt"`
	SystemPromptFile           string              `json:"system_prompt_file"`
	Anthropic                  UserConfigAnthropic `json:"anthropic"`
	OpenAI                     UserConfigOpenAI    `json:"openai"`
	MaxToolSteps               int                 `json:"max_tool_steps"`
	MaxRecentItems             int                 `json:"max_recent_items"`
	CompactionThreshold        int                 `json:"compaction_threshold"`
	MaxEstimatedTokens         int                 `json:"max_estimated_tokens"`
	MicrocompactBytesThreshold int                 `json:"microcompact_bytes_threshold"`
	MaxCompactionPasses        int                 `json:"max_compaction_passes"`
}

type UserConfigListen struct {
	Addr string `json:"addr"`
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
		Anthropic: UserConfigAnthropic{
			BaseURL: "https://api.anthropic.com",
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
