package cli

import (
	"bytes"
	"errors"
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
	if provider.APIFamily != "openai_responses" {
		t.Fatalf("provider api_family = %q, want openai_responses", provider.APIFamily)
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
				return config.Config{}, fmt.Errorf("wrapped: %w", config.ErrActiveProfileRequired)
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

func TestEnsureRuntimeConfiguredProviderPromptDoesNotOfferFake(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	var out bytes.Buffer
	stdin := strings.NewReader(strings.Join([]string{
		"fake",
		"anthropic",
		"claude-sonnet-4-5",
		"https://api.anthropic.com",
		"ANTHROPIC_API_KEY",
		"trusted_local",
	}, "\n") + "\n")

	if err := ensureRuntimeConfigured(stdin, &out, config.Config{}); err != nil {
		t.Fatalf("ensureRuntimeConfigured() error = %v", err)
	}
	if strings.Contains(out.String(), "Provider [anthropic/openai_compatible/fake]") {
		t.Fatalf("provider prompt still exposes fake option:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "Provider [anthropic/openai_compatible]") {
		t.Fatalf("provider prompt missing explicit provider choices:\n%s", out.String())
	}
}

func TestIsRecoverableSetupConfigErrorUsesTypedErrors(t *testing.T) {
	if !isRecoverableSetupConfigError(fmt.Errorf("wrapped typed error: %w", config.ErrActiveProfileRequired)) {
		t.Fatal("typed active profile error should be recoverable")
	}
	if isRecoverableSetupConfigError(errors.New("active_profile is required")) {
		t.Fatal("plain string error should not be recoverable")
	}
	if isRecoverableSetupConfigError(fmt.Errorf("wrapped typed error: %w", config.ErrLegacyConfigFieldsUnsupported)) {
		t.Fatal("legacy-config typed error should not be recoverable")
	}
}
