package engine

import (
	"context"
	"log/slog"
	"strings"
	"time"

	contextstate "go-agent/internal/context"
	"go-agent/internal/memory"
	"go-agent/internal/model"
	rolectx "go-agent/internal/roles"
	"go-agent/internal/types"
)

func maybeRefreshContextHeadSummary(ctx context.Context, e *Engine, in Input) error {
	_, err := refreshContextHeadSummary(ctx, e, in)
	return err
}

func refreshContextHeadSummary(ctx context.Context, e *Engine, in Input) (contextHeadSummaryRefreshReport, error) {
	report := contextHeadSummaryRefreshReport{}
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

	existing, hasExisting, err := e.store.GetContextHeadSummary(ctx, sessionID, contextHeadID)
	if err != nil {
		return report, err
	}

	start := 0
	var existingSummary *model.Summary
	if hasExisting {
		if existing.UpToItemID < 0 {
			hasExisting = false
		} else {
			start = contextHeadSummaryStartIndexForUpToItemID(timelineItems, existing.UpToItemID)
			if existing.UpToItemID > 0 && start == 0 && timelineItems[0].ItemID > existing.UpToItemID {
				hasExisting = false
			}
			if summary, ok, err := decodeContextHeadSummaryPayload(existing.SummaryPayload); err != nil {
				return report, err
			} else if ok {
				existingSummary = &summary
			}
		}
	}

	freshItems := cloneConversationItemsForPrompt(items[start:])
	if !shouldRefreshContextHeadSummary(hasExisting, freshItems) {
		return report, nil
	}

	compactInput := buildContextHeadSummaryCompactionInput(existingSummary, freshItems)
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
		summary.RangeLabel = contextHeadSummaryRangeLabel
	}
	upToItemID := timelineItems[len(timelineItems)-1].ItemID

	now := time.Now().UTC()
	record := types.ContextHeadSummary{
		SessionID:      sessionID,
		ContextHeadID:  contextHeadID,
		WorkspaceRoot:  in.Session.WorkspaceRoot,
		SourceTurnID:   in.Turn.ID,
		UpToItemID:     upToItemID,
		ItemCount:      safeEnd,
		SummaryPayload: encodeContextHeadSummaryPayload(summary),
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if hasExisting && !existing.CreatedAt.IsZero() {
		record.CreatedAt = existing.CreatedAt
	}
	if err := e.store.UpsertContextHeadSummary(ctx, record); err != nil {
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

	roleID := resolveMemoryRoleID(ctx, in)

	workspaceEntries := make([]types.MemoryEntry, 0, 1+durableWorkspaceDetailCapPerKind*4)
	if workspaceMemory, ok := buildWorkspaceDurableMemory(record, summary, roleID); ok {
		workspaceEntries = append(workspaceEntries, workspaceMemory)
	}
	workspaceEntries = append(workspaceEntries, buildWorkspaceDetailMemories(record, summary, roleID)...)
	for _, entry := range workspaceEntries {
		if err := e.store.UpsertMemoryEntry(ctx, entry); err == nil {
			report.WorkspaceEntriesUpserted++
		}
	}
	if pruned, err := pruneWorkspaceDurableMemories(ctx, e.store, record.WorkspaceRoot, roleID, workspaceEntries); err == nil {
		report.WorkspaceEntriesPruned = pruned
	}

	for _, globalMemory := range buildGlobalDurableMemories(record, summary, roleID) {
		if err := e.store.UpsertMemoryEntry(ctx, globalMemory); err == nil {
			report.GlobalEntriesUpserted++
		}
	}
	deprecateLowScoringMemories(e.store, record.WorkspaceRoot, roleID, time.Now().UTC())
	return report, nil
}

func deprecateLowScoringMemories(store ConversationStore, workspaceRoot string, roleID string, now time.Time) {
	if store == nil {
		return
	}
	deprecationStore, ok := store.(interface {
		DeprecateMemoryEntries(context.Context, []string) error
	})
	if !ok {
		return
	}
	ctx := context.Background()
	entries, err := store.ListVisibleMemoryEntries(ctx, workspaceRoot, roleID)
	if err != nil {
		return
	}
	var deprecated []string
	for _, entry := range entries {
		if memory.EffectiveScore(entry, now) >= memory.DeprecationThreshold {
			continue
		}
		if err := store.InsertColdIndexEntry(ctx, types.ColdIndexEntry{
			ID:           "cold_memory_deprecated_" + entry.ID,
			WorkspaceID:  workspaceRoot,
			OwnerRoleID:  entry.OwnerRoleID,
			Visibility:   entry.Visibility,
			SourceType:   "memory_deprecated",
			SourceID:     entry.ID,
			SearchText:   entry.Content,
			SummaryLine:  truncateToRunes(entry.Content, 200),
			FilesChanged: entry.SourceRefs,
			OccurredAt:   entry.CreatedAt,
			CreatedAt:    now,
			ContextRef: types.ColdContextRef{
				SessionID:     entry.SourceSessionID,
				ContextHeadID: entry.SourceContextHeadID,
				ItemCount:     0,
			},
		}); err != nil {
			slog.Warn("cold index insert failed, skipping deprecation", "memory_id", entry.ID, "error", err)
			continue
		}
		deprecated = append(deprecated, entry.ID)
	}
	_ = deprecationStore.DeprecateMemoryEntries(ctx, deprecated)
}

func truncateToRunes(s string, max int) string {
	if max <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max])
}

