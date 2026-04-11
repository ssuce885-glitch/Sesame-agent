package config

import (
	"net/url"
	"strings"
)

const (
	compatModeAnthropic = "anthropic"
	compatModeOpenAI    = "openai"
)

func resolveModelProvider(explicitProvider, compatMode, genericBaseURL string) string {
	if provider := strings.TrimSpace(explicitProvider); provider != "" {
		return provider
	}
	if provider := compatModeToModelProvider(compatMode); provider != "" {
		return provider
	}
	if provider := inferModelProviderFromBaseURL(genericBaseURL); provider != "" {
		return provider
	}
	return ""
}

func compatModeToModelProvider(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "anthropic", "anthropic_messages", "anthropic-messages":
		return "anthropic"
	case "openai", "openai_compatible", "openai-compatible", "openai_responses", "openai_responses_compatible", "openai-responses":
		return "openai_compatible"
	case "fake":
		return "fake"
	default:
		return ""
	}
}

func inferModelProviderFromBaseURL(raw string) string {
	baseURL := strings.TrimSpace(raw)
	if baseURL == "" {
		return ""
	}
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return ""
	}

	host := strings.ToLower(strings.TrimSpace(parsed.Hostname()))
	path := strings.ToLower(strings.TrimSpace(parsed.Path))

	switch {
	case strings.Contains(host, "minimaxi"):
		return "anthropic"
	case strings.Contains(path, "/anthropic"):
		return "anthropic"
	case strings.Contains(host, "anthropic.com"):
		return "anthropic"
	case strings.Contains(host, "volces.com"):
		return "openai_compatible"
	case strings.Contains(host, "openai.com"):
		return "openai_compatible"
	case strings.Contains(path, "/responses"):
		return "openai_compatible"
	default:
		return ""
	}
}

func selectedProviderBaseURL(useGeneric bool, genericBaseURL, providerBaseURL, fallback string) string {
	values := []string{providerBaseURL}
	if useGeneric {
		values = append([]string{genericBaseURL}, values...)
	}
	values = append(values, fallback)
	return firstNonEmpty(values...)
}

func selectedProviderAPIKey(useGeneric bool, genericAPIKey, providerAPIKey string) string {
	values := []string{providerAPIKey}
	if useGeneric {
		values = append([]string{genericAPIKey}, values...)
	}
	return firstNonEmpty(values...)
}
