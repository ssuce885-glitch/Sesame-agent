package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadUsesDefaultsAndRequiresDataDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	t.Run("uses defaults when data dir is set", func(t *testing.T) {
		t.Setenv("AGENTD_ADDR", "")
		dataDir := t.TempDir()
		t.Setenv("AGENTD_DATA_DIR", dataDir)
		t.Setenv("AGENTD_MODEL_PROVIDER", "")
		t.Setenv("ANTHROPIC_MODEL", "")
		t.Setenv("AGENTD_LOG_LEVEL", "")
		t.Setenv("ANTHROPIC_API_KEY", "")
		t.Setenv("AGENTD_PERMISSION_PROFILE", "")
		t.Setenv("AGENTD_MAX_TOOL_STEPS", "")
		t.Setenv("AGENTD_MAX_SHELL_OUTPUT_BYTES", "")
		t.Setenv("AGENTD_SHELL_TIMEOUT_SECONDS", "")
		t.Setenv("AGENTD_MAX_FILE_WRITE_BYTES", "")
		t.Setenv("AGENTD_MAX_RECENT_ITEMS", "")
		t.Setenv("AGENTD_COMPACTION_THRESHOLD", "")
		t.Setenv("AGENTD_MAX_ESTIMATED_TOKENS", "")
		t.Setenv("AGENTD_MAX_COMPACTION_PASSES", "")
		t.Setenv("AGENTD_PROVIDER_CACHE_PROFILE", "")
		t.Setenv("AGENTD_CACHE_EXPIRY_SECONDS", "")
		t.Setenv("AGENTD_MICROCOMPACT_BYTES_THRESHOLD", "")
		t.Setenv("AGENTD_MAX_CONCURRENT_TASKS", "")
		t.Setenv("AGENTD_TASK_OUTPUT_MAX_BYTES", "")
		t.Setenv("AGENTD_REMOTE_EXECUTOR_SHIM_COMMAND", "")
		t.Setenv("AGENTD_REMOTE_EXECUTOR_TIMEOUT_SECONDS", "")

		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load() returned error: %v", err)
		}

		if cfg.Addr != "127.0.0.1:4317" {
			t.Fatalf("Addr = %q, want %q", cfg.Addr, "127.0.0.1:4317")
		}
		if cfg.DataDir != dataDir {
			t.Fatalf("DataDir = %q, want %q", cfg.DataDir, dataDir)
		}
		if cfg.Model != "claude-sonnet-4-5" {
			t.Fatalf("Model = %q, want %q", cfg.Model, "claude-sonnet-4-5")
		}
		if cfg.ModelProvider != "anthropic" {
			t.Fatalf("ModelProvider = %q, want %q", cfg.ModelProvider, "anthropic")
		}
		if cfg.PermissionProfile != "read_only" {
			t.Fatalf("PermissionProfile = %q, want %q", cfg.PermissionProfile, "read_only")
		}
		if cfg.MaxToolSteps != 8 {
			t.Fatalf("MaxToolSteps = %d, want %d", cfg.MaxToolSteps, 8)
		}
		if cfg.MaxShellOutputBytes != 4096 {
			t.Fatalf("MaxShellOutputBytes = %d, want %d", cfg.MaxShellOutputBytes, 4096)
		}
		if cfg.ShellTimeoutSeconds != 30 {
			t.Fatalf("ShellTimeoutSeconds = %d, want %d", cfg.ShellTimeoutSeconds, 30)
		}
		if cfg.MaxFileWriteBytes != 1<<20 {
			t.Fatalf("MaxFileWriteBytes = %d, want %d", cfg.MaxFileWriteBytes, 1<<20)
		}
		if cfg.MaxRecentItems != 8 {
			t.Fatalf("MaxRecentItems = %d, want %d", cfg.MaxRecentItems, 8)
		}
		if cfg.CompactionThreshold != 16 {
			t.Fatalf("CompactionThreshold = %d, want %d", cfg.CompactionThreshold, 16)
		}
		if cfg.MaxEstimatedTokens != 6000 {
			t.Fatalf("MaxEstimatedTokens = %d, want %d", cfg.MaxEstimatedTokens, 6000)
		}
		if cfg.MaxCompactionPasses != 1 {
			t.Fatalf("MaxCompactionPasses = %d, want %d", cfg.MaxCompactionPasses, 1)
		}
		if cfg.MaxWorkspacePromptBytes != 32768 {
			t.Fatalf("MaxWorkspacePromptBytes = %d, want %d", cfg.MaxWorkspacePromptBytes, 32768)
		}
		if cfg.ProviderCacheProfile != "none" {
			t.Fatalf("ProviderCacheProfile = %q, want %q", cfg.ProviderCacheProfile, "none")
		}
		if cfg.CacheExpirySeconds != 86400 {
			t.Fatalf("CacheExpirySeconds = %d, want %d", cfg.CacheExpirySeconds, 86400)
		}
		if cfg.MicrocompactBytesThreshold != 4096 {
			t.Fatalf("MicrocompactBytesThreshold = %d, want %d", cfg.MicrocompactBytesThreshold, 4096)
		}
		if cfg.MaxConcurrentTasks != 8 {
			t.Fatalf("MaxConcurrentTasks = %d, want %d", cfg.MaxConcurrentTasks, 8)
		}
		if cfg.TaskOutputMaxBytes != 1<<20 {
			t.Fatalf("TaskOutputMaxBytes = %d, want %d", cfg.TaskOutputMaxBytes, 1<<20)
		}
		if cfg.RemoteExecutorShimCommand != "" {
			t.Fatalf("RemoteExecutorShimCommand = %q, want empty", cfg.RemoteExecutorShimCommand)
		}
		if cfg.RemoteExecutorTimeoutSeconds != 300 {
			t.Fatalf("RemoteExecutorTimeoutSeconds = %d, want %d", cfg.RemoteExecutorTimeoutSeconds, 300)
		}
	})

	t.Run("returns zero config when data dir is missing", func(t *testing.T) {
		t.Setenv("AGENTD_ADDR", "")
		t.Setenv("AGENTD_DATA_DIR", "")
		t.Setenv("ANTHROPIC_MODEL", "")
		t.Setenv("AGENTD_LOG_LEVEL", "")
		t.Setenv("ANTHROPIC_API_KEY", "")

		cfg, err := Load()
		if err == nil {
			t.Fatal("Load() error = nil, want error")
		}
		if cfg != (Config{}) {
			t.Fatalf("cfg = %#v, want zero Config", cfg)
		}
	})

	t.Run("reads anthropic api key", func(t *testing.T) {
		t.Setenv("AGENTD_ADDR", "")
		t.Setenv("AGENTD_DATA_DIR", t.TempDir())
		t.Setenv("ANTHROPIC_MODEL", "")
		t.Setenv("AGENTD_LOG_LEVEL", "")
		t.Setenv("ANTHROPIC_API_KEY", "test-api-key")

		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load() returned error: %v", err)
		}
		if cfg.AnthropicAPIKey != "test-api-key" {
			t.Fatalf("AnthropicAPIKey = %q, want %q", cfg.AnthropicAPIKey, "test-api-key")
		}
	})

	t.Run("reads runtime overrides", func(t *testing.T) {
		t.Setenv("AGENTD_ADDR", "")
		t.Setenv("AGENTD_DATA_DIR", t.TempDir())
		t.Setenv("ANTHROPIC_MODEL", "")
		t.Setenv("AGENTD_LOG_LEVEL", "")
		t.Setenv("ANTHROPIC_API_KEY", "")
		t.Setenv("AGENTD_PERMISSION_PROFILE", "trusted_local")
		t.Setenv("AGENTD_MAX_TOOL_STEPS", "3")
		t.Setenv("AGENTD_MAX_SHELL_OUTPUT_BYTES", "512")
		t.Setenv("AGENTD_SHELL_TIMEOUT_SECONDS", "12")
		t.Setenv("AGENTD_MAX_FILE_WRITE_BYTES", "2048")
		t.Setenv("AGENTD_MAX_RECENT_ITEMS", "4")
		t.Setenv("AGENTD_COMPACTION_THRESHOLD", "9")
		t.Setenv("AGENTD_MAX_ESTIMATED_TOKENS", "2222")
		t.Setenv("AGENTD_MAX_COMPACTION_PASSES", "5")

		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load() returned error: %v", err)
		}

		if cfg.PermissionProfile != "trusted_local" {
			t.Fatalf("PermissionProfile = %q, want %q", cfg.PermissionProfile, "trusted_local")
		}
		if cfg.MaxToolSteps != 3 {
			t.Fatalf("MaxToolSteps = %d, want %d", cfg.MaxToolSteps, 3)
		}
		if cfg.MaxShellOutputBytes != 512 {
			t.Fatalf("MaxShellOutputBytes = %d, want %d", cfg.MaxShellOutputBytes, 512)
		}
		if cfg.ShellTimeoutSeconds != 12 {
			t.Fatalf("ShellTimeoutSeconds = %d, want %d", cfg.ShellTimeoutSeconds, 12)
		}
		if cfg.MaxFileWriteBytes != 2048 {
			t.Fatalf("MaxFileWriteBytes = %d, want %d", cfg.MaxFileWriteBytes, 2048)
		}
		if cfg.MaxRecentItems != 4 {
			t.Fatalf("MaxRecentItems = %d, want %d", cfg.MaxRecentItems, 4)
		}
		if cfg.CompactionThreshold != 9 {
			t.Fatalf("CompactionThreshold = %d, want %d", cfg.CompactionThreshold, 9)
		}
		if cfg.MaxEstimatedTokens != 2222 {
			t.Fatalf("MaxEstimatedTokens = %d, want %d", cfg.MaxEstimatedTokens, 2222)
		}
		if cfg.MaxCompactionPasses != 5 {
			t.Fatalf("MaxCompactionPasses = %d, want %d", cfg.MaxCompactionPasses, 5)
		}
	})
}

