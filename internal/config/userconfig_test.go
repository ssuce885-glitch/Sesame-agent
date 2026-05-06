package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestUserConfigPatchRootVision(t *testing.T) {
	root := userConfigPatchRoot(UserConfig{
		Vision: UserConfigVision{
			Provider: "openai_compatible",
			APIKey:   "vision-key",
			BaseURL:  "https://api.openai.com/v1",
			Model:    "vision-model",
		},
	})

	var vision map[string]string
	if err := json.Unmarshal(root["vision"], &vision); err != nil {
		t.Fatalf("unmarshal vision patch: %v", err)
	}
	if vision["provider"] != "openai_compatible" {
		t.Fatalf("provider = %q, want openai_compatible", vision["provider"])
	}
	if vision["api_key"] != "vision-key" {
		t.Fatalf("api_key = %q, want vision-key", vision["api_key"])
	}
	if vision["base_url"] != "https://api.openai.com/v1" {
		t.Fatalf("base_url = %q, want https://api.openai.com/v1", vision["base_url"])
	}
	if vision["model"] != "vision-model" {
		t.Fatalf("model = %q, want vision-model", vision["model"])
	}
}

func TestUserConfigPatchRootResetVision(t *testing.T) {
	root := userConfigPatchRoot(UserConfig{ResetVision: true})

	var vision map[string]string
	if err := json.Unmarshal(root["vision"], &vision); err != nil {
		t.Fatalf("unmarshal vision patch: %v", err)
	}
	for _, key := range []string{"provider", "api_key", "base_url", "model"} {
		if vision[key] != "" {
			t.Fatalf("%s = %q, want empty string", key, vision[key])
		}
	}
}

func TestLoadConfigVisionDefaults(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("HOMEDRIVE", "")
	t.Setenv("HOMEPATH", "")
	t.Setenv("SESAME_VISION_PROVIDER", "")
	t.Setenv("SESAME_VISION_API_KEY", "")
	t.Setenv("SESAME_VISION_BASE_URL", "")
	t.Setenv("SESAME_VISION_MODEL", "")

	globalRoot := filepath.Join(home, DirName)
	if err := os.MkdirAll(globalRoot, 0o755); err != nil {
		t.Fatalf("mkdir global root: %v", err)
	}
	data := []byte(`{"vision":{"provider":"openai_compatible","api_key":"vision-key","model":"vision-model"}}`)
	if err := os.WriteFile(filepath.Join(globalRoot, "config.json"), data, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := ResolveCLIStartupConfig(CLIStartupOverrides{WorkspaceRoot: t.TempDir()})
	if err != nil {
		t.Fatalf("ResolveCLIStartupConfig returned error: %v", err)
	}
	if cfg.VisionProvider != "openai_compatible" {
		t.Fatalf("VisionProvider = %q, want openai_compatible", cfg.VisionProvider)
	}
	if cfg.VisionAPIKey != "vision-key" {
		t.Fatalf("VisionAPIKey = %q, want vision-key", cfg.VisionAPIKey)
	}
	if cfg.VisionBaseURL != "https://api.openai.com/v1" {
		t.Fatalf("VisionBaseURL = %q, want https://api.openai.com/v1", cfg.VisionBaseURL)
	}
	if cfg.VisionModel != "vision-model" {
		t.Fatalf("VisionModel = %q, want vision-model", cfg.VisionModel)
	}
	for _, field := range MissingSetupFields(cfg) {
		if field == "vision" || field == "vision.model" || field == "vision.api_key" {
			t.Fatalf("MissingSetupFields included optional vision field %q", field)
		}
	}
}
