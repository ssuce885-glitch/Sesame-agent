package engine

import (
	contextstate "go-agent/internal/context"
	"go-agent/internal/model"
)

type SummaryBundle = contextstate.SummaryBundle

func selectPromptSummaryBundle(sessionMemory *model.Summary, boundary *model.Summary, rolling []model.Summary) SummaryBundle {
	if rollingSummaryMaxCount > 0 && len(rolling) > rollingSummaryMaxCount {
		rolling = rolling[len(rolling)-rollingSummaryMaxCount:]
	}
	return SummaryBundle{
		SessionMemory: normalizeOptionalSummary(sessionMemory, sessionMemorySummaryTokenBudget),
		Boundary:      normalizeOptionalSummary(boundary, boundarySummaryTokenBudget),
		Rolling:       takeSummaryBudget(rolling, rollingSummaryTokenBudget, rollingSummaryMaxCount),
	}
}

func flattenSummaryBundle(bundle SummaryBundle) []model.Summary {
	out := make([]model.Summary, 0, 2+len(bundle.Rolling))
	if bundle.SessionMemory != nil {
		out = append(out, *bundle.SessionMemory)
	}
	if bundle.Boundary != nil {
		out = append(out, *bundle.Boundary)
	}
	out = append(out, bundle.Rolling...)
	return out
}

func normalizeOptionalSummary(summary *model.Summary, tokenBudget int) *model.Summary {
	if summary == nil {
		return nil
	}
	normalized := takeSummaryBudget([]model.Summary{*summary}, tokenBudget, 1)
	if len(normalized) == 0 {
		return nil
	}
	value := normalized[0]
	return &value
}
