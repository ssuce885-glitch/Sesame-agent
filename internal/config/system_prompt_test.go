package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveSystemPromptDefaultsToSesameIdentity(t *testing.T) {
	got, err := (Config{}).ResolveSystemPrompt()
	if err != nil {
		t.Fatalf("ResolveSystemPrompt returned error: %v", err)
	}
	if !strings.Contains(got, "You are Sesame") {
		t.Fatalf("default prompt missing Sesame identity: %q", got)
	}
	if !strings.Contains(got, "Do not claim to be Claude") {
		t.Fatalf("default prompt missing provider identity guardrail: %q", got)
	}
}

func TestResolveSystemPromptUsesExplicitPrompt(t *testing.T) {
	got, err := (Config{SystemPrompt: "Custom prompt."}).ResolveSystemPrompt()
	if err != nil {
		t.Fatalf("ResolveSystemPrompt returned error: %v", err)
	}
	if got != "Custom prompt." {
		t.Fatalf("prompt = %q, want explicit prompt", got)
	}
}

func TestResolveSystemPromptUsesPromptFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "prompt.md")
	if err := os.WriteFile(path, []byte("File prompt.\n"), 0o600); err != nil {
		t.Fatalf("write prompt file: %v", err)
	}
	got, err := (Config{SystemPromptFile: path}).ResolveSystemPrompt()
	if err != nil {
		t.Fatalf("ResolveSystemPrompt returned error: %v", err)
	}
	if got != "File prompt." {
		t.Fatalf("prompt = %q, want file prompt", got)
	}
}
