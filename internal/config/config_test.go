package config

import "testing"

func TestLoadUsesDefaultsAndRequiresDataDir(t *testing.T) {
	t.Setenv("AGENTD_ADDR", "")
	dataDir := t.TempDir()
	t.Setenv("AGENTD_DATA_DIR", dataDir)
	t.Setenv("ANTHROPIC_MODEL", "")
	t.Setenv("AGENTD_LOG_LEVEL", "")
	t.Setenv("ANTHROPIC_API_KEY", "test-api-key")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("expected Load to succeed, got error: %v", err)
	}

	if cfg.Addr != "127.0.0.1:4317" {
		t.Fatalf("expected default addr 127.0.0.1:4317, got %q", cfg.Addr)
	}

	if cfg.DataDir != dataDir {
		t.Fatalf("expected data dir to equal env value, got %q", cfg.DataDir)
	}

	if cfg.Model != "claude-sonnet-4-5" {
		t.Fatalf("expected default model claude-sonnet-4-5, got %q", cfg.Model)
	}

	if cfg.AnthropicAPIKey != "test-api-key" {
		t.Fatalf("expected api key from env, got %q", cfg.AnthropicAPIKey)
	}
}

func TestLoadRequiresDataDir(t *testing.T) {
	t.Setenv("AGENTD_ADDR", "")
	t.Setenv("AGENTD_DATA_DIR", "")
	t.Setenv("ANTHROPIC_MODEL", "")
	t.Setenv("AGENTD_LOG_LEVEL", "")
	t.Setenv("ANTHROPIC_API_KEY", "test-api-key")

	if _, err := Load(); err == nil {
		t.Fatal("expected Load to return an error when AGENTD_DATA_DIR is empty")
	}
}
