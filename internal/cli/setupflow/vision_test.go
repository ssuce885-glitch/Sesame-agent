package setupflow

import (
	"bufio"
	"bytes"
	"strings"
	"testing"

	"go-agent/internal/config"
)

func TestCollectVisionSetupConfiguresOpenAICompatible(t *testing.T) {
	input := strings.Join([]string{
		"1",
		"2",
		"vision-key",
		"",
		"vision-model",
	}, "\n") + "\n"
	var out bytes.Buffer

	state, err := collectVisionSetupState(
		bufio.NewReader(strings.NewReader(input)),
		&out,
		config.Config{},
		config.UserConfig{},
	)
	if err != nil {
		t.Fatalf("collectVisionSetupState returned error: %v", err)
	}
	if state.Provider != "openai_compatible" {
		t.Fatalf("Provider = %q, want openai_compatible", state.Provider)
	}
	if state.APIKey != "vision-key" {
		t.Fatalf("APIKey = %q, want vision-key", state.APIKey)
	}
	if state.BaseURL != "https://api.openai.com/v1" {
		t.Fatalf("BaseURL = %q, want https://api.openai.com/v1", state.BaseURL)
	}
	if state.Model != "vision-model" {
		t.Fatalf("Model = %q, want vision-model", state.Model)
	}
	if !strings.Contains(out.String(), "Vision Model Setup") {
		t.Fatalf("output did not include Vision Model Setup title:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "Status: Not Configured") {
		t.Fatalf("output did not include Not Configured status:\n%s", out.String())
	}
}

func TestBuildVisionConfigPatchesResetWhenDisabled(t *testing.T) {
	patch := buildVisionConfigPatches(visionSetupState{})
	if !patch.ResetVision {
		t.Fatal("ResetVision = false, want true")
	}
	if patch.Vision != (config.UserConfigVision{}) {
		t.Fatalf("Vision = %+v, want empty", patch.Vision)
	}
}

func TestIntegrationsStatusVisionConfigured(t *testing.T) {
	got := integrationsStatus(config.Config{}, config.UserConfig{
		Vision: config.UserConfigVision{
			Provider: "anthropic",
			Model:    "vision-model",
		},
	})
	if got != "Vision Configured" {
		t.Fatalf("integrationsStatus = %q, want Vision Configured", got)
	}
}
