package config

import "testing"

func TestLoadUsesDefaultsAndRequiresDataDir(t *testing.T) {
	t.Setenv("AGENTD_ADDR", "")
	t.Setenv("AGENTD_DATA_DIR", t.TempDir())
	t.Setenv("ANTHROPIC_MODEL", "")
	t.Setenv("AGENTD_LOG_LEVEL", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("expected Load to succeed, got error: %v", err)
	}

	if cfg.Addr != "127.0.0.1:4317" {
		t.Fatalf("expected default addr 127.0.0.1:4317, got %q", cfg.Addr)
	}

	if cfg.DataDir == "" {
		t.Fatal("expected non-empty data dir")
	}

	if cfg.Model != "claude-sonnet-4-5" {
		t.Fatalf("expected default model claude-sonnet-4-5, got %q", cfg.Model)
	}
}
