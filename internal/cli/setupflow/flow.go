package setupflow

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mattn/go-isatty"

	"go-agent/internal/config"
	discordcfg "go-agent/internal/connectors/discord"
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
			return fmt.Errorf("sesame is not configured; run 'sesame setup' in an interactive terminal")
		}
		return fmt.Errorf("interactive input required for %s", strings.TrimSpace(action))
	}

	fileCfg, err := config.LoadUserConfig()
	if err != nil {
		return err
	}

	// Delegate to TUI wizard when running in a terminal.
	if isTerminalReaderWriter(r, w) {
		return runTUIWizard(cfg, fileCfg, action, missing)
	}

	// Fall back to plain-text flow for piped/non-terminal input.
	reader := bufio.NewReader(r)
	for {
		nextCfg, err := config.ResolveCLIStartupConfig(config.CLIStartupOverrides{
			DataDir:        cfg.DataDir,
			Addr:           cfg.Addr,
			Model:          cfg.Model,
			PermissionMode: cfg.PermissionProfile,
			WorkspaceRoot:  cfg.Paths.WorkspaceRoot,
		})
		if err == nil {
			cfg = nextCfg
		}
		fileCfg, err = config.LoadUserConfig()
		if err != nil {
			return err
		}

		choice, err := chooseHomeSection(reader, w, cfg, fileCfg, missing, configPath, action)
		if err != nil {
			if err == io.EOF && modelConfigured(cfg) {
				return nil
			}
			return err
		}

		switch choice {
		case homeModelSetup:
			state, err := collectModelSetupFlowState(reader, w, cfg, fileCfg, action, missing, configPath)
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
			fmt.Fprintln(w, "\nSaved config. Returning to configuration home...")
		case homeIntegrations:
			if err := runIntegrationsMenu(reader, w, cfg, fileCfg); err != nil {
				if err == io.EOF && modelConfigured(cfg) {
					return nil
				}
				return err
			}
		case homeContinue:
			if modelConfigured(cfg) || isSetupAction(action) {
				return nil
			}
			fmt.Fprintln(w, "Continue Startup is disabled until Model Setup is complete.")
		default:
			return fmt.Errorf("unknown configuration section: %q", choice)
		}

		nextCfg, err = config.ResolveCLIStartupConfig(config.CLIStartupOverrides{
			DataDir:        cfg.DataDir,
			Addr:           cfg.Addr,
			Model:          cfg.Model,
			PermissionMode: cfg.PermissionProfile,
			WorkspaceRoot:  cfg.Paths.WorkspaceRoot,
		})
		if err == nil {
			cfg = nextCfg
		}
		missing = config.MissingSetupFields(cfg)
	}
}

func isTerminalReaderWriter(r io.Reader, w io.Writer) bool {
	inFile, ok1 := r.(*os.File)
	outFile, ok2 := w.(*os.File)
	if !ok1 || !ok2 {
		return false
	}
	return isatty.IsTerminal(inFile.Fd()) && isatty.IsTerminal(outFile.Fd())
}

func runTUIWizard(cfg config.Config, fileCfg config.UserConfig, action string, missing []string) error {
	m := newSetupModel(cfg, fileCfg, action, missing)
	p := tea.NewProgram(m, tea.WithAltScreen())
	finalModel, err := p.Run()
	if errors.Is(err, tea.ErrProgramKilled) {
		return nil
	}
	if err != nil {
		return err
	}
	if final, ok := finalModel.(setupModel); ok && final.showSaved {
		return nil
	}
	if len(missing) > 0 && !modelConfigured(cfg) {
		return fmt.Errorf("setup cancelled before configuration was saved")
	}
	return nil
}

func chooseHomeSection(reader *bufio.Reader, w io.Writer, cfg config.Config, fileCfg config.UserConfig, missing []string, configPath, action string) (homeChoice, error) {
	fmt.Fprintln(w, "Configuration")
	fmt.Fprintf(w, "Config file: %s\n", configPath)
	fmt.Fprintf(w, "Model Setup: %s\n", modelSetupStatus(cfg))
	fmt.Fprintf(w, "Third-Party Integrations: %s\n", integrationsStatus(cfg, fileCfg))
	if len(missing) > 0 {
		fmt.Fprintf(w, "Missing fields: %s\n", strings.Join(missing, ", "))
	}
	choices := []string{
		"Model Setup (Required)",
		"Third-Party Integrations",
	}
	if isSetupAction(action) {
		choices = append(choices, "Save and Exit")
	} else if modelConfigured(cfg) {
		choices = append(choices, "Continue Startup")
	} else {
		choices = append(choices, "Continue Startup (disabled until Model Setup is complete)")
	}
	for _, choice := range choices {
		fmt.Fprintf(w, "- %s\n", choice)
	}
	idx, err := chooseArrowOption(reader, w, "Select section", choices, 0)
	if err != nil {
		return "", err
	}
	switch idx {
	case 0:
		return homeModelSetup, nil
	case 1:
		return homeIntegrations, nil
	case 2:
		return homeContinue, nil
	default:
		return "", fmt.Errorf("unknown home selection index: %d", idx)
	}
}

func runIntegrationsMenu(reader *bufio.Reader, w io.Writer, cfg config.Config, fileCfg config.UserConfig) error {
	fmt.Fprintln(w, "Third-Party Integrations")
	choices := []string{"Discord", "Vision Model", "Back"}
	idx, err := chooseArrowOption(reader, w, "Select integration", choices, 0)
	if err != nil {
		return err
	}
	switch idx {
	case 0:
		return runDiscordSetup(reader, w, cfg, fileCfg)
	case 1:
		return runVisionSetup(reader, w, cfg, fileCfg)
	case 2:
		return nil
	default:
		return fmt.Errorf("unknown integration selection index: %d", idx)
	}
}

func modelSetupStatus(cfg config.Config) string {
	if modelConfigured(cfg) {
		return "Configured"
	}
	return "Not Configured"
}

func integrationsStatus(cfg config.Config, fileCfg config.UserConfig) string {
	statuses := make([]string, 0, 2)
	if fileCfg.Discord.Enabled {
		workspaceRoot := strings.TrimSpace(cfg.Paths.WorkspaceRoot)
		if workspaceRoot == "" {
			statuses = append(statuses, "Discord Enabled")
		} else {
			binding, err := loadWorkspaceBinding(workspaceRoot)
			if err != nil {
				statuses = append(statuses, "Discord Config Error")
			} else if strings.TrimSpace(binding.GuildID) != "" && strings.TrimSpace(binding.ChannelID) != "" {
				statuses = append(statuses, "Discord Configured")
			} else {
				statuses = append(statuses, "Discord Partially Configured")
			}
		}
	}
	if strings.TrimSpace(fileCfg.Vision.Provider) != "" && strings.TrimSpace(fileCfg.Vision.Model) != "" {
		statuses = append(statuses, "Vision Configured")
	}
	if len(statuses) == 0 {
		return "Not Configured"
	}
	return strings.Join(statuses, ", ")
}

func loadWorkspaceBinding(workspaceRoot string) (struct{ GuildID, ChannelID string }, error) {
	binding, err := discordcfg.LoadWorkspaceBinding(workspaceRoot)
	if err != nil {
		return struct{ GuildID, ChannelID string }{}, err
	}
	return struct{ GuildID, ChannelID string }{
		GuildID:   binding.GuildID,
		ChannelID: binding.ChannelID,
	}, nil
}

func collectModelSetupFlowState(reader *bufio.Reader, w io.Writer, cfg config.Config, fileCfg config.UserConfig, action string, missing []string, configPath string) (flowState, error) {
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