func shouldPromoteHeadToDurableMemory(headID string, canonicalHeadID string) bool {
	headID = strings.TrimSpace(headID)
	canonicalHeadID = strings.TrimSpace(canonicalHeadID)
	return headID != "" && headID == canonicalHeadID
}

func shouldRefreshContextHeadSummary(hasExisting bool, freshItems []model.ConversationItem) bool {
	if len(freshItems) == 0 {
		return false
	}

	estimatedTokens := contextstate.EstimatePromptTokens("", freshItems, SummaryBundle{}, nil)
	signals := countContextHeadSummarySignals(freshItems)
	if hasExisting {
		return len(freshItems) >= contextHeadSummaryCooldownMinItems ||
			estimatedTokens >= contextHeadSummaryUpdateMinTokens*2 ||
			signals >= contextHeadSummarySignalThreshold*2
	}
	return len(freshItems) >= contextHeadSummaryBootstrapMinItems ||
		estimatedTokens >= contextHeadSummaryBootstrapMinTokens ||
		signals >= contextHeadSummarySignalThreshold
}

func countContextHeadSummarySignals(items []model.ConversationItem) int {
	signals := 0
	for _, item := range items {
		switch item.Kind {
		case model.ConversationItemToolCall, model.ConversationItemToolResult, model.ConversationItemSummary:
			signals++
		case model.ConversationItemAssistantText, model.ConversationItemAssistantThinking:
			if len(strings.TrimSpace(item.Text)) >= contextHeadSummaryLongAssistantChars {
				signals++
			}
		}
	}
	return signals
}

func buildContextHeadSummaryCompactionInput(existing *model.Summary, freshItems []model.ConversationItem) []model.ConversationItem {
	out := make([]model.ConversationItem, 0, len(freshItems)+1)
	if existing != nil && !isZeroSummary(*existing) {
		summary := cloneSummaryForContextHeadSummary(*existing)
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

func contextHeadSummaryKeyForInput(in Input) string {
	sessionID := sessionIDForInput(in)
	headID := strings.TrimSpace(in.Turn.ContextHeadID)
	if sessionID == "" || headID == "" {
		return sessionID
	}
	return sessionID + ":" + headID
}

func runObservedContextHeadSummaryRefresh(ctx context.Context, e *Engine, in Input, async bool) error {
	ctx = context.WithoutCancel(ctx)
	_ = emitContextHeadSummaryEvent(ctx, in, types.EventContextHeadSummaryStarted, types.ContextHeadSummaryEventPayload{
		SourceTurnID:  in.Turn.ID,
		WorkspaceRoot: in.Session.WorkspaceRoot,
		Async:         async,
	})

	report, err := refreshContextHeadSummary(ctx, e, in)
	if err != nil {
		_ = emitContextHeadSummaryEvent(ctx, in, types.EventContextHeadSummaryFailed, types.ContextHeadSummaryEventPayload{
			SourceTurnID:  in.Turn.ID,
			WorkspaceRoot: in.Session.WorkspaceRoot,
			Async:         async,
			Message:       err.Error(),
		})
		return err
	}

	_ = emitContextHeadSummaryEvent(ctx, in, types.EventContextHeadSummaryCompleted, types.ContextHeadSummaryEventPayload{
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

func emitContextHeadSummaryEvent(ctx context.Context, in Input, eventType string, payload types.ContextHeadSummaryEventPayload) error {
	if in.Sink == nil {
		return nil
	}
	event, err := types.NewEvent(in.Session.ID, in.Turn.ID, eventType, payload)
	if err != nil {
		return err
	}
	return in.Sink.Emit(ctx, event)
}

func startAsyncContextHeadSummaryRefresh(ctx context.Context, e *Engine, in Input) {
	if e == nil {
		return
	}
	headKey := contextHeadSummaryKeyForInput(in)
	if headKey == "" {
		return
	}

	e.contextHeadSummaryMu.Lock()
	if e.contextHeadSummaryRunning == nil {
		e.contextHeadSummaryRunning = make(map[string]bool)
	}
	if e.contextHeadSummaryPending == nil {
		e.contextHeadSummaryPending = make(map[string]Input)
	}
	if e.contextHeadSummaryRunning[headKey] {
		e.contextHeadSummaryPending[headKey] = in
		e.contextHeadSummaryMu.Unlock()
		return
	}
	e.contextHeadSummaryRunning[headKey] = true
	e.contextHeadSummaryMu.Unlock()

	e.contextHeadSummaryWG.Add(1)
	go func(current Input) {
		defer e.contextHeadSummaryWG.Done()
		for {
			_ = runObservedContextHeadSummaryRefresh(ctx, e, current, true)

			e.contextHeadSummaryMu.Lock()
			next, ok := e.contextHeadSummaryPending[headKey]
			if ok {
				delete(e.contextHeadSummaryPending, headKey)
				e.contextHeadSummaryMu.Unlock()
				current = next
				continue
			}
			delete(e.contextHeadSummaryRunning, headKey)
			e.contextHeadSummaryMu.Unlock()
			return
		}
	}(in)
}

// resolveMemoryRoleID returns the role ID that should own new memory entries.
// For specialist sessions, the role ID comes from the context (set by the daemon
// when it dispatches a task to a role). For main_parent sessions, memory is
// written with workspace/global scope and no role owner.
func resolveMemoryRoleID(ctx context.Context, in Input) string {
	if specialistID := rolectx.SpecialistRoleIDFromContext(ctx); specialistID != "" {
		return specialistID
	}
	return ""
}
