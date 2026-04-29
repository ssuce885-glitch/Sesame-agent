package suites

import (
	"fmt"
	"strings"

	"go-agent/cmd/eval/internal/evalcore"
)

const recallCodeword = "ZEBRA-42"

func RecallSuite(opts evalcore.SuiteOptions) evalcore.EvalSuite {
	fillerTurns := 19
	if opts.Quick {
		fillerTurns = 4
	}
	if opts.Long {
		fillerTurns = 100
	}

	turns := []evalcore.EvalTurn{
		{
			Message: "Please store this exact durable fact for future turns: The secret project codename is ZEBRA-42. Reply with a short acknowledgement.",
		},
	}
	for idx := 1; idx <= fillerTurns; idx++ {
		turns = append(turns, evalcore.EvalTurn{
			Message: recallFillerPrompt(idx),
		})
	}
	turns = append(turns, evalcore.EvalTurn{
		Message: "What was the secret project codename? Answer with only the codename.",
		Validate: func(response evalcore.EvalResponse) []evalcore.EvalResult {
			text := strings.ToUpper(response.AssistantText)
			return []evalcore.EvalResult{
				evalcore.Result(
					"recall codeword after many turns",
					strings.Contains(text, recallCodeword),
					fmt.Sprintf("assistant_text=%q", strings.TrimSpace(response.AssistantText)),
				),
			}
		},
	})

	return evalcore.EvalSuite{
		Name:        "recall",
		Description: "Checks that a fact seeded on turn 1 is still retrievable after many unrelated turns.",
		Turns:       turns,
		MinPassRate: 1.0,
	}
}

func recallFillerPrompt(idx int) string {
	topics := []string{
		"Give a two sentence explanation of why database indexes matter.",
		"List three practical risks in a deployment checklist.",
		"Summarize the tradeoff between latency and throughput.",
		"Describe one way to make logs easier to search.",
		"Explain how to keep a small project README useful.",
	}
	return fmt.Sprintf("Unrelated discussion %02d: %s Do not mention any project codenames.", idx, topics[(idx-1)%len(topics)])
}
