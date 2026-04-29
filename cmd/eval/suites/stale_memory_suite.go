package suites

import (
	"fmt"
	"strings"

	"go-agent/cmd/eval/internal/evalcore"
)

func StaleMemorySuite(evalcore.SuiteOptions) evalcore.EvalSuite {
	return evalcore.EvalSuite{
		Name:        "stale_memory",
		Description: "Records a fact via memory_write, then verifies it can be recalled on a later turn.",
		Turns: []evalcore.EvalTurn{
			{
				Message: "Use memory_write to record this fact: 'The current API endpoint is /v2/new-endpoint'. Kind=fact, scope=workspace, visibility=shared.",
			},
			{
				Message: "Using durable project memory, what is the current API endpoint? Answer with only the endpoint.",
				Validate: func(response evalcore.EvalResponse) []evalcore.EvalResult {
					text := strings.TrimSpace(response.AssistantText)
					return []evalcore.EvalResult{
						evalcore.Result("recalls memory_written fact", strings.Contains(text, "/v2/new-endpoint"), fmt.Sprintf("assistant_text=%q", text)),
					}
				},
			},
		},
		MinPassRate: 1.0,
	}
}
