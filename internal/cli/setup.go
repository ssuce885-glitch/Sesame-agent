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

	provider, err := promptRequired(reader, stdout, "Provider [anthropic/openai_compatible/fake]", firstNonEmptyLocal(fileCfg.Provider, cfg.ModelProvider))
	if err != nil {
		return err
	}
	for provider != "anthropic" && provider != "openai_compatible" && provider != "fake" {
		provider, err = promptRequired(reader, stdout, "Provider [anthropic/openai_compatible/fake]", provider)
		if err != nil {
			return err
		}
	}

	modelDefault := firstNonEmptyLocal(fileCfg.Model, cfg.Model)
	model, err := promptRequired(reader, stdout, "Model", modelDefault)
	if err != nil {
		return err
	}

	next := fileCfg
	next.Provider = provider
	next.Model = model

	switch provider {
	case "openai_compatible":
		baseURL, err := promptRequired(reader, stdout, "OpenAI-compatible base URL", firstNonEmptyLocal(fileCfg.OpenAI.BaseURL, cfg.OpenAIBaseURL))
		if err != nil {
			return err
		}
		apiKey, err := promptSecretRequired(reader, stdout, "OpenAI-compatible API key", firstNonEmptyLocal(fileCfg.OpenAI.APIKey, cfg.OpenAIAPIKey))
		if err != nil {
			return err
		}
		next.OpenAI.BaseURL = baseURL
		next.OpenAI.APIKey = apiKey
		next.OpenAI.Model = model
	case "anthropic":
		baseURL, err := promptRequired(reader, stdout, "Anthropic base URL", firstNonEmptyLocal(fileCfg.Anthropic.BaseURL, cfg.AnthropicBaseURL, "https://api.anthropic.com"))
		if err != nil {
			return err
		}
		apiKey, err := promptSecretRequired(reader, stdout, "Anthropic API key", firstNonEmptyLocal(fileCfg.Anthropic.APIKey, cfg.AnthropicAPIKey))
		if err != nil {
			return err
		}
		next.Anthropic.BaseURL = baseURL
		next.Anthropic.APIKey = apiKey
		next.Anthropic.Model = model
	case "fake":
		next.OpenAI = config.UserConfigOpenAI{}
		next.Anthropic.APIKey = ""
		next.Anthropic.Model = ""
	}
	permissionProfile, err := promptRequired(reader, stdout, "Permission profile", firstNonEmptyLocal(fileCfg.PermissionProfile, cfg.PermissionProfile, "trusted_local"))
	if err != nil {
		return err
	}
	next.PermissionProfile = permissionProfile

	if err := config.WriteUserConfig(next); err != nil {
		return err
	}
	fmt.Fprintln(stdout, "\nSaved config. Continuing startup...")
	return nil
}

func promptRequired(reader *bufio.Reader, stdout io.Writer, label, defaultValue string) (string, error) {
	return promptValue(reader, stdout, label, defaultValue, defaultValue)
}

func promptSecretRequired(reader *bufio.Reader, stdout io.Writer, label, defaultValue string) (string, error) {
	display := ""
	if strings.TrimSpace(defaultValue) != "" {
		display = "your-key"
	}
	return promptValue(reader, stdout, label, display, defaultValue)
}

func promptValue(reader *bufio.Reader, stdout io.Writer, label, displayValue, actualDefault string) (string, error) {
	for {
		if strings.TrimSpace(displayValue) != "" {
			fmt.Fprintf(stdout, "%s [%s]: ", label, displayValue)
		} else {
			fmt.Fprintf(stdout, "%s: ", label)
		}
		line, err := reader.ReadString('\n')
		if err != nil && err != io.EOF {
			return "", err
		}
		value := strings.TrimSpace(line)
		if value == "" {
			value = strings.TrimSpace(actualDefault)
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
