package config

import "testing"

func TestLoadUsesDefaultsAndRequiresDataDir(t *testing.T) {
	t.Run("uses defaults when data dir is set", func(t *testing.T) {
		t.Setenv("AGENTD_ADDR", "")
		dataDir := t.TempDir()
		t.Setenv("AGENTD_DATA_DIR", dataDir)
		t.Setenv("ANTHROPIC_MODEL", "")
		t.Setenv("AGENTD_LOG_LEVEL", "")
		t.Setenv("ANTHROPIC_API_KEY", "")

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
}
