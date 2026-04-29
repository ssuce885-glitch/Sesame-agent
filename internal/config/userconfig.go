package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type UserConfig struct {
	Listen                         UserConfigListen    `json:"listen"`
	Provider                       string              `json:"provider"`
	CompatMode                     string              `json:"compat_mode"`
	Model                          string              `json:"model"`
	QAModel                        string              `json:"qa_model,omitempty"`
	BaseURL                        string              `json:"base_url"`
	APIKey                         string              `json:"api_key"`
	DataDir                        string              `json:"data_dir"`
	PermissionProfile              string              `json:"permission_profile"`
	ProviderCacheProfile           string              `json:"provider_cache_profile"`
	SystemPrompt                   string              `json:"system_prompt"`
	SystemPromptFile               string              `json:"system_prompt_file"`
	Anthropic                      UserConfigAnthropic `json:"anthropic"`
	OpenAI                         UserConfigOpenAI    `json:"openai"`
	Vision                         UserConfigVision    `json:"vision"`
	Skills                         UserConfigSkills    `json:"skills"`
	Discord                        UserConfigDiscord   `json:"discord"`
	MaxToolSteps                   int                 `json:"max_tool_steps"`
	MaxRecentItems                 int                 `json:"max_recent_items"`
	CompactionThreshold            int                 `json:"compaction_threshold"`
	MaxEstimatedTokens             int                 `json:"max_estimated_tokens"`
	ModelContextWindow             int                 `json:"model_context_window"`
	MicrocompactBytesThreshold     int                 `json:"microcompact_bytes_threshold"`
	MaxCompactionPasses            int                 `json:"max_compaction_passes"`
	MaxCompactionBatchItems        int                 `json:"max_compaction_batch_items"`
	DefaultRoleBudget              RoleBudgetConfig    `json:"default_role_budget,omitempty"`
	ResetAnthropic                 bool                `json:"-"`
	ResetOpenAI                    bool                `json:"-"`
	ResetVision                    bool                `json:"-"`
	SetDiscordEnabled              bool                `json:"-"`
	SetDiscordMessageContentIntent bool                `json:"-"`
	SetDiscordLogIgnoredMessages   bool                `json:"-"`
	ClearDiscordBotToken           bool                `json:"-"`
	ClearDiscordBotTokenEnv        bool                `json:"-"`
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

type UserConfigVision struct {
	Provider string `json:"provider"`
	APIKey   string `json:"api_key"`
	BaseURL  string `json:"base_url"`
	Model    string `json:"model"`
}

type UserConfigSkills struct {
	Entries map[string]UserConfigSkillEntry `json:"entries"`
}

type UserConfigDiscord struct {
	Enabled              bool     `json:"enabled"`
	BotToken             string   `json:"bot_token"`
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

func MergeAndWriteUserConfig(patch UserConfig) error {
	paths, err := ResolvePaths("", "")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(paths.GlobalRoot, 0o755); err != nil {
		return err
	}

	existingRoot := map[string]json.RawMessage{}
	existingData, err := os.ReadFile(paths.GlobalConfigFile)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if err == nil {
		if err := json.Unmarshal(existingData, &existingRoot); err != nil {
			return fmt.Errorf("%s: %w", paths.GlobalConfigFile, err)
		}
	}

	patchRoot, err := userConfigPatchRoot(patch)
	if err != nil {
		return err
	}
	if len(patchRoot) == 0 {
		return nil
	}

	existingRoot, err = mergeRawJSONObjects(existingRoot, patchRoot)
	if err != nil {
		return err
	}

	out, err := json.MarshalIndent(existingRoot, "", "  ")
	if err != nil {
		return err
	}
	out = append(out, '\n')
	return os.WriteFile(paths.GlobalConfigFile, out, 0o600)
}

func userConfigPatchRoot(patch UserConfig) (map[string]json.RawMessage, error) {
	out := map[string]json.RawMessage{}

	if strings.TrimSpace(patch.Provider) != "" {
		if err := putJSONString(out, "provider", patch.Provider); err != nil {
			return nil, err
		}
	}
	if strings.TrimSpace(patch.Model) != "" {
		if err := putJSONString(out, "model", patch.Model); err != nil {
			return nil, err
		}
	}
	if strings.TrimSpace(patch.QAModel) != "" {
		if err := putJSONString(out, "qa_model", patch.QAModel); err != nil {
			return nil, err
		}
	}
	if strings.TrimSpace(patch.PermissionProfile) != "" {
		if err := putJSONString(out, "permission_profile", patch.PermissionProfile); err != nil {
			return nil, err
		}
	}
	if !isZeroRoleBudget(patch.DefaultRoleBudget) {
		if err := putJSONValue(out, "default_role_budget", patch.DefaultRoleBudget); err != nil {
			return nil, err
		}
	}

	listenPatch := map[string]json.RawMessage{}
	if strings.TrimSpace(patch.Listen.Addr) != "" {
		if err := putJSONString(listenPatch, "addr", patch.Listen.Addr); err != nil {
			return nil, err
		}
	}
	if len(listenPatch) > 0 {
		if err := putJSONObject(out, "listen", listenPatch); err != nil {
			return nil, err
		}
	}

	discordPatch := map[string]json.RawMessage{}
	if patch.SetDiscordEnabled || patch.Discord.Enabled {
		if err := putJSONBool(discordPatch, "enabled", patch.Discord.Enabled); err != nil {
			return nil, err
		}
	}
	if patch.ClearDiscordBotToken {
		if err := putJSONString(discordPatch, "bot_token", ""); err != nil {
			return nil, err
		}
	}
	if strings.TrimSpace(patch.Discord.BotToken) != "" {
		if err := putJSONString(discordPatch, "bot_token", patch.Discord.BotToken); err != nil {
			return nil, err
		}
	}
	if patch.ClearDiscordBotTokenEnv {
		if err := putJSONString(discordPatch, "bot_token_env", ""); err != nil {
			return nil, err
		}
	}
	if strings.TrimSpace(patch.Discord.BotTokenEnv) != "" {
		if err := putJSONString(discordPatch, "bot_token_env", patch.Discord.BotTokenEnv); err != nil {
			return nil, err
		}
	}
	if len(patch.Discord.GatewayIntents) > 0 {
		if err := putJSONStringArray(discordPatch, "gateway_intents", patch.Discord.GatewayIntents); err != nil {
			return nil, err
		}
	}
	if patch.SetDiscordMessageContentIntent || patch.Discord.MessageContentIntent {
		if err := putJSONBool(discordPatch, "message_content_intent", patch.Discord.MessageContentIntent); err != nil {
			return nil, err
		}
	}
	if patch.SetDiscordLogIgnoredMessages || patch.Discord.LogIgnoredMessages {
		if err := putJSONBool(discordPatch, "log_ignored_messages", patch.Discord.LogIgnoredMessages); err != nil {
			return nil, err
		}
	}
	if len(discordPatch) > 0 {
		if err := putJSONObject(out, "discord", discordPatch); err != nil {
			return nil, err
		}
	}

	openAIPatch := map[string]json.RawMessage{}
	if patch.ResetOpenAI {
		if err := putJSONString(openAIPatch, "api_key", ""); err != nil {
			return nil, err
		}
		if err := putJSONString(openAIPatch, "base_url", ""); err != nil {
			return nil, err
		}
		if err := putJSONString(openAIPatch, "model", ""); err != nil {
			return nil, err
		}
	}
	if strings.TrimSpace(patch.OpenAI.APIKey) != "" {
		if err := putJSONString(openAIPatch, "api_key", patch.OpenAI.APIKey); err != nil {
			return nil, err
		}
	}
	if strings.TrimSpace(patch.OpenAI.BaseURL) != "" {
		if err := putJSONString(openAIPatch, "base_url", patch.OpenAI.BaseURL); err != nil {
			return nil, err
		}
	}
	if strings.TrimSpace(patch.OpenAI.Model) != "" {
		if err := putJSONString(openAIPatch, "model", patch.OpenAI.Model); err != nil {
			return nil, err
		}
	}
	if len(openAIPatch) > 0 {
		if err := putJSONObject(out, "openai", openAIPatch); err != nil {
			return nil, err
		}
	}

	anthropicPatch := map[string]json.RawMessage{}
	if patch.ResetAnthropic {
		if err := putJSONString(anthropicPatch, "api_key", ""); err != nil {
			return nil, err
		}
		if err := putJSONString(anthropicPatch, "base_url", ""); err != nil {
			return nil, err
		}
		if err := putJSONString(anthropicPatch, "model", ""); err != nil {
			return nil, err
		}
	}
	if strings.TrimSpace(patch.Anthropic.APIKey) != "" {
		if err := putJSONString(anthropicPatch, "api_key", patch.Anthropic.APIKey); err != nil {
			return nil, err
		}
	}
	if strings.TrimSpace(patch.Anthropic.BaseURL) != "" {
		if err := putJSONString(anthropicPatch, "base_url", patch.Anthropic.BaseURL); err != nil {
			return nil, err
		}
	}
	if strings.TrimSpace(patch.Anthropic.Model) != "" {
		if err := putJSONString(anthropicPatch, "model", patch.Anthropic.Model); err != nil {
			return nil, err
		}
	}
	if len(anthropicPatch) > 0 {
		if err := putJSONObject(out, "anthropic", anthropicPatch); err != nil {
			return nil, err
		}
	}

	visionPatch := map[string]json.RawMessage{}
	if patch.ResetVision {
		if err := putJSONString(visionPatch, "provider", ""); err != nil {
			return nil, err
		}
		if err := putJSONString(visionPatch, "api_key", ""); err != nil {
			return nil, err
		}
		if err := putJSONString(visionPatch, "base_url", ""); err != nil {
			return nil, err
		}
		if err := putJSONString(visionPatch, "model", ""); err != nil {
			return nil, err
		}
	}
	if strings.TrimSpace(patch.Vision.Provider) != "" {
		if err := putJSONString(visionPatch, "provider", patch.Vision.Provider); err != nil {
			return nil, err
		}
	}
	if strings.TrimSpace(patch.Vision.APIKey) != "" {
		if err := putJSONString(visionPatch, "api_key", patch.Vision.APIKey); err != nil {
			return nil, err
		}
	}
	if strings.TrimSpace(patch.Vision.BaseURL) != "" {
		if err := putJSONString(visionPatch, "base_url", patch.Vision.BaseURL); err != nil {
			return nil, err
		}
	}
	if strings.TrimSpace(patch.Vision.Model) != "" {
		if err := putJSONString(visionPatch, "model", patch.Vision.Model); err != nil {
			return nil, err
		}
	}
	if len(visionPatch) > 0 {
		if err := putJSONObject(out, "vision", visionPatch); err != nil {
			return nil, err
		}
	}

	return out, nil
}

func putJSONString(dst map[string]json.RawMessage, key, value string) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	dst[key] = data
	return nil
}

func putJSONBool(dst map[string]json.RawMessage, key string, value bool) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	dst[key] = data
	return nil
}

