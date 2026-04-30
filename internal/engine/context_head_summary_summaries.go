package engine

import (
	"strings"

	"go-agent/internal/memory"
	"go-agent/internal/model"
)

func cloneSummaryForContextHeadSummary(summary model.Summary) model.Summary {
	cloned := model.Summary{
		RangeLabel:       summary.RangeLabel,
		UserGoals:        append([]string(nil), summary.UserGoals...),
		ImportantChoices: append([]string(nil), summary.ImportantChoices...),
		FilesTouched:     append([]string(nil), summary.FilesTouched...),
		ToolOutcomes:     append([]string(nil), summary.ToolOutcomes...),
		OpenThreads:      append([]string(nil), summary.OpenThreads...),
	}
	if strings.TrimSpace(cloned.RangeLabel) == "" {
		cloned.RangeLabel = contextHeadSummaryRangeLabel
	}
	return cloned
}

func normalizeSummaryForPrompt(summary model.Summary) model.Summary {
	normalized := cloneSummaryForContextHeadSummary(summary)
	normalized.RangeLabel = strings.TrimSpace(normalized.RangeLabel)
	normalized.UserGoals = dedupeSummaryStrings(normalized.UserGoals)
	normalized.ImportantChoices = dedupeSummaryStrings(normalized.ImportantChoices)
	normalized.FilesTouched = dedupeSummaryStrings(normalized.FilesTouched)
	normalized.ToolOutcomes = dedupeSummaryStrings(normalized.ToolOutcomes)
	normalized.OpenThreads = dedupeSummaryStrings(normalized.OpenThreads)
	return normalized
}

func dedupeSummaryStrings(values []string) []string {
	if len(values) <= 1 {
		return values
	}

	type semanticValue struct {
		text   string
		tokens map[string]struct{}
	}

	out := make([]semanticValue, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}

		candidate := semanticValue{
			text:   trimmed,
			tokens: tokenSet(memory.SemanticTerms(trimmed)),
		}

		merged := false
		for i := range out {
			if !semanticallyEquivalentStrings(out[i].text, out[i].tokens, candidate.text, candidate.tokens) {
				continue
			}
			if summaryStringSpecificity(candidate.text, candidate.tokens) > summaryStringSpecificity(out[i].text, out[i].tokens) {
				out[i] = candidate
			}
			merged = true
			break
		}
		if !merged {
			out = append(out, candidate)
		}
	}

	result := make([]string, 0, len(out))
	for _, value := range out {
		result = append(result, value.text)
	}
	return result
}

func takeSummaryBudget(summaries []model.Summary, tokenBudget int, maxCount int) []model.Summary {
	if len(summaries) == 0 || maxCount == 0 {
		return nil
	}

	out := make([]model.Summary, 0, minInt(len(summaries), maxCount))
	usedTokens := 0
	for _, summary := range summaries {
		normalized := normalizeSummaryForPrompt(summary)
		cost := estimateSummaryInjectionTokens(normalized)
		if len(out) > 0 && tokenBudget > 0 && usedTokens+cost > tokenBudget {
			break
		}
		out = append(out, normalized)
		usedTokens += cost
		if len(out) >= maxCount {
			break
		}
	}
	return out
}

func tokenSet(values []string) map[string]struct{} {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]struct{}, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			continue
		}
		out[value] = struct{}{}
	}
	return out
}

func semanticallyEquivalentStrings(a string, aTokens map[string]struct{}, b string, bTokens map[string]struct{}) bool {
	aNorm := strings.ToLower(strings.TrimSpace(a))
	bNorm := strings.ToLower(strings.TrimSpace(b))
	if aNorm == bNorm {
		return true
	}
	if len(aTokens) == 0 || len(bTokens) == 0 {
		return false
	}
	intersection := 0
	for token := range aTokens {
		if _, ok := bTokens[token]; ok {
			intersection++
		}
	}
	if intersection == 0 {
		return false
	}
	smaller := len(aTokens)
	if len(bTokens) < smaller {
		smaller = len(bTokens)
	}
	if smaller >= 2 && intersection == smaller {
		return true
	}
	return smaller >= 3 && float64(intersection)/float64(smaller) >= 0.85
}

func summaryStringSpecificity(text string, tokens map[string]struct{}) int {
	return len(tokens)*100 + len([]rune(text))
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func isZeroSummary(summary model.Summary) bool {
	return strings.TrimSpace(summary.RangeLabel) == "" &&
		len(summary.UserGoals) == 0 &&
		len(summary.ImportantChoices) == 0 &&
		len(summary.FilesTouched) == 0 &&
		len(summary.ToolOutcomes) == 0 &&
		len(summary.OpenThreads) == 0
}
