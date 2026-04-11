package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolvePathsUsesSesameRoots(t *testing.T) {
	home := t.TempDir()
	workspace := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("SESAME_DATA_DIR", "")

	paths, err := ResolvePaths(workspace, "")
	if err != nil {
		t.Fatalf("ResolvePaths() error = %v", err)
	}
	if paths.GlobalRoot != filepath.Join(home, ".sesame") {
		t.Fatalf("GlobalRoot = %q, want %q", paths.GlobalRoot, filepath.Join(home, ".sesame"))
	}
	if paths.DataDir != filepath.Join(home, ".sesame") {
		t.Fatalf("DataDir = %q, want %q", paths.DataDir, filepath.Join(home, ".sesame"))
	}
	if paths.WorkspacePromptFile != filepath.Join(workspace, ".sesame", "prompt.md") {
		t.Fatalf("WorkspacePromptFile = %q", paths.WorkspacePromptFile)
	}
	if paths.WorkspaceSkillsDir != filepath.Join(workspace, ".sesame", "skills") {
		t.Fatalf("WorkspaceSkillsDir = %q", paths.WorkspaceSkillsDir)
	}
}

func TestLoadUserConfigReadsSesameConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	content := `{"provider":"openai_compatible","model":"gpt-5.4","permission_profile":"trusted_local","max_recent_items":14,"compaction_threshold":40,"max_estimated_tokens":20000,"microcompact_bytes_threshold":16384,"max_compaction_passes":2,"openai":{"api_key":"sk-test","base_url":"https://example.com/v1"}}`
	writeConfigFile(t, filepath.Join(home, ".sesame", "config.json"), content)

	cfg, err := LoadUserConfig()
	if err != nil {
		t.Fatalf("LoadUserConfig() error = %v", err)
	}
	if cfg.Provider != "openai_compatible" {
		t.Fatalf("Provider = %q, want openai_compatible", cfg.Provider)
	}
	if cfg.Model != "gpt-5.4" {
		t.Fatalf("Model = %q, want gpt-5.4", cfg.Model)
	}
	if cfg.OpenAI.APIKey != "sk-test" {
		t.Fatalf("OpenAI.APIKey = %q, want sk-test", cfg.OpenAI.APIKey)
	}
	if cfg.MaxRecentItems != 14 {
		t.Fatalf("MaxRecentItems = %d, want 14", cfg.MaxRecentItems)
	}
	if cfg.CompactionThreshold != 40 {
		t.Fatalf("CompactionThreshold = %d, want 40", cfg.CompactionThreshold)
	}
	if cfg.MaxEstimatedTokens != 20000 {
		t.Fatalf("MaxEstimatedTokens = %d, want 20000", cfg.MaxEstimatedTokens)
	}
	if cfg.MicrocompactBytesThreshold != 16384 {
		t.Fatalf("MicrocompactBytesThreshold = %d, want 16384", cfg.MicrocompactBytesThreshold)
	}
	if cfg.MaxCompactionPasses != 2 {
		t.Fatalf("MaxCompactionPasses = %d, want 2", cfg.MaxCompactionPasses)
	}
}

func TestMergedSkillEnvLoadsEnabledSkillEntries(t *testing.T) {
	globalRoot := t.TempDir()
	writeConfigFile(t, filepath.Join(globalRoot, "config.json"), `{
		"skills": {
			"entries": {
				"send-email": {
					"enabled": true,
					"env": {
						"EMAIL_SMTP_SERVER": "smtp.example.com",
						"EMAIL_SENDER": "bot@example.com"
					}
				},
				"disabled-skill": {
					"enabled": false,
					"env": {
						"SHOULD_NOT_LOAD": "1"
					}
				}
			}
		}
	}`)

	env, err := MergedSkillEnv(globalRoot, []string{"send_email", "disabled-skill"})
	if err != nil {
		t.Fatalf("MergedSkillEnv() error = %v", err)
	}
	if env["EMAIL_SMTP_SERVER"] != "smtp.example.com" {
		t.Fatalf("EMAIL_SMTP_SERVER = %q, want smtp.example.com", env["EMAIL_SMTP_SERVER"])
	}
	if env["EMAIL_SENDER"] != "bot@example.com" {
		t.Fatalf("EMAIL_SENDER = %q, want bot@example.com", env["EMAIL_SENDER"])
	}
	if _, ok := env["SHOULD_NOT_LOAD"]; ok {
		t.Fatalf("env unexpectedly included disabled skill values: %v", env)
	}
}

