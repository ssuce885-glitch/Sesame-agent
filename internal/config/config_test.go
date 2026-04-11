package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveCLIStartupConfigLoadsExplicitActiveProfile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	writeConfigFile(t, filepath.Join(home, ".sesame", "config.json"), `{
		"active_profile": "coding",
		"model_providers": {
			"anthropic-prod": {
				"api_family": "anthropic_messages",
				"base_url": "https://api.anthropic.com",
				"api_key_env": "ANTHROPIC_API_KEY"
			}
		},
		"profiles": {
			"coding": {
				"model": "claude-sonnet-4-5",
				"model_provider": "anthropic-prod",
				"cache_profile": "anthropic_default"
			}
		}
	}`)
	t.Setenv("ANTHROPIC_API_KEY", "test-key")

	cfg, err := ResolveCLIStartupConfig(CLIStartupOverrides{})
	if err != nil {
		t.Fatalf("ResolveCLIStartupConfig() error = %v", err)
	}
	if cfg.ActiveProfile != "coding" {
		t.Fatalf("ActiveProfile = %q, want coding", cfg.ActiveProfile)
	}
	if cfg.Profiles["coding"].ModelProvider != "anthropic-prod" {
		t.Fatalf("profile provider = %q, want anthropic-prod", cfg.Profiles["coding"].ModelProvider)
	}
}

func TestResolveCLIStartupConfigRejectsLegacyProviderFields(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	writeConfigFile(t, filepath.Join(home, ".sesame", "config.json"), `{
		"provider": "openai_compatible",
		"model": "gpt-5.4",
		"base_url": "https://example.com/v1",
		"api_key": "legacy"
	}`)

	_, err := ResolveCLIStartupConfig(CLIStartupOverrides{})
	if err == nil {
		t.Fatal("ResolveCLIStartupConfig() error = nil, want legacy config rejection")
	}
	if !strings.Contains(err.Error(), "legacy config fields are no longer supported") {
		t.Fatalf("error = %v, want legacy config rejection message", err)
	}
}

func TestResolveCLIStartupConfigBuildsRuntimeCompatibilityFieldsFromActiveProfile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	writeConfigFile(t, filepath.Join(home, ".sesame", "config.json"), `{
		"active_profile": "coding",
		"model_providers": {
			"anthropic-prod": {
				"api_family": "anthropic_messages",
				"base_url": "https://api.anthropic.com",
				"api_key_env": "ANTHROPIC_API_KEY"
			}
		},
		"profiles": {
			"coding": {
				"model": "claude-sonnet-4-5",
				"model_provider": "anthropic-prod",
				"cache_profile": "anthropic_default"
			}
		}
	}`)
	t.Setenv("ANTHROPIC_API_KEY", "test-key")

	cfg, err := ResolveCLIStartupConfig(CLIStartupOverrides{})
	if err != nil {
		t.Fatalf("ResolveCLIStartupConfig() error = %v", err)
	}
	if cfg.ModelProvider != "anthropic" {
		t.Fatalf("ModelProvider = %q, want anthropic", cfg.ModelProvider)
	}
	if cfg.Model != "claude-sonnet-4-5" {
		t.Fatalf("Model = %q, want claude-sonnet-4-5", cfg.Model)
	}
	if cfg.AnthropicAPIKey != "test-key" {
		t.Fatalf("AnthropicAPIKey = %q, want test-key", cfg.AnthropicAPIKey)
	}
	if cfg.AnthropicBaseURL != "https://api.anthropic.com" {
		t.Fatalf("AnthropicBaseURL = %q, want https://api.anthropic.com", cfg.AnthropicBaseURL)
	}
	if cfg.ProviderCacheProfile != "anthropic_default" {
		t.Fatalf("ProviderCacheProfile = %q, want anthropic_default", cfg.ProviderCacheProfile)
	}
}

func TestMissingSetupFieldsAllowsFakeProviderWithoutAuthFields(t *testing.T) {
	cfg := Config{
		ActiveProfile: "smoke",
		ModelProviders: map[string]ModelProviderConfig{
			"fake-provider": {
				ID:        "fake-provider",
				APIFamily: "fake",
			},
		},
		Profiles: map[string]ProfileConfig{
			"smoke": {
				ID:            "smoke",
				Model:         "fake-smoke",
				ModelProvider: "fake-provider",
			},
		},
	}

	missing := MissingSetupFields(cfg)
	if len(missing) != 0 {
		t.Fatalf("MissingSetupFields() = %v, want none for fake provider", missing)
	}
}

func writeConfigFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", path, err)
	}
}
