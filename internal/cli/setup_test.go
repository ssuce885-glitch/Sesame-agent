package cli

import (
	"fmt"
	"io"
	"strings"
	"testing"

	"go-agent/internal/config"
)

func TestEnsureRuntimeConfiguredWritesExplicitModelConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	stdin := strings.NewReader(strings.Join([]string{
		"openai_compatible",
		"gpt-5.4",
		"https://example.com/v1",
		"OPENAI_API_KEY",
		"trusted_local",
	}, "\n") + "\n")

	if err := ensureRuntimeConfigured(stdin, io.Discard, config.Config{}); err != nil {
		t.Fatalf("ensureRuntimeConfigured() error = %v", err)
	}

	uc, err := config.LoadUserConfig()
	if err != nil {
		t.Fatalf("LoadUserConfig() error = %v", err)
	}
	if uc.ActiveProfile == "" {
		t.Fatal("ActiveProfile is empty, want explicit active_profile")
	}
	profile, ok := uc.Profiles[uc.ActiveProfile]
	if !ok {
		t.Fatalf("active profile %q missing from profiles", uc.ActiveProfile)
	}
	if profile.Model != "gpt-5.4" {
		t.Fatalf("profile model = %q, want gpt-5.4", profile.Model)
	}
	provider, ok := uc.ModelProviders[profile.ModelProvider]
	if !ok {
		t.Fatalf("profile provider %q missing from model_providers", profile.ModelProvider)
	}
	if provider.APIFamily == "" {
		t.Fatal("provider api_family is empty")
	}
	if provider.BaseURL != "https://example.com/v1" {
		t.Fatalf("provider base_url = %q, want https://example.com/v1", provider.BaseURL)
	}
	if provider.APIKeyEnv != "OPENAI_API_KEY" {
		t.Fatalf("provider api_key_env = %q, want OPENAI_API_KEY", provider.APIKeyEnv)
	}
}

func TestLoadRuntimeConfigWithSetupRecoversFromMissingActiveProfile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	stdin := strings.NewReader(strings.Join([]string{
		"anthropic",
		"claude-sonnet-4-5",
		"https://api.anthropic.com",
		"ANTHROPIC_API_KEY",
		"trusted_local",
	}, "\n") + "\n")

	calls := 0
	app := App{
		Stdin:  stdin,
		Stdout: io.Discard,
		LoadConfig: func(opts Options) (config.Config, error) {
			calls++
			if calls == 1 {
				return config.Config{}, fmt.Errorf("active_profile is required")
			}
			return config.Config{Addr: "127.0.0.1:4317"}, nil
		},
	}

	cfg, err := app.loadRuntimeConfigWithSetup(Options{})
	if err != nil {
		t.Fatalf("loadRuntimeConfigWithSetup() error = %v", err)
	}
	if calls != 2 {
		t.Fatalf("LoadConfig call count = %d, want 2", calls)
	}
	if cfg.Addr != "127.0.0.1:4317" {
		t.Fatalf("cfg.Addr = %q, want 127.0.0.1:4317", cfg.Addr)
	}
}
