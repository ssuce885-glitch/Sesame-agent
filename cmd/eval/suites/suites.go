package suites

import (
	"strings"

	"go-agent/cmd/eval/internal/evalcore"
)

func All(opts evalcore.SuiteOptions) []evalcore.EvalSuite {
	return []evalcore.EvalSuite{
		RecallSuite(opts),
		CompressionSuite(opts),
		RoleDriftSuite(opts),
		InjectionSuite(opts),
		RecoverySuite(opts),
		StaleMemorySuite(opts),
	}
}

func hasTool(response evalcore.EvalResponse, name string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	for _, tool := range response.ToolCalls {
		if strings.ToLower(strings.TrimSpace(tool)) == name {
			return true
		}
	}
	return false
}

func hasAnyTool(response evalcore.EvalResponse, names ...string) bool {
	for _, name := range names {
		if hasTool(response, name) {
			return true
		}
	}
	return false
}

func containsFold(text, needle string) bool {
	return strings.Contains(strings.ToLower(text), strings.ToLower(needle))
}

func containsAnyFold(text string, needles ...string) bool {
	for _, needle := range needles {
		if containsFold(text, needle) {
			return true
		}
	}
	return false
}
