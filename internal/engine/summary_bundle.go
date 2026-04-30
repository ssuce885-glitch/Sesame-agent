package engine

import (
	contextstate "go-agent/internal/context"
	"go-agent/internal/model"
)

type SummaryBundle = contextstate.SummaryBundle

func selectPromptSummaryBundle(contextHeadSummary *model.Summary, boundary *model.Summary, rolling []model.Summary) SummaryBundle {
	if rollingSummaryMaxCount > 0 && len(rolling) > rollingSummaryMaxCount {
		rolling = rolling[len(rolling)-rollingSummaryMaxCount:]
	}
	return SummaryBundle{
		ContextHeadSummary: normalizeOptionalSummary(contextHeadSummary, contextHeadSummaryTokenBudget),
		Boundary:           normalizeOptionalSummary(boundary, boundarySummaryTokenBudget),
		Rolling:            takeSummaryBudget(rolling, rollingSummaryTokenBudget, rollingSummaryMaxCount),
	}
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
