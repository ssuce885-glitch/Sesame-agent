package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type UserConfig struct {
	Listen                     UserConfigListen    `json:"listen"`
	Provider                   string              `json:"provider"`
	CompatMode                 string              `json:"compat_mode"`
	Model                      string              `json:"model"`
	BaseURL                    string              `json:"base_url"`
	APIKey                     string              `json:"api_key"`
	DataDir                    string              `json:"data_dir"`
	PermissionProfile          string              `json:"permission_profile"`
	ProviderCacheProfile       string              `json:"provider_cache_profile"`
	SystemPrompt               string              `json:"system_prompt"`
	SystemPromptFile           string              `json:"system_prompt_file"`
	Anthropic                  UserConfigAnthropic `json:"anthropic"`
	OpenAI                     UserConfigOpenAI    `json:"openai"`
	Skills                     UserConfigSkills    `json:"skills"`
	Discord                    UserConfigDiscord   `json:"discord"`
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

type UserConfigSkills struct {
	Entries map[string]UserConfigSkillEntry `json:"entries"`
}

type UserConfigDiscord struct {
	Enabled              bool     `json:"enabled"`
	BotTokenEnv          string   `json:"bot_token_env"`
	GatewayIntents       []string `json:"gateway_intents"`
	MessageContentIntent bool     `json:"message_content_intent"`
	LogIgnoredMessages   bool     `json:"log_ignored_messages"`
}

type UserConfigSkillEntry struct {
	Enabled *bool             `json:"enabled,omitempty"`
	Env     map[string]string `json:"env"`
}

type CLIConfigFile struct {
	ShowExtensionsOnStartup *bool `json:"show_extensions_on_startup,omitempty"`
}

func loadUserConfig() (UserConfig, error) {
	paths, err := ResolvePaths("", "")
	if err != nil {
		return UserConfig{}, err
	}
	return loadUserConfigFromPath(paths.GlobalConfigFile)
}

func loadUserConfigFromPath(path string) (UserConfig, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return UserConfig{}, nil
	}
	if err != nil {
		return UserConfig{}, err
	}
	var cfg UserConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return UserConfig{}, fmt.Errorf("%s: %w", path, err)
	}
	return cfg, nil
}

func LoadUserConfig() (UserConfig, error) {
	return loadUserConfig()
}

func LoadUserConfigFromGlobalRoot(globalRoot string) (UserConfig, error) {
	globalRoot = strings.TrimSpace(globalRoot)
	if globalRoot == "" {
		return loadUserConfig()
	}
	return loadUserConfigFromPath(filepath.Join(globalRoot, "config.json"))
}

func MergedSkillEnv(globalRoot string, skillNames []string) (map[string]string, error) {
	if len(skillNames) == 0 {
		return nil, nil
	}
	cfg, err := LoadUserConfigFromGlobalRoot(globalRoot)
	if err != nil {
		return nil, err
	}
	if len(cfg.Skills.Entries) == 0 {
		return nil, nil
	}

	out := make(map[string]string)
	for _, skillName := range skillNames {
		entry, ok := lookupSkillEntry(cfg.Skills.Entries, skillName)
		if !ok {
			continue
		}
		if entry.Enabled != nil && !*entry.Enabled {
			continue
		}
		for key, value := range entry.Env {
			key = strings.TrimSpace(key)
			if key == "" {
				continue
			}
			out[key] = value
		}
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
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

func lookupSkillEntry(entries map[string]UserConfigSkillEntry, name string) (UserConfigSkillEntry, bool) {
	want := canonicalSkillConfigName(name)
	if want == "" {
		return UserConfigSkillEntry{}, false
	}
	for entryName, entry := range entries {
		if canonicalSkillConfigName(entryName) == want {
			return entry, true
		}
	}
	return UserConfigSkillEntry{}, false
}

func canonicalSkillConfigName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return ""
	}
	var builder strings.Builder
	builder.Grow(len(name))
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z':
			builder.WriteRune(r)
		case r >= '0' && r <= '9':
			builder.WriteRune(r)
		case r == '.':
			builder.WriteRune(r)
		}
	}
	return builder.String()
}
