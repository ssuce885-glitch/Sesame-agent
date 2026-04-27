package setupflow

import (
	"strings"

	"go-agent/internal/config"
)

type vendorOption struct {
	key            string
	label          string
	compat         string
	defaultModel   string
	defaultBaseURL string
}

type homeChoice string

const (
	homeModelSetup   homeChoice = "model_setup"
	homeIntegrations homeChoice = "integrations"
	homeContinue     homeChoice = "continue"
)

type integrationChoice string

const (
	integrationDiscord integrationChoice = "discord"
	integrationVision  integrationChoice = "vision"
)

type flowState struct {
	action            string
	missingFields     []string
	vendor            vendorOption
	provider          string
	model             string
	apiKey            string
	baseURL           string
	permissionProfile string
	listenAddr        string
}

func isSetupAction(action string) bool {
	switch strings.ToLower(strings.TrimSpace(action)) {
	case "setup", "configure":
		return true
	default:
		return false
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func modelConfigured(cfg config.Config) bool {
	return len(config.MissingSetupFields(cfg)) == 0
}

func defaultVendors() []vendorOption {
	return []vendorOption{
		{
			key:            "anthropic",
			label:          "Anthropic",
			defaultModel:   "claude-sonnet-4-5",
			defaultBaseURL: "https://api.anthropic.com",
		},
		{
			key:            "openai",
			label:          "OpenAI-compatible",
			defaultModel:   "gpt-5.4",
			defaultBaseURL: "https://api.openai.com/v1",
		},
		{
			key:            "minimax",
			label:          "MiniMax (Anthropic compatible)",
			defaultModel:   "minimax-m1",
			defaultBaseURL: "https://api.minimax.chat",
		},
		{
			key:            "volcengine",
			label:          "Volcengine (OpenAI compatible)",
			defaultModel:   "doubao-seed-1-6-250615",
			defaultBaseURL: "https://ark.cn-beijing.volces.com/api/v3",
		},
		{
			key:          "fake",
			label:        "Fake (local smoke)",
			defaultModel: "fake-smoke",
		},
		{
			key:   "custom",
			label: "Custom (choose compatibility)",
		},
	}
}

func defaultVendorIndex(provider string) int {
	switch strings.TrimSpace(provider) {
	case "openai_compatible":
		return 1
	case "fake":
		return 4
	default:
		return 0
	}
}
