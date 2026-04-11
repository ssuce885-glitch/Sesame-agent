package config

import (
	"errors"
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

func TestResolveCLIStartupConfigRejectsFakeAPIFamily(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	writeConfigFile(t, filepath.Join(home, ".sesame", "config.json"), `{
		"active_profile": "smoke",
		"model_providers": {
			"fake-provider": {
				"api_family": "fake",
				"base_url": "",
				"api_key_env": ""
			}
		},
		"profiles": {
			"smoke": {
				"model": "fake-smoke",
				"model_provider": "fake-provider",
				"cache_profile": "none"
			}
		}
	}`)

	_, err := ResolveCLIStartupConfig(CLIStartupOverrides{})
	if err == nil {
		t.Fatal("ResolveCLIStartupConfig() error = nil, want fake api_family rejection")
	}
	if !errors.Is(err, ErrUnsupportedAPIFamily) {
		t.Fatalf("error = %v, want ErrUnsupportedAPIFamily", err)
	}
}

func TestResolveCLIStartupConfigModelOverrideAffectsOnlyRuntimeSelection(t *testing.T) {
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
			},
			"review": {
				"model": "claude-3-5-haiku",
				"model_provider": "anthropic-prod",
				"cache_profile": "anthropic_default"
			}
		}
	}`)
	t.Setenv("ANTHROPIC_API_KEY", "test-key")

	cfg, err := ResolveCLIStartupConfig(CLIStartupOverrides{Model: "override-model"})
	if err != nil {
		t.Fatalf("ResolveCLIStartupConfig() error = %v", err)
	}
	if cfg.Model != "override-model" {
		t.Fatalf("Model = %q, want override-model", cfg.Model)
	}
	if cfg.Profiles["coding"].Model != "override-model" {
		t.Fatalf("active profile model in cfg.Profiles = %q, want override-model", cfg.Profiles["coding"].Model)
	}
	if cfg.Profiles["review"].Model != "claude-3-5-haiku" {
		t.Fatalf("non-active profile model in cfg.Profiles = %q, want unchanged claude-3-5-haiku", cfg.Profiles["review"].Model)
	}
}

func TestResolveCLIStartupConfigRejectsUnsupportedAPIFamily(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	writeConfigFile(t, filepath.Join(home, ".sesame", "config.json"), `{
		"active_profile": "coding",
		"model_providers": {
			"custom": {
				"api_family": "totally_custom_family",
				"base_url": "https://example.com",
				"api_key_env": "CUSTOM_API_KEY"
			}
		},
		"profiles": {
			"coding": {
				"model": "custom-model",
				"model_provider": "custom",
				"cache_profile": "none"
			}
		}
	}`)
	t.Setenv("CUSTOM_API_KEY", "test-key")

	_, err := ResolveCLIStartupConfig(CLIStartupOverrides{})
	if err == nil {
		t.Fatal("ResolveCLIStartupConfig() error = nil, want unsupported api_family rejection")
	}
	if !errors.Is(err, ErrUnsupportedAPIFamily) {
		t.Fatalf("error = %v, want ErrUnsupportedAPIFamily", err)
	}
}

func TestResolveCLIStartupConfigRejectsOpenAIChatCompletionsAPIFamily(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	writeConfigFile(t, filepath.Join(home, ".sesame", "config.json"), `{
		"active_profile": "coding",
		"model_providers": {
			"openai": {
				"api_family": "openai_chat_completions",
				"base_url": "https://api.openai.com/v1",
				"api_key_env": "OPENAI_API_KEY"
			}
		},
		"profiles": {
			"coding": {
				"model": "gpt-5.4",
				"model_provider": "openai",
				"cache_profile": "openai_responses"
			}
		}
	}`)
	t.Setenv("OPENAI_API_KEY", "test-key")

	_, err := ResolveCLIStartupConfig(CLIStartupOverrides{})
	if err == nil {
		t.Fatal("ResolveCLIStartupConfig() error = nil, want openai_chat_completions rejection")
	}
	if !errors.Is(err, ErrUnsupportedAPIFamily) {
		t.Fatalf("error = %v, want ErrUnsupportedAPIFamily", err)
	}
}

func TestResolveCLIStartupConfigAcceptsOpenAIResponsesAPIFamily(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	writeConfigFile(t, filepath.Join(home, ".sesame", "config.json"), `{
		"active_profile": "coding",
		"model_providers": {
			"openai": {
				"api_family": "openai_responses",
				"base_url": "https://api.openai.com/v1",
				"api_key_env": "OPENAI_API_KEY"
			}
		},
		"profiles": {
			"coding": {
				"model": "gpt-5.4",
				"model_provider": "openai",
				"cache_profile": "openai_responses"
			}
		}
	}`)
	t.Setenv("OPENAI_API_KEY", "test-key")

	cfg, err := ResolveCLIStartupConfig(CLIStartupOverrides{})
	if err != nil {
		t.Fatalf("ResolveCLIStartupConfig() error = %v", err)
	}
	if cfg.ModelProvider != "openai_compatible" {
		t.Fatalf("ModelProvider = %q, want openai_compatible", cfg.ModelProvider)
	}
	if cfg.OpenAIAPIKey != "test-key" {
		t.Fatalf("OpenAIAPIKey = %q, want test-key", cfg.OpenAIAPIKey)
	}
	if cfg.OpenAIBaseURL != "https://api.openai.com/v1" {
		t.Fatalf("OpenAIBaseURL = %q, want https://api.openai.com/v1", cfg.OpenAIBaseURL)
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