func TestLoadReadsTaskExecutionOverrides(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	t.Setenv("AGENTD_DATA_DIR", t.TempDir())
	t.Setenv("AGENTD_MAX_CONCURRENT_TASKS", "3")
	t.Setenv("AGENTD_TASK_OUTPUT_MAX_BYTES", "8192")
	t.Setenv("AGENTD_REMOTE_EXECUTOR_SHIM_COMMAND", "ssh deploy@example")
	t.Setenv("AGENTD_REMOTE_EXECUTOR_TIMEOUT_SECONDS", "45")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}

	if cfg.MaxConcurrentTasks != 3 {
		t.Fatalf("MaxConcurrentTasks = %d, want %d", cfg.MaxConcurrentTasks, 3)
	}
	if cfg.TaskOutputMaxBytes != 8192 {
		t.Fatalf("TaskOutputMaxBytes = %d, want %d", cfg.TaskOutputMaxBytes, 8192)
	}
	if cfg.RemoteExecutorShimCommand != "ssh deploy@example" {
		t.Fatalf("RemoteExecutorShimCommand = %q, want %q", cfg.RemoteExecutorShimCommand, "ssh deploy@example")
	}
	if cfg.RemoteExecutorTimeoutSeconds != 45 {
		t.Fatalf("RemoteExecutorTimeoutSeconds = %d, want %d", cfg.RemoteExecutorTimeoutSeconds, 45)
	}
}

