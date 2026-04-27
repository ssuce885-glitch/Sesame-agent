package setupflow

import (
	"bufio"
	"fmt"
	"io"
	"strings"

	"go-agent/internal/config"
)

type visionSetupState struct {
	Provider string
	APIKey   string
	BaseURL  string
	Model    string
}

func runVisionSetup(reader *bufio.Reader, w io.Writer, cfg config.Config, fileCfg config.UserConfig) error {
	state, err := collectVisionSetupState(reader, w, cfg, fileCfg)
	if err != nil {
		return err
	}

	if err := config.MergeAndWriteUserConfig(buildVisionConfigPatches(state)); err != nil {
		return err
	}

	if strings.TrimSpace(state.Provider) == "" {
		fmt.Fprintln(w, "Vision model configuration cleared. Returning to configuration home...")
	} else {
		fmt.Fprintln(w, "Saved Vision Model configuration. Returning to configuration home...")
	}
	return nil
}

func collectVisionSetupState(reader *bufio.Reader, w io.Writer, cfg config.Config, fileCfg config.UserConfig) (visionSetupState, error) {
	state := defaultVisionSetupState(cfg, fileCfg)

	fmt.Fprintln(w, "Vision Model Setup")
	fmt.Fprintf(w, "Status: %s\n", visionSetupStatus(state))

	enabled, err := readBoolChoice(reader, w, "Enable Vision Model", "Enabled", "Disabled", visionHasAnyConfig(state))
	if err != nil {
		return visionSetupState{}, err
	}
	if !enabled {
		return visionSetupState{}, nil
	}

	providerIdx, err := chooseArrowOption(reader, w, "Vision provider", []string{"Anthropic", "OpenAI-compatible"}, defaultVisionProviderIndex(state.Provider))
	if err != nil {
		return visionSetupState{}, err
	}
	if providerIdx == 0 {
		state.Provider = "anthropic"
	} else {
		state.Provider = "openai_compatible"
	}

	state.APIKey, err = readSecretInput(reader, w, visionAPIKeyLabel(state.Provider), state.APIKey)
	if err != nil {
		return visionSetupState{}, err
	}
	state.BaseURL, err = readTextInput(reader, w, visionBaseURLLabel(state.Provider), firstNonEmpty(state.BaseURL, visionSetupDefaultBaseURL(state.Provider)))
	if err != nil {
		return visionSetupState{}, err
	}
	state.Model, err = readTextInput(reader, w, "Vision Model", state.Model)
	if err != nil {
		return visionSetupState{}, err
	}

	return state, nil
}

func buildVisionConfigPatches(state visionSetupState) config.UserConfig {
	if strings.TrimSpace(state.Provider) == "" {
		return config.UserConfig{ResetVision: true}
	}
	return config.UserConfig{
		Vision: config.UserConfigVision{
			Provider: strings.TrimSpace(state.Provider),
			APIKey:   strings.TrimSpace(state.APIKey),
			BaseURL:  strings.TrimSpace(state.BaseURL),
			Model:    strings.TrimSpace(state.Model),
		},
		ResetVision: true,
	}
}

func defaultVisionSetupState(cfg config.Config, fileCfg config.UserConfig) visionSetupState {
	return visionSetupState{
		Provider: firstNonEmpty(fileCfg.Vision.Provider, cfg.VisionProvider),
		APIKey:   firstNonEmpty(fileCfg.Vision.APIKey, cfg.VisionAPIKey),
		BaseURL:  firstNonEmpty(fileCfg.Vision.BaseURL, cfg.VisionBaseURL),
		Model:    firstNonEmpty(fileCfg.Vision.Model, cfg.VisionModel),
	}
}

func visionSetupStatus(state visionSetupState) string {
	if strings.TrimSpace(state.Provider) != "" && strings.TrimSpace(state.Model) != "" {
		return "Configured"
	}
	return "Not Configured"
}

func visionHasAnyConfig(state visionSetupState) bool {
	return strings.TrimSpace(state.Provider) != "" ||
		strings.TrimSpace(state.APIKey) != "" ||
		strings.TrimSpace(state.BaseURL) != "" ||
		strings.TrimSpace(state.Model) != ""
}

func defaultVisionProviderIndex(provider string) int {
	if strings.TrimSpace(provider) == "openai_compatible" {
		return 1
	}
	return 0
}

func visionAPIKeyLabel(provider string) string {
	if strings.TrimSpace(provider) == "openai_compatible" {
		return "OpenAI-compatible API key"
	}
	return "Anthropic API key"
}

func visionBaseURLLabel(provider string) string {
	if strings.TrimSpace(provider) == "openai_compatible" {
		return "OpenAI-compatible base URL"
	}
	return "Anthropic base URL"
}

func visionSetupDefaultBaseURL(provider string) string {
	switch strings.TrimSpace(provider) {
	case "anthropic":
		return "https://api.anthropic.com"
	case "openai_compatible":
		return "https://api.openai.com/v1"
	default:
		return ""
	}
}
