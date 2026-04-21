package engine

import (
	"context"
	"strings"

	"go-agent/internal/model"
	"go-agent/internal/types"
)

func loadHeadMemorySummary(ctx context.Context, store ConversationStore, sessionID, contextHeadID string) (model.Summary, bool, error) {
	if store == nil || strings.TrimSpace(sessionID) == "" || strings.TrimSpace(contextHeadID) == "" {
		return model.Summary{}, false, nil
	}

	memory, ok, err := store.GetHeadMemory(ctx, sessionID, contextHeadID)
	if err != nil || !ok {
		return model.Summary{}, false, err
	}
	return decodeSessionMemorySummary(memory.SummaryPayload)
}

func loadHeadMemoryBundle(ctx context.Context, store ConversationStore, sessionID, contextHeadID string) (SummaryBundle, []types.ConversationCompaction, error) {
	if store == nil || strings.TrimSpace(sessionID) == "" || strings.TrimSpace(contextHeadID) == "" {
		return SummaryBundle{}, nil, nil
	}

	compactions, err := store.ListConversationCompactionsByStoredContextHead(ctx, sessionID, contextHeadID)
	if err != nil {
		return SummaryBundle{}, nil, err
	}
	headMemory, hasHeadMemory, err := loadHeadMemorySummary(ctx, store, sessionID, contextHeadID)
	if err != nil {
		return SummaryBundle{}, nil, err
	}

	var headMemorySummary *model.Summary
	if hasHeadMemory {
		value := cloneSummaryForSessionMemory(headMemory)
		headMemorySummary = &value
	}

	if boundarySummary, ok, err := activeBoundarySummary(compactions); err != nil {
		return SummaryBundle{}, nil, err
	} else if ok {
		value := normalizeSummaryForPrompt(boundarySummary)
		return selectPromptSummaryBundle(headMemorySummary, &value, nil), compactions, nil
	}

	return selectPromptSummaryBundle(headMemorySummary, nil, nil), compactions, nil
}

func activeBoundarySummary(compactions []types.ConversationCompaction) (model.Summary, bool, error) {
	boundaryCompaction, ok := activeBoundaryCompaction(compactions)
	if !ok {
		return model.Summary{}, false, nil
	}
	return decodeCompactionSummaryPayload(boundaryCompaction.SummaryPayload)
}

func resolveConversationReadContextHeadID(ctx context.Context, store ConversationStore, preferredContextHeadID string) (string, error) {
	if resolved := strings.TrimSpace(preferredContextHeadID); resolved != "" {
		return resolved, nil
	}
	if store == nil {
		return "", nil
	}
	current, ok, err := store.GetCurrentContextHeadID(ctx)
	if err != nil || !ok {
		return "", err
	}
	return strings.TrimSpace(current), nil
}

func headMemoryStartIndexForUpToItemID(items []types.ConversationTimelineItem, upToItemID int64) int {
	if upToItemID <= 0 || len(items) == 0 {
		return 0
	}
	for i, item := range items {
		if item.ItemID > upToItemID {
			return i
		}
	}
	return len(items)
}
