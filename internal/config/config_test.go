package config

import "testing"

func TestLoadUsesDefaultsAndRequiresDataDir(t *testing.T) {
	t.Setenv("AGENTD_ADDR", "")
	t.Setenv("AGENTD_DATA_DIR", "")
	t.Setenv("ANTHROPIC_MODEL", "")
	t.Setenv("AGENTD_LOG_LEVEL", "")

	cfg, err := Load()
	if err == nil {
		t.Fatal("expected error when AGENTD_DATA_DIR is empty")
	}

	if cfg.Addr != "127.0.0.1:4317" {
		t.Fatalf("expected default addr 127.0.0.1:4317, got %q", cfg.Addr)
	}

	if cfg.DataDir != "" {
		t.Fatalf("expected empty data dir on error, got %q", cfg.DataDir)
	}

	if cfg.Model != "claude-sonnet-4-5" {
		t.Fatalf("expected default model claude-sonnet-4-5, got %q", cfg.Model)
	}
}
