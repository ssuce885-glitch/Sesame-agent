package engine

import (
	"strings"

	"go-agent/internal/memory"
	"go-agent/internal/model"
)

func cloneSummaryForSessionMemory(summary model.Summary) model.Summary {
	cloned := model.Summary{
		RangeLabel:       summary.RangeLabel,
		UserGoals:        append([]string(nil), summary.UserGoals...),
		ImportantChoices: append([]string(nil), summary.ImportantChoices...),
		FilesTouched:     append([]string(nil), summary.FilesTouched...),
		ToolOutcomes:     append([]string(nil), summary.ToolOutcomes...),
		OpenThreads:      append([]string(nil), summary.OpenThreads...),
	}
	if strings.TrimSpace(cloned.RangeLabel) == "" {
		cloned.RangeLabel = sessionMemoryRangeLabel
	}
	return cloned
}

func prependSessionMemorySummary(summaries []model.Summary, summary model.Summary) []model.Summary {
	out := make([]model.Summary, 0, len(summaries)+1)
	out = append(out, cloneSummaryForSessionMemory(summary))
	for _, existing := range summaries {
		out = append(out, cloneSummaryForSessionMemory(existing))
	}
	return out
}

func dedupeSummaries(summaries []model.Summary) []model.Summary {
	if len(summaries) <= 1 {
		return summaries
	}

	seen := make(map[string]struct{}, len(summaries))
	out := make([]model.Summary, 0, len(summaries))
	for _, summary := range summaries {
		normalized := normalizeSummaryForPrompt(summary)
		key := encodeSessionMemorySummary(normalized)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, normalized)
	}
	return out
}

func normalizeSummaryForPrompt(summary model.Summary) model.Summary {
	normalized := cloneSummaryForSessionMemory(summary)
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

func selectPromptSummaries(summaries []model.Summary, sessionMemoryPresent bool) SummaryBundle {
	summaries = dedupeSummaries(summaries)
	if len(summaries) == 0 {
		return SummaryBundle{}
	}

	var sessionMemory *model.Summary
	start := 0
	if sessionMemoryPresent {
		if selected := takeSummaryBudget(summaries[:1], sessionMemorySummaryTokenBudget, sessionMemorySummaryMaxCount); len(selected) > 0 {
			value := selected[0]
			sessionMemory = &value
		}
		start = 1
	}

	var boundary *model.Summary
	if start < len(summaries) {
		value := summaries[start]
		boundary = &value
		start++
	}

	rolling := summaries[start:]
	return selectPromptSummaryBundle(sessionMemory, boundary, rolling)
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

func removeMatchingSummary(summaries []model.Summary, target model.Summary) []model.Summary {
	if len(summaries) == 0 {
		return nil
	}

	targetKey := encodeSessionMemorySummary(normalizeSummaryForPrompt(target))
	out := make([]model.Summary, 0, len(summaries))
	for _, summary := range summaries {
		summaryKey := encodeSessionMemorySummary(normalizeSummaryForPrompt(summary))
		if summaryKey == targetKey {
			continue
		}
		out = append(out, summary)
	}
	return out
}

func isZeroSummary(summary model.Summary) bool {
	return strings.TrimSpace(summary.RangeLabel) == "" &&
		len(summary.UserGoals) == 0 &&
		len(summary.ImportantChoices) == 0 &&
		len(summary.FilesTouched) == 0 &&
		len(summary.ToolOutcomes) == 0 &&
		len(summary.OpenThreads) == 0
}
