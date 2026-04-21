package engine

import (
	"context"
	"strings"
	"time"

	contextstate "go-agent/internal/context"
	"go-agent/internal/model"
	"go-agent/internal/types"
)

func maybeRefreshHeadMemory(ctx context.Context, e *Engine, in Input) error {
	_, err := refreshHeadMemory(ctx, e, in)
	return err
}

func refreshHeadMemory(ctx context.Context, e *Engine, in Input) (headMemoryRefreshReport, error) {
	report := headMemoryRefreshReport{}
	if e == nil || e.store == nil || e.compactor == nil {
		return report, nil
	}

	sessionID := in.Turn.SessionID
	if strings.TrimSpace(sessionID) == "" {
		sessionID = in.Session.ID
	}
	if strings.TrimSpace(sessionID) == "" {
		return report, nil
	}

	contextHeadID, err := resolveConversationReadContextHeadID(ctx, e.store, in.Turn.ContextHeadID)
	if err != nil || contextHeadID == "" {
		return report, err
	}

	timelineItems, err := e.store.ListConversationTimelineItemsByContextHead(ctx, sessionID, contextHeadID)
	if err != nil {
		return report, err
	}
	if len(timelineItems) == 0 {
		return report, nil
	}

	items := make([]model.ConversationItem, 0, len(timelineItems))
	for _, item := range timelineItems {
		items = append(items, item.Item)
	}
	safeEnd := model.NearestSafeConversationBoundary(items, len(items))
	if safeEnd <= 0 {
		return report, nil
	}
	timelineItems = timelineItems[:safeEnd]
	items = items[:safeEnd]

	existing, hasExisting, err := e.store.GetHeadMemory(ctx, sessionID, contextHeadID)
	if err != nil {
		return report, err
	}

	start := 0
	var existingSummary *model.Summary
	if hasExisting {
		if existing.UpToItemID < 0 {
			hasExisting = false
		} else {
			start = headMemoryStartIndexForUpToItemID(timelineItems, existing.UpToItemID)
			if existing.UpToItemID > 0 && start == 0 && timelineItems[0].ItemID > existing.UpToItemID {
				hasExisting = false
			}
			if summary, ok, err := decodeSessionMemorySummary(existing.SummaryPayload); err != nil {
				return report, err
			} else if ok {
				existingSummary = &summary
			}
		}
	}

	freshItems := cloneConversationItemsForPrompt(items[start:])
	if !shouldRefreshSessionMemory(hasExisting, freshItems) {
		return report, nil
	}

	compactInput := buildSessionMemoryCompactionInput(existingSummary, freshItems)
	if len(compactInput) == 0 {
		return report, nil
	}

	summary, err := e.compactor.Compact(ctx, compactInput)
	if err != nil {
		return report, err
	}
	if isZeroSummary(summary) {
		return report, nil
	}
	if strings.TrimSpace(summary.RangeLabel) == "" {
		summary.RangeLabel = sessionMemoryRangeLabel
	}
	upToItemID := timelineItems[len(timelineItems)-1].ItemID

	now := time.Now().UTC()
	record := types.HeadMemory{
		SessionID:      sessionID,
		ContextHeadID:  contextHeadID,
		WorkspaceRoot:  in.Session.WorkspaceRoot,
		SourceTurnID:   in.Turn.ID,
		UpToItemID:     upToItemID,
		ItemCount:      safeEnd,
		SummaryPayload: encodeSessionMemorySummary(summary),
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if hasExisting && !existing.CreatedAt.IsZero() {
		record.CreatedAt = existing.CreatedAt
	}
	if err := e.store.UpsertHeadMemory(ctx, record); err != nil {
		return report, err
	}
	report.Updated = true

	canonicalHeadID, err := resolveConversationReadContextHeadID(ctx, e.store, "")
	if err != nil {
		return report, err
	}
	if !shouldPromoteHeadToDurableMemory(contextHeadID, canonicalHeadID) {
		return report, nil
	}

	workspaceEntries := make([]types.MemoryEntry, 0, 1+durableWorkspaceDetailCapPerKind*4)
	if workspaceMemory, ok := buildWorkspaceDurableMemory(record, summary); ok {
		workspaceEntries = append(workspaceEntries, workspaceMemory)
	}
	workspaceEntries = append(workspaceEntries, buildWorkspaceDetailMemories(record, summary)...)
	for _, entry := range workspaceEntries {
		if err := e.store.UpsertMemoryEntry(ctx, entry); err == nil {
			report.WorkspaceEntriesUpserted++
		}
	}
	if pruned, err := pruneWorkspaceDurableMemories(ctx, e.store, record.WorkspaceRoot, workspaceEntries); err == nil {
		report.WorkspaceEntriesPruned = pruned
	}

	for _, globalMemory := range buildGlobalDurableMemories(record, summary) {
		if err := e.store.UpsertMemoryEntry(ctx, globalMemory); err == nil {
			report.GlobalEntriesUpserted++
		}
	}
	return report, nil
}

func shouldPromoteHeadToDurableMemory(headID string, canonicalHeadID string) bool {
	headID = strings.TrimSpace(headID)
	canonicalHeadID = strings.TrimSpace(canonicalHeadID)
	return headID != "" && headID == canonicalHeadID
}

func shouldRefreshSessionMemory(hasExisting bool, freshItems []model.ConversationItem) bool {
	if len(freshItems) == 0 {
		return false
	}

	estimatedTokens := contextstate.EstimatePromptTokens("", freshItems, SummaryBundle{}, nil)
	signals := countSessionMemorySignals(freshItems)
	if hasExisting {
		return len(freshItems) >= sessionMemoryUpdateMinItems ||
			estimatedTokens >= sessionMemoryUpdateMinTokens ||
			signals >= sessionMemorySignalThreshold
	}
	return len(freshItems) >= sessionMemoryBootstrapMinItems ||
		estimatedTokens >= sessionMemoryBootstrapMinTokens ||
		signals >= sessionMemorySignalThreshold
}

func countSessionMemorySignals(items []model.ConversationItem) int {
	signals := 0
	for _, item := range items {
		switch item.Kind {
		case model.ConversationItemToolCall, model.ConversationItemToolResult, model.ConversationItemSummary:
			signals++
		case model.ConversationItemAssistantText, model.ConversationItemAssistantThinking:
			if len(strings.TrimSpace(item.Text)) >= sessionMemoryLongAssistantChars {
				signals++
			}
		}
	}
	return signals
}

func buildSessionMemoryCompactionInput(existing *model.Summary, freshItems []model.ConversationItem) []model.ConversationItem {
	out := make([]model.ConversationItem, 0, len(freshItems)+1)
	if existing != nil && !isZeroSummary(*existing) {
		summary := cloneSummaryForSessionMemory(*existing)
		out = append(out, model.ConversationItem{
			Kind:    model.ConversationItemSummary,
			Summary: &summary,
		})
	}
	out = append(out, cloneConversationItemsForPrompt(freshItems)...)
	return out
}

func sessionIDForInput(in Input) string {
	sessionID := strings.TrimSpace(in.Turn.SessionID)
	if sessionID != "" {
		return sessionID
	}
	return strings.TrimSpace(in.Session.ID)
}

func headMemoryKeyForInput(in Input) string {
	sessionID := sessionIDForInput(in)
	headID := strings.TrimSpace(in.Turn.ContextHeadID)
	if sessionID == "" || headID == "" {
		return sessionID
	}
	return sessionID + ":" + headID
}

func runObservedHeadMemoryRefresh(ctx context.Context, e *Engine, in Input, async bool) error {
	ctx = context.WithoutCancel(ctx)
	_ = emitHeadMemoryEvent(ctx, in, types.EventHeadMemoryStarted, types.HeadMemoryEventPayload{
		SourceTurnID:  in.Turn.ID,
		WorkspaceRoot: in.Session.WorkspaceRoot,
		Async:         async,
	})

	report, err := refreshHeadMemory(ctx, e, in)
	if err != nil {
		_ = emitHeadMemoryEvent(ctx, in, types.EventHeadMemoryFailed, types.HeadMemoryEventPayload{
			SourceTurnID:  in.Turn.ID,
			WorkspaceRoot: in.Session.WorkspaceRoot,
			Async:         async,
			Message:       err.Error(),
		})
		return err
	}

	_ = emitHeadMemoryEvent(ctx, in, types.EventHeadMemoryCompleted, types.HeadMemoryEventPayload{
		SourceTurnID:             in.Turn.ID,
		WorkspaceRoot:            in.Session.WorkspaceRoot,
		Async:                    async,
		Updated:                  report.Updated,
		WorkspaceEntriesUpserted: report.WorkspaceEntriesUpserted,
		GlobalEntriesUpserted:    report.GlobalEntriesUpserted,
		WorkspaceEntriesPruned:   report.WorkspaceEntriesPruned,
	})
	return nil
}

func emitHeadMemoryEvent(ctx context.Context, in Input, eventType string, payload types.HeadMemoryEventPayload) error {
	if in.Sink == nil {
		return nil
	}
	event, err := types.NewEvent(in.Session.ID, in.Turn.ID, eventType, payload)
	if err != nil {
		return err
	}
	return in.Sink.Emit(ctx, event)
}

func startAsyncHeadMemoryRefresh(ctx context.Context, e *Engine, in Input) {
	if e == nil {
		return
	}
	headKey := headMemoryKeyForInput(in)
	if headKey == "" {
		return
	}

	e.headMemoryMu.Lock()
	if e.headMemoryRunning == nil {
		e.headMemoryRunning = make(map[string]bool)
	}
	if e.headMemoryPending == nil {
		e.headMemoryPending = make(map[string]Input)
	}
	if e.headMemoryRunning[headKey] {
		e.headMemoryPending[headKey] = in
		e.headMemoryMu.Unlock()
		return
	}
	e.headMemoryRunning[headKey] = true
	e.headMemoryMu.Unlock()

	e.headMemoryWG.Add(1)
	go func(current Input) {
		defer e.headMemoryWG.Done()
		for {
			_ = runObservedHeadMemoryRefresh(ctx, e, current, true)

			e.headMemoryMu.Lock()
			next, ok := e.headMemoryPending[headKey]
			if ok {
				delete(e.headMemoryPending, headKey)
				e.headMemoryMu.Unlock()
				current = next
				continue
			}
			delete(e.headMemoryRunning, headKey)
			e.headMemoryMu.Unlock()
			return
		}
	}(in)
}