func putJSONStringArray(dst map[string]json.RawMessage, key string, values []string) error {
	data, err := json.Marshal(values)
	if err != nil {
		return err
	}
	dst[key] = data
	return nil
}

func putJSONObject(dst map[string]json.RawMessage, key string, value map[string]json.RawMessage) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	dst[key] = data
	return nil
}

func putJSONValue(dst map[string]json.RawMessage, key string, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	dst[key] = data
	return nil
}

func isZeroRoleBudget(budget RoleBudgetConfig) bool {
	return strings.TrimSpace(budget.MaxRuntime) == "" &&
		budget.MaxToolCalls == 0 &&
		budget.MaxContextTokens == 0 &&
		budget.MaxCost == 0 &&
		budget.MaxTurnsPerHour == 0 &&
		budget.MaxConcurrent == 0
}

func mergeRawJSONObjects(existing, patch map[string]json.RawMessage) (map[string]json.RawMessage, error) {
	if existing == nil {
		existing = map[string]json.RawMessage{}
	}
	for key, patchValue := range patch {
		existingValue, ok := existing[key]
		if !ok {
			existing[key] = patchValue
			continue
		}

		mergedValue, merged, err := mergeRawJSONValue(existingValue, patchValue)
		if err != nil {
			return nil, err
		}
		if merged {
			existing[key] = mergedValue
			continue
		}
		existing[key] = patchValue
	}
	return existing, nil
}

func mergeRawJSONValue(existing, patch json.RawMessage) (json.RawMessage, bool, error) {
	var existingObj map[string]json.RawMessage
	if err := json.Unmarshal(existing, &existingObj); err != nil {
		return nil, false, nil
	}
	var patchObj map[string]json.RawMessage
	if err := json.Unmarshal(patch, &patchObj); err != nil {
		return nil, false, nil
	}
	mergedObj, err := mergeRawJSONObjects(existingObj, patchObj)
	if err != nil {
		return nil, false, err
	}
	merged, err := json.Marshal(mergedObj)
	if err != nil {
		return nil, false, err
	}
	return json.RawMessage(merged), true, nil
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