func TestResolveCLIStartupConfigUsesSesameEnvAndDefaults(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("SESAME_ADDR", "")
	t.Setenv("SESAME_DATA_DIR", "")
	t.Setenv("SESAME_MODEL_PROVIDER", "")
	t.Setenv("SESAME_MODEL", "")
	t.Setenv("SESAME_PERMISSION_PROFILE", "")

	cfg, err := ResolveCLIStartupConfig(CLIStartupOverrides{})
	if err != nil {
		t.Fatalf("ResolveCLIStartupConfig() error = %v", err)
	}
	if cfg.DataDir != filepath.Join(home, ".sesame") {
		t.Fatalf("DataDir = %q, want %q", cfg.DataDir, filepath.Join(home, ".sesame"))
	}
	if cfg.ModelProvider != "anthropic" {
		t.Fatalf("ModelProvider = %q, want anthropic", cfg.ModelProvider)
	}
	if cfg.Model != "claude-sonnet-4-5" {
		t.Fatalf("Model = %q, want claude-sonnet-4-5", cfg.Model)
	}
	if cfg.PermissionProfile != "read_only" {
		t.Fatalf("PermissionProfile = %q, want read_only", cfg.PermissionProfile)
	}
	if cfg.MaxRecentItems != 12 {
		t.Fatalf("MaxRecentItems = %d, want 12", cfg.MaxRecentItems)
	}
	if cfg.CompactionThreshold != 32 {
		t.Fatalf("CompactionThreshold = %d, want 32", cfg.CompactionThreshold)
	}
	if cfg.MaxEstimatedTokens != 16000 {
		t.Fatalf("MaxEstimatedTokens = %d, want 16000", cfg.MaxEstimatedTokens)
	}
	if cfg.MicrocompactBytesThreshold != 8192 {
		t.Fatalf("MicrocompactBytesThreshold = %d, want 8192", cfg.MicrocompactBytesThreshold)
	}
	if cfg.MaxCompactionPasses != 1 {
		t.Fatalf("MaxCompactionPasses = %d, want 1", cfg.MaxCompactionPasses)
	}
}

func TestResolveCLIStartupConfigUsesUserConfigContextThresholds(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("SESAME_MAX_RECENT_ITEMS", "")
	t.Setenv("SESAME_COMPACTION_THRESHOLD", "")
	t.Setenv("SESAME_MAX_ESTIMATED_TOKENS", "")
	t.Setenv("SESAME_MICROCOMPACT_BYTES_THRESHOLD", "")
	t.Setenv("SESAME_MAX_COMPACTION_PASSES", "")

	content := `{
		"max_recent_items": 24,
		"compaction_threshold": 64,
		"max_estimated_tokens": 48000,
		"microcompact_bytes_threshold": 12288,
		"max_compaction_passes": 3
	}`
	writeConfigFile(t, filepath.Join(home, ".sesame", "config.json"), content)

	cfg, err := ResolveCLIStartupConfig(CLIStartupOverrides{})
	if err != nil {
		t.Fatalf("ResolveCLIStartupConfig() error = %v", err)
	}
	if cfg.MaxRecentItems != 24 {
		t.Fatalf("MaxRecentItems = %d, want 24", cfg.MaxRecentItems)
	}
	if cfg.CompactionThreshold != 64 {
		t.Fatalf("CompactionThreshold = %d, want 64", cfg.CompactionThreshold)
	}
	if cfg.MaxEstimatedTokens != 48000 {
		t.Fatalf("MaxEstimatedTokens = %d, want 48000", cfg.MaxEstimatedTokens)
	}
	if cfg.MicrocompactBytesThreshold != 12288 {
		t.Fatalf("MicrocompactBytesThreshold = %d, want 12288", cfg.MicrocompactBytesThreshold)
	}
	if cfg.MaxCompactionPasses != 3 {
		t.Fatalf("MaxCompactionPasses = %d, want 3", cfg.MaxCompactionPasses)
	}
}

func TestResolveCLIStartupConfigInfersAnthropicFromGenericMiniMaxConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("SESAME_MODEL_PROVIDER", "")
	t.Setenv("SESAME_COMPAT_MODE", "")
	t.Setenv("SESAME_BASE_URL", "")
	t.Setenv("SESAME_API_KEY", "")

	content := `{
		"model": "MiniMax-M2.7",
		"base_url": "https://api.minimaxi.com/anthropic",
		"api_key": "minimax-key"
	}`
	writeConfigFile(t, filepath.Join(home, ".sesame", "config.json"), content)

	cfg, err := ResolveCLIStartupConfig(CLIStartupOverrides{})
	if err != nil {
		t.Fatalf("ResolveCLIStartupConfig() error = %v", err)
	}
	if cfg.ModelProvider != "anthropic" {
		t.Fatalf("ModelProvider = %q, want anthropic", cfg.ModelProvider)
	}
	if cfg.AnthropicBaseURL != "https://api.minimaxi.com/anthropic" {
		t.Fatalf("AnthropicBaseURL = %q, want MiniMax base URL", cfg.AnthropicBaseURL)
	}
	if cfg.AnthropicAPIKey != "minimax-key" {
		t.Fatalf("AnthropicAPIKey = %q, want minimax-key", cfg.AnthropicAPIKey)
	}
}

