package cli

import (
	"bufio"
	"fmt"
	"io"
	"strings"

	"go-agent/internal/config"
)

func ensureRuntimeConfigured(stdin io.Reader, stdout io.Writer, cfg config.Config) error {
	configPath, _, err := config.EnsureUserConfigFile()
	if err != nil {
		return err
	}
	if _, _, err := config.EnsureCLIConfigFile(); err != nil {
		return err
	}

	missing := config.MissingSetupFields(cfg)
	if len(missing) == 0 {
		return nil
	}
	if stdin == nil {
		return fmt.Errorf("missing required configuration in %s: %s", configPath, strings.Join(missing, ", "))
	}

	fileCfg, err := config.LoadUserConfig()
	if err != nil {
		return err
	}

	reader := bufio.NewReader(stdin)
	fmt.Fprintf(stdout, "Sesame setup is required.\nConfig file: %s\n", configPath)
	fmt.Fprintf(stdout, "Missing fields: %s\n\n", strings.Join(missing, ", "))

	activeProfile := firstNonEmptyLocal(fileCfg.ActiveProfile, cfg.ActiveProfile, "default")
	currentProfile := fileCfg.Profiles[activeProfile]
	providerDefault := firstNonEmptyLocal(
		cfg.ModelProvider,
		providerTypeForProfile(fileCfg, activeProfile),
		"anthropic",
	)
	providerDefault = sanitizeInteractiveProvider(providerDefault)
	provider, err := promptRequired(reader, stdout, "Provider [anthropic/openai_compatible]", providerDefault)
	if err != nil {
		return err
	}
	for provider != "anthropic" && provider != "openai_compatible" {
		provider, err = promptRequired(reader, stdout, "Provider [anthropic/openai_compatible]", provider)
		if err != nil {
			return err
		}
	}

	modelDefault := firstNonEmptyLocal(currentProfile.Model, cfg.Model, defaultModelForProvider(provider))
	model, err := promptRequired(reader, stdout, "Model", modelDefault)
	if err != nil {
		return err
	}

	baseURL := ""
	apiKeyEnv := ""
	switch provider {
	case "openai_compatible":
		baseURL, err = promptRequired(reader, stdout, "OpenAI-compatible base URL", firstNonEmptyLocal(baseURLForProviderType(fileCfg, provider), cfg.OpenAIBaseURL, "https://api.openai.com/v1"))
		if err != nil {
			return err
		}
		apiKeyEnv, err = promptRequired(reader, stdout, "OpenAI-compatible API key env var", firstNonEmptyLocal(apiKeyEnvForProviderType(fileCfg, provider), "OPENAI_API_KEY"))
		if err != nil {
			return err
		}
	case "anthropic":
		baseURL, err = promptRequired(reader, stdout, "Anthropic base URL", firstNonEmptyLocal(baseURLForProviderType(fileCfg, provider), cfg.AnthropicBaseURL, "https://api.anthropic.com"))
		if err != nil {
			return err
		}
		apiKeyEnv, err = promptRequired(reader, stdout, "Anthropic API key env var", firstNonEmptyLocal(apiKeyEnvForProviderType(fileCfg, provider), "ANTHROPIC_API_KEY"))
		if err != nil {
			return err
		}
	}

	permissionProfile, err := promptRequired(reader, stdout, "Permission profile", firstNonEmptyLocal(fileCfg.PermissionProfile, cfg.PermissionProfile, "trusted_local"))
	if err != nil {
		return err
	}

	next := fileCfg
	if next.ModelProviders == nil {
		next.ModelProviders = make(map[string]config.UserConfigModelProvider)
	}
	if next.Profiles == nil {
		next.Profiles = make(map[string]config.UserConfigProfile)
	}
	providerID := providerIDForSetup(provider, currentProfile.ModelProvider)
	next.ModelProviders[providerID] = config.UserConfigModelProvider{
		APIFamily: apiFamilyForProvider(provider),
		BaseURL:   strings.TrimSpace(baseURL),
		APIKeyEnv: strings.TrimSpace(apiKeyEnv),
	}
	profile := currentProfile
	profile.Model = strings.TrimSpace(model)
	profile.ModelProvider = providerID
	profile.CacheProfile = cacheProfileForProvider(provider)

	next.ActiveProfile = activeProfile
	next.Profiles[activeProfile] = profile
	next.PermissionProfile = permissionProfile

	if err := config.WriteUserConfig(next); err != nil {
		return err
	}
	fmt.Fprintln(stdout, "\nSaved config. Continuing startup...")
	return nil
}

