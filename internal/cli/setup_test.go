package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go-agent/internal/config"
)

func TestEnsureRuntimeConfiguredWritesConfigFromPrompts(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	input := strings.NewReader("openai_compatible\ngpt-5.4\nhttps://example.com/v1\nsk-test\ntrusted_local\n")
	var out bytes.Buffer
	cfg := config.Config{
		ModelProvider: "openai_compatible",
	}

	if err := ensureRuntimeConfigured(input, &out, cfg); err != nil {
		t.Fatalf("ensureRuntimeConfigured() error = %v", err)
	}

	path := filepath.Join(home, ".sesame", "config.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	text := string(data)
	if !strings.Contains(text, `"provider": "openai_compatible"`) {
		t.Fatalf("config.json = %q, want provider", text)
	}
	if !strings.Contains(text, `"api_key": "sk-test"`) {
		t.Fatalf("config.json = %q, want api key", text)
	}
}