func TestResolveCLIStartupConfigInfersOpenAICompatibleFromGenericVolcengineConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("SESAME_MODEL_PROVIDER", "")
	t.Setenv("SESAME_COMPAT_MODE", "")
	t.Setenv("SESAME_BASE_URL", "")
	t.Setenv("SESAME_API_KEY", "")

	content := `{
		"model": "glm-4-7-251222",
		"base_url": "https://ark.cn-beijing.volces.com/api/v3",
		"api_key": "volc-key"
	}`
	writeConfigFile(t, filepath.Join(home, ".sesame", "config.json"), content)

	cfg, err := ResolveCLIStartupConfig(CLIStartupOverrides{})
	if err != nil {
		t.Fatalf("ResolveCLIStartupConfig() error = %v", err)
	}
	if cfg.ModelProvider != "openai_compatible" {
		t.Fatalf("ModelProvider = %q, want openai_compatible", cfg.ModelProvider)
	}
	if cfg.OpenAIBaseURL != "https://ark.cn-beijing.volces.com/api/v3" {
		t.Fatalf("OpenAIBaseURL = %q, want Volcengine base URL", cfg.OpenAIBaseURL)
	}
	if cfg.OpenAIAPIKey != "volc-key" {
		t.Fatalf("OpenAIAPIKey = %q, want volc-key", cfg.OpenAIAPIKey)
	}
	if cfg.ProviderCacheProfile != "ark_responses" {
		t.Fatalf("ProviderCacheProfile = %q, want ark_responses", cfg.ProviderCacheProfile)
	}
}

func TestResolveCLIStartupConfigHonorsConfiguredProviderWithoutGenericBaseURL(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("SESAME_MODEL_PROVIDER", "")
	t.Setenv("SESAME_COMPAT_MODE", "")
	t.Setenv("SESAME_BASE_URL", "")
	t.Setenv("SESAME_API_KEY", "")

	content := `{
		"provider": "openai_compatible",
		"model": "glm-4-7-251222",
		"openai": {
			"api_key": "volc-key",
			"base_url": "https://ark.cn-beijing.volces.com/api/v3"
		},
		"anthropic": {
			"api_key": "minimax-key",
			"base_url": "https://api.minimaxi.com/anthropic"
		}
	}`
	writeConfigFile(t, filepath.Join(home, ".sesame", "config.json"), content)

	cfg, err := ResolveCLIStartupConfig(CLIStartupOverrides{})
	if err != nil {
		t.Fatalf("ResolveCLIStartupConfig() error = %v", err)
	}
	if cfg.ModelProvider != "openai_compatible" {
		t.Fatalf("ModelProvider = %q, want openai_compatible", cfg.ModelProvider)
	}
	if cfg.OpenAIBaseURL != "https://ark.cn-beijing.volces.com/api/v3" {
		t.Fatalf("OpenAIBaseURL = %q, want Volcengine base URL", cfg.OpenAIBaseURL)
	}
	if cfg.OpenAIAPIKey != "volc-key" {
		t.Fatalf("OpenAIAPIKey = %q, want volc-key", cfg.OpenAIAPIKey)
	}
}

func TestResolveCLIStartupConfigPrefersOverridesAndEnv(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("SESAME_MODEL_PROVIDER", "fake")
	t.Setenv("SESAME_MODEL", "")
	t.Setenv("SESAME_PERMISSION_PROFILE", "")

	cfg, err := ResolveCLIStartupConfig(CLIStartupOverrides{
		DataDir:        "E:/runtime",
		Addr:           "127.0.0.1:9000",
		Model:          "fake-smoke",
		PermissionMode: "trusted_local",
	})
	if err != nil {
		t.Fatalf("ResolveCLIStartupConfig() error = %v", err)
	}
	if cfg.DataDir != "E:/runtime" {
		t.Fatalf("DataDir = %q, want E:/runtime", cfg.DataDir)
	}
	if cfg.Addr != "127.0.0.1:9000" {
		t.Fatalf("Addr = %q, want 127.0.0.1:9000", cfg.Addr)
	}
	if cfg.ModelProvider != "fake" {
		t.Fatalf("ModelProvider = %q, want fake", cfg.ModelProvider)
	}
	if cfg.Model != "fake-smoke" {
		t.Fatalf("Model = %q, want fake-smoke", cfg.Model)
	}
	if cfg.PermissionProfile != "trusted_local" {
		t.Fatalf("PermissionProfile = %q, want trusted_local", cfg.PermissionProfile)
	}
}

func TestMissingSetupFields(t *testing.T) {
	openaiCfg := Config{
		ModelProvider: "openai_compatible",
		Model:         "gpt-5.4",
	}
	missing := MissingSetupFields(openaiCfg)
	if len(missing) != 2 {
		t.Fatalf("len(missing) = %d, want 2", len(missing))
	}

	fakeCfg := Config{
		ModelProvider: "fake",
		Model:         "fake-smoke",
	}
	if missing := MissingSetupFields(fakeCfg); len(missing) != 0 {
		t.Fatalf("missing = %v, want empty for fake provider", missing)
	}
}

func TestResolveSystemPrompt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "prompt.md")
	if err := os.WriteFile(path, []byte("from file"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	got, err := (Config{SystemPromptFile: path}).ResolveSystemPrompt()
	if err != nil {
		t.Fatalf("ResolveSystemPrompt() error = %v", err)
	}
	if got != "from file" {
		t.Fatalf("ResolveSystemPrompt() = %q, want from file", got)
	}
}

func writeConfigFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
}