func promptRequired(reader *bufio.Reader, stdout io.Writer, label, defaultValue string) (string, error) {
	for {
		if strings.TrimSpace(defaultValue) != "" {
			fmt.Fprintf(stdout, "%s [%s]: ", label, defaultValue)
		} else {
			fmt.Fprintf(stdout, "%s: ", label)
		}
		line, err := reader.ReadString('\n')
		if err != nil && err != io.EOF {
			return "", err
		}
		value := strings.TrimSpace(line)
		if value == "" {
			value = strings.TrimSpace(defaultValue)
		}
		if value != "" {
			return value, nil
		}
		if err == io.EOF {
			return "", io.EOF
		}
	}
}

func firstNonEmptyLocal(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func providerTypeForProfile(cfg config.UserConfig, activeProfile string) string {
	profile, ok := cfg.Profiles[strings.TrimSpace(activeProfile)]
	if !ok {
		return ""
	}
	provider, ok := cfg.ModelProviders[strings.TrimSpace(profile.ModelProvider)]
	if !ok {
		return ""
	}
	return providerTypeFromAPIFamily(provider.APIFamily)
}

func baseURLForProviderType(cfg config.UserConfig, providerType string) string {
	for _, provider := range cfg.ModelProviders {
		if providerTypeFromAPIFamily(provider.APIFamily) == strings.TrimSpace(providerType) {
			return strings.TrimSpace(provider.BaseURL)
		}
	}
	return ""
}

func apiKeyEnvForProviderType(cfg config.UserConfig, providerType string) string {
	for _, provider := range cfg.ModelProviders {
		if providerTypeFromAPIFamily(provider.APIFamily) == strings.TrimSpace(providerType) {
			return strings.TrimSpace(provider.APIKeyEnv)
		}
	}
	return ""
}

func providerTypeFromAPIFamily(apiFamily string) string {
	switch strings.ToLower(strings.TrimSpace(apiFamily)) {
	case "anthropic_messages":
		return "anthropic"
	case "openai_responses":
		return "openai_compatible"
	default:
		return ""
	}
}

func defaultModelForProvider(provider string) string {
	switch strings.TrimSpace(provider) {
	case "openai_compatible":
		return "gpt-5.4"
	default:
		return "claude-sonnet-4-5"
	}
}

func apiFamilyForProvider(provider string) string {
	switch strings.TrimSpace(provider) {
	case "openai_compatible":
		return "openai_responses"
	default:
		return "anthropic_messages"
	}
}

func cacheProfileForProvider(provider string) string {
	switch strings.TrimSpace(provider) {
	case "openai_compatible":
		return "openai_responses"
	default:
		return "anthropic_default"
	}
}

func providerIDForSetup(provider, current string) string {
	if strings.TrimSpace(current) != "" {
		return strings.TrimSpace(current)
	}
	switch strings.TrimSpace(provider) {
	case "openai_compatible":
		return "openai"
	default:
		return "anthropic"
	}
}

func sanitizeInteractiveProvider(provider string) string {
	switch strings.TrimSpace(provider) {
	case "openai_compatible":
		return "openai_compatible"
	case "anthropic":
		return "anthropic"
	default:
		return "anthropic"
	}
}
