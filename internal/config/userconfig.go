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

	patchRoot := userConfigPatchRoot(patch)
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

func userConfigPatchRoot(patch UserConfig) map[string]json.RawMessage {
	out := map[string]json.RawMessage{}

	if strings.TrimSpace(patch.Provider) != "" {
		putJSONString(out, "provider", patch.Provider)
	}
	if strings.TrimSpace(patch.Model) != "" {
		putJSONString(out, "model", patch.Model)
	}
	if strings.TrimSpace(patch.QAModel) != "" {
		putJSONString(out, "qa_model", patch.QAModel)
	}
	if strings.TrimSpace(patch.PermissionProfile) != "" {
		putJSONString(out, "permission_profile", patch.PermissionProfile)
	}
	if !isZeroRoleBudget(patch.DefaultRoleBudget) {
		putJSONValue(out, "default_role_budget", patch.DefaultRoleBudget)
	}

	listenPatch := map[string]json.RawMessage{}
	if strings.TrimSpace(patch.Listen.Addr) != "" {
		putJSONString(listenPatch, "addr", patch.Listen.Addr)
	}
	if len(listenPatch) > 0 {
		putJSONObject(out, "listen", listenPatch)
	}

	discordPatch := map[string]json.RawMessage{}
	if patch.SetDiscordEnabled || patch.Discord.Enabled {
		putJSONBool(discordPatch, "enabled", patch.Discord.Enabled)
	}
	if patch.ClearDiscordBotToken {
		putJSONString(discordPatch, "bot_token", "")
	}
	if strings.TrimSpace(patch.Discord.BotToken) != "" {
		putJSONString(discordPatch, "bot_token", patch.Discord.BotToken)
	}
	if patch.ClearDiscordBotTokenEnv {
		putJSONString(discordPatch, "bot_token_env", "")
	}
	if strings.TrimSpace(patch.Discord.BotTokenEnv) != "" {
		putJSONString(discordPatch, "bot_token_env", patch.Discord.BotTokenEnv)
	}
	if len(patch.Discord.GatewayIntents) > 0 {
		putJSONStringArray(discordPatch, "gateway_intents", patch.Discord.GatewayIntents)
	}
	if patch.SetDiscordMessageContentIntent || patch.Discord.MessageContentIntent {
		putJSONBool(discordPatch, "message_content_intent", patch.Discord.MessageContentIntent)
	}
	if patch.SetDiscordLogIgnoredMessages || patch.Discord.LogIgnoredMessages {
		putJSONBool(discordPatch, "log_ignored_messages", patch.Discord.LogIgnoredMessages)
	}
	if len(discordPatch) > 0 {
		putJSONObject(out, "discord", discordPatch)
	}

	openAIPatch := map[string]json.RawMessage{}
	if patch.ResetOpenAI {
		putJSONString(openAIPatch, "api_key", "")
		putJSONString(openAIPatch, "base_url", "")
		putJSONString(openAIPatch, "model", "")
	}
	if strings.TrimSpace(patch.OpenAI.APIKey) != "" {
		putJSONString(openAIPatch, "api_key", patch.OpenAI.APIKey)
	}
	if strings.TrimSpace(patch.OpenAI.BaseURL) != "" {
		putJSONString(openAIPatch, "base_url", patch.OpenAI.BaseURL)
	}
	if strings.TrimSpace(patch.OpenAI.Model) != "" {
		putJSONString(openAIPatch, "model", patch.OpenAI.Model)
	}
	if len(openAIPatch) > 0 {
		putJSONObject(out, "openai", openAIPatch)
	}

	anthropicPatch := map[string]json.RawMessage{}
	if patch.ResetAnthropic {
		putJSONString(anthropicPatch, "api_key", "")
		putJSONString(anthropicPatch, "base_url", "")
		putJSONString(anthropicPatch, "model", "")
	}
	if strings.TrimSpace(patch.Anthropic.APIKey) != "" {
		putJSONString(anthropicPatch, "api_key", patch.Anthropic.APIKey)
	}
	if strings.TrimSpace(patch.Anthropic.BaseURL) != "" {
		putJSONString(anthropicPatch, "base_url", patch.Anthropic.BaseURL)
	}
	if strings.TrimSpace(patch.Anthropic.Model) != "" {
		putJSONString(anthropicPatch, "model", patch.Anthropic.Model)
	}
	if len(anthropicPatch) > 0 {
		putJSONObject(out, "anthropic", anthropicPatch)
	}

	visionPatch := map[string]json.RawMessage{}
	if patch.ResetVision {
		putJSONString(visionPatch, "provider", "")
		putJSONString(visionPatch, "api_key", "")
		putJSONString(visionPatch, "base_url", "")
		putJSONString(visionPatch, "model", "")
	}
	if strings.TrimSpace(patch.Vision.Provider) != "" {
		putJSONString(visionPatch, "provider", patch.Vision.Provider)
	}
	if strings.TrimSpace(patch.Vision.APIKey) != "" {
		putJSONString(visionPatch, "api_key", patch.Vision.APIKey)
	}
	if strings.TrimSpace(patch.Vision.BaseURL) != "" {
		putJSONString(visionPatch, "base_url", patch.Vision.BaseURL)
	}
	if strings.TrimSpace(patch.Vision.Model) != "" {
		putJSONString(visionPatch, "model", patch.Vision.Model)
	}
	if len(visionPatch) > 0 {
		putJSONObject(out, "vision", visionPatch)
	}

	return out
}

func putJSONString(dst map[string]json.RawMessage, key, value string) {
	data, _ := json.Marshal(value)
	dst[key] = data
}

func putJSONBool(dst map[string]json.RawMessage, key string, value bool) {
	data, _ := json.Marshal(value)
	dst[key] = data
}

func putJSONStringArray(dst map[string]json.RawMessage, key string, values []string) {
	data, _ := json.Marshal(values)
	dst[key] = data
}

func putJSONObject(dst map[string]json.RawMessage, key string, value map[string]json.RawMessage) {
	data, _ := json.Marshal(value)
	dst[key] = data
}

func putJSONValue(dst map[string]json.RawMessage, key string, value any) {
	data, _ := json.Marshal(value)
	dst[key] = data
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
