package config

import "testing"

func TestLoadUsesDefaultsAndRequiresDataDir(t *testing.T) {
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
		if cfg.ProviderCacheProfile != "none" {
			t.Fatalf("ProviderCacheProfile = %q, want %q", cfg.ProviderCacheProfile, "none")
		}
		if cfg.CacheExpirySeconds != 86400 {
			t.Fatalf("CacheExpirySeconds = %d, want %d", cfg.CacheExpirySeconds, 86400)
		}
		if cfg.MicrocompactBytesThreshold != 4096 {
			t.Fatalf("MicrocompactBytesThreshold = %d, want %d", cfg.MicrocompactBytesThreshold, 4096)
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
