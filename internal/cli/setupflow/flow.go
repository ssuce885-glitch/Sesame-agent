package setupflow

import (
	"bufio"
	"fmt"
	"io"
	"strings"

	"go-agent/internal/config"
)

// Run executes the lightweight setup wizard for startup/setup/configure flows.
func Run(r io.Reader, w io.Writer, cfg config.Config, action string) error {
	configPath, _, err := config.EnsureUserConfigFile()
	if err != nil {
		return err
	}
	if _, _, err := config.EnsureCLIConfigFile(); err != nil {
		return err
	}

	missing := config.MissingSetupFields(cfg)
	explicitSetup := isSetupAction(action)
	if len(missing) == 0 && !explicitSetup {
		return nil
	}
	if r == nil {
		if len(missing) > 0 {
			return fmt.Errorf("Sesame is not configured. Run 'sesame setup' in an interactive terminal.")
		}
		return fmt.Errorf("interactive input required for %s", strings.TrimSpace(action))
	}

	fileCfg, err := config.LoadUserConfig()
	if err != nil {
		return err
	}

	state, err := collectFlowState(bufio.NewReader(r), w, cfg, fileCfg, action, missing, configPath)
	if err != nil {
		return err
	}

	patch := config.UserConfig{
		Provider:          state.provider,
		Model:             state.model,
		PermissionProfile: state.permissionProfile,
		Listen: config.UserConfigListen{
			Addr: state.listenAddr,
		},
	}
	switch state.provider {
	case "openai_compatible":
		patch.OpenAI = config.UserConfigOpenAI{
			APIKey:  state.apiKey,
			BaseURL: state.baseURL,
			Model:   state.model,
		}
	case "anthropic":
		patch.Anthropic = config.UserConfigAnthropic{
			APIKey:  state.apiKey,
			BaseURL: state.baseURL,
			Model:   state.model,
		}
	case "fake":
		patch.ResetOpenAI = true
		patch.ResetAnthropic = true
	}

	if err := config.MergeAndWriteUserConfig(patch); err != nil {
		return err
	}
	fmt.Fprintln(w, "\nSaved config. Continuing startup...")
	return nil
}

func collectFlowState(reader *bufio.Reader, w io.Writer, cfg config.Config, fileCfg config.UserConfig, action string, missing []string, configPath string) (flowState, error) {
	state := flowState{
		action:        strings.TrimSpace(action),
		missingFields: missing,
	}

	fmt.Fprintf(w, "Sesame setup\nConfig file: %s\n", configPath)
	if len(missing) > 0 {
		fmt.Fprintf(w, "Missing fields: %s\n", strings.Join(missing, ", "))
	}
	fmt.Fprintln(w, "Choose a vendor to configure.")

	vendors := defaultVendors()
	labels := make([]string, 0, len(vendors))
	for _, vendor := range vendors {
		labels = append(labels, vendor.label)
	}

	providerDefault := firstNonEmpty(fileCfg.Provider, cfg.ModelProvider)
	vendorIdx, err := chooseArrowOption(reader, w, "Select vendor", labels, defaultVendorIndex(providerDefault))
	if err != nil {
		return flowState{}, err
	}
	state.vendor = vendors[vendorIdx]

	compat := state.vendor.compat
	if state.vendor.key == "custom" {
		compatIdx, err := chooseArrowOption(reader, w, "Custom compatibility", []string{"Anthropic-compatible", "OpenAI-compatible"}, 0)
		if err != nil {
			return flowState{}, err
		}
		if compatIdx == 0 {
			compat = "anthropic"
		} else {
			compat = "openai"
		}
	}

	provider, err := providerForVendor(state.vendor.key, compat)
	if err != nil {
		return flowState{}, err
	}
	state.provider = provider

	switch provider {
	case "openai_compatible":
		state.apiKey, err = readSecretInput(reader, w, "OpenAI-compatible API key", firstNonEmpty(fileCfg.OpenAI.APIKey, cfg.OpenAIAPIKey))
		if err != nil {
			return flowState{}, err
		}
	case "anthropic":
		state.apiKey, err = readSecretInput(reader, w, "Anthropic API key", firstNonEmpty(fileCfg.Anthropic.APIKey, cfg.AnthropicAPIKey))
		if err != nil {
			return flowState{}, err
		}
	}

	state.model, err = readTextInput(reader, w, "Model", firstNonEmpty(fileCfg.Model, cfg.Model, state.vendor.defaultModel))
	if err != nil {
		return flowState{}, err
	}

	switch provider {
	case "openai_compatible":
		state.baseURL, err = readTextInput(reader, w, "OpenAI-compatible base URL", firstNonEmpty(fileCfg.OpenAI.BaseURL, cfg.OpenAIBaseURL, state.vendor.defaultBaseURL, "https://api.openai.com/v1"))
		if err != nil {
			return flowState{}, err
		}
	case "anthropic":
		state.baseURL, err = readTextInput(reader, w, "Anthropic base URL", firstNonEmpty(fileCfg.Anthropic.BaseURL, cfg.AnthropicBaseURL, state.vendor.defaultBaseURL, "https://api.anthropic.com"))
		if err != nil {
			return flowState{}, err
		}
	}

	state.permissionProfile, err = readTextInput(reader, w, "Permission profile", firstNonEmpty(fileCfg.PermissionProfile, cfg.PermissionProfile, "trusted_local"))
	if err != nil {
		return flowState{}, err
	}
	state.listenAddr, err = readTextInput(reader, w, "Listen addr", firstNonEmpty(fileCfg.Listen.Addr, cfg.Addr, "127.0.0.1:4317"))
	if err != nil {
		return flowState{}, err
	}

	return state, nil
}