func TestLoadReadsProviderCacheOverrides(t *testing.T) {
	t.Setenv("AGENTD_ADDR", "")
	t.Setenv("AGENTD_DATA_DIR", t.TempDir())
	t.Setenv("ANTHROPIC_MODEL", "")
	t.Setenv("AGENTD_LOG_LEVEL", "")
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("AGENTD_PROVIDER_CACHE_PROFILE", "ark_responses")
	t.Setenv("AGENTD_CACHE_EXPIRY_SECONDS", "7200")
	t.Setenv("AGENTD_MICROCOMPACT_BYTES_THRESHOLD", "8192")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}

	if cfg.ProviderCacheProfile != "ark_responses" {
		t.Fatalf("ProviderCacheProfile = %q, want %q", cfg.ProviderCacheProfile, "ark_responses")
	}
	if cfg.CacheExpirySeconds != 7200 {
		t.Fatalf("CacheExpirySeconds = %d, want %d", cfg.CacheExpirySeconds, 7200)
	}
	if cfg.MicrocompactBytesThreshold != 8192 {
		t.Fatalf("MicrocompactBytesThreshold = %d, want %d", cfg.MicrocompactBytesThreshold, 8192)
	}
}

func TestLoadReadsSystemPromptOverrides(t *testing.T) {
	t.Setenv("AGENTD_ADDR", "")
	t.Setenv("AGENTD_DATA_DIR", t.TempDir())
	t.Setenv("ANTHROPIC_MODEL", "")
	t.Setenv("AGENTD_LOG_LEVEL", "")
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("AGENTD_SYSTEM_PROMPT", "env prompt")
	t.Setenv("AGENTD_SYSTEM_PROMPT_FILE", "C:/tmp/system.md")
	t.Setenv("AGENTD_MAX_WORKSPACE_PROMPT_BYTES", "1234")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}

	if cfg.SystemPrompt != "env prompt" {
		t.Fatalf("SystemPrompt = %q, want %q", cfg.SystemPrompt, "env prompt")
	}
	if cfg.SystemPromptFile != "C:/tmp/system.md" {
		t.Fatalf("SystemPromptFile = %q, want %q", cfg.SystemPromptFile, "C:/tmp/system.md")
	}
	if cfg.MaxWorkspacePromptBytes != 1234 {
		t.Fatalf("MaxWorkspacePromptBytes = %d, want %d", cfg.MaxWorkspacePromptBytes, 1234)
	}
}

func TestLoadUserConfig(t *testing.T) {
	t.Run("file not found returns zero config", func(t *testing.T) {
		t.Setenv("HOME", t.TempDir())
		t.Setenv("USERPROFILE", os.Getenv("HOME"))
		uc, err := loadUserConfig()
		if err != nil {
			t.Fatalf("loadUserConfig() error = %v", err)
		}
		if uc != (UserConfig{}) {
			t.Fatalf("expected zero UserConfig, got %+v", uc)
		}
	})

	t.Run("valid file is parsed", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		t.Setenv("USERPROFILE", home)
		dir := filepath.Join(home, ".agentd")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		content := `{"provider":"anthropic","anthropic":{"api_key":"sk-test","model":"claude-opus-4"}}`
		if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		uc, err := loadUserConfig()
		if err != nil {
			t.Fatalf("loadUserConfig() error = %v", err)
		}
		if uc.Provider != "anthropic" {
			t.Fatalf("Provider = %q, want %q", uc.Provider, "anthropic")
		}
		if uc.Anthropic.APIKey != "sk-test" {
			t.Fatalf("APIKey = %q, want %q", uc.Anthropic.APIKey, "sk-test")
		}
		if uc.Anthropic.Model != "claude-opus-4" {
			t.Fatalf("Model = %q, want %q", uc.Anthropic.Model, "claude-opus-4")
		}
	})

	t.Run("invalid JSON returns error", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		t.Setenv("USERPROFILE", home)
		dir := filepath.Join(home, ".agentd")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte("{bad json"), 0o644); err != nil {
			t.Fatal(err)
		}
		_, err := loadUserConfig()
		if err == nil {
			t.Fatal("loadUserConfig() error = nil, want error")
		}
	})
}

func TestLoadUserConfigEnvPriority(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	dir := filepath.Join(home, ".agentd")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := `{"anthropic":{"api_key":"from-file"}}`
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Run("env var overrides file value", func(t *testing.T) {
		t.Setenv("AGENTD_DATA_DIR", t.TempDir())
		t.Setenv("ANTHROPIC_API_KEY", "from-env")
		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}
		if cfg.AnthropicAPIKey != "from-env" {
			t.Fatalf("AnthropicAPIKey = %q, want %q", cfg.AnthropicAPIKey, "from-env")
		}
	})

	t.Run("file value used when env var absent", func(t *testing.T) {
		t.Setenv("AGENTD_DATA_DIR", t.TempDir())
		t.Setenv("ANTHROPIC_API_KEY", "")
		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}
		if cfg.AnthropicAPIKey != "from-file" {
			t.Fatalf("AnthropicAPIKey = %q, want %q", cfg.AnthropicAPIKey, "from-file")
		}
	})
}

func TestResolveSystemPrompt(t *testing.T) {
	t.Run("prefers inline system prompt over file", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "prompt.md")
		if err := os.WriteFile(path, []byte("from file"), 0o644); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}

		got, err := (Config{
			SystemPrompt:     "from env",
			SystemPromptFile: path,
		}).ResolveSystemPrompt()
		if err != nil {
			t.Fatalf("ResolveSystemPrompt() error = %v", err)
		}
		if got != "from env" {
			t.Fatalf("ResolveSystemPrompt() = %q, want %q", got, "from env")
		}
	})

	t.Run("reads configured prompt file", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "prompt.md")
		if err := os.WriteFile(path, []byte("from file"), 0o644); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}

		got, err := (Config{SystemPromptFile: path}).ResolveSystemPrompt()
		if err != nil {
			t.Fatalf("ResolveSystemPrompt() error = %v", err)
		}
		if got != "from file" {
			t.Fatalf("ResolveSystemPrompt() = %q, want %q", got, "from file")
		}
	})

	t.Run("returns error for missing file", func(t *testing.T) {
		_, err := (Config{
			SystemPromptFile: filepath.Join(t.TempDir(), "missing.md"),
		}).ResolveSystemPrompt()
		if err == nil {
			t.Fatal("ResolveSystemPrompt() error = nil, want error")
		}
	})
}
