package engine

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	contextstate "go-agent/internal/context"
	"go-agent/internal/model"
	"go-agent/internal/types"
)

func runLoop(ctx context.Context, e *Engine, in Input) error {
	if e == nil {
		return errors.New("engine is required")
	}
	if e.model == nil {
		return errors.New("model client is required")
	}
	if e.registry == nil {
		return errors.New("tool registry is required")
	}
	if e.permission == nil {
		return errors.New("permission engine is required")
	}
	if in.Sink == nil {
		return errors.New("event sink is required")
	}
	emitter := newLoopEmitter(in)
	state, err := prepareLoopState(ctx, e, in, emitter)
	if err != nil {
		return err
	}
	if err := persistInitialLoopItems(ctx, e, in, emitter, state); err != nil {
		return err
	}
	runErr := executePreparedLoop(ctx, e, in, emitter, state)
	if e.runtimeService != nil {
		if finishErr := e.runtimeService.FinishCurrentRun(context.WithoutCancel(ctx), state.turnCtx, in.Session.ID, in.Turn.ID, runErr); finishErr != nil && runErr == nil {
			return finishErr
		}
	}
	return runErr
}

func effectiveTurnMessage(turn types.Turn) string {
	if normalizeTurnKind(turn.Kind) == types.TurnKindReportBatch {
		return "Review the reports and continue the conversation."
	}
	return turn.UserMessage
}

func turnEntryUserItem(in Input) model.ConversationItem {
	return model.UserMessageItem(effectiveTurnMessage(in.Turn))
}

func loadConversationState(ctx context.Context, e *Engine, in Input, sessionID string, turnMessage string) (int, contextstate.WorkingSet, error) {
	if e.store == nil || e.ctxManager == nil {
		return 0, contextstate.WorkingSet{}, nil
	}

	contextHeadID, err := resolveConversationReadContextHeadID(ctx, e.store, in.Turn.ContextHeadID)
	if err != nil {
		return 0, contextstate.WorkingSet{}, err
	}
	items, err := loadPromptItemsForHead(ctx, e.store, sessionID, contextHeadID)
	if err != nil {
		return 0, contextstate.WorkingSet{}, err
	}
	totalItems := len(items)

	summaryBundle, compactions, err := loadContextHeadSummaryBundle(ctx, e.store, sessionID, contextHeadID)
	if err != nil {
		return 0, contextstate.WorkingSet{}, err
	}
	hasContextHeadSummary := summaryBundle.ContextHeadSummary != nil
	contextHeadSummaryUpTo := loadContextHeadSummaryUpTo(ctx, e.store, sessionID, contextHeadID)

	roleID := resolveMemoryRoleID(ctx, in)
	entries, err := e.store.ListVisibleMemoryEntries(ctx, in.Session.WorkspaceRoot, roleID)
	if err != nil {
		return 0, contextstate.WorkingSet{}, err
	}

	memoryRefs, usedMemoryIDs := buildMemoryRefsAndUsage(entries, hasContextHeadSummary, in.Session.WorkspaceRoot, turnMessage)
	if err := markMemoryEntriesUsed(ctx, e.store, usedMemoryIDs); err != nil {
		return 0, contextstate.WorkingSet{}, err
	}

	persistedMicroItems := activeMicrocompactItems(compactions)
	recentWindowItems, recentWindowOverride := recentRawItemsForCompactionWindow(items, compactions)
	working := e.ctxManager.Build(turnMessage, items, summaryBundle, memoryRefs)
	working = setPromptItems(working, persistedMicroItems, recentWindowItems, recentWindowOverride, turnMessage)
	if e.compactor != nil {
		working, summaryBundle, err = runCompactionPasses(
			ctx,
			e,
			sessionID,
			contextHeadID,
			turnMessage,
			items,
			summaryBundle,
			memoryRefs,
			compactions,
			recentWindowItems,
			contextHeadSummaryUpTo,
			working,
		)
		if err != nil {
			return 0, contextstate.WorkingSet{}, err
		}
	}

	return totalItems, working, nil
}

type memoryUsageStore interface {
	MarkMemoryEntriesUsed(context.Context, []string, time.Time) error
}

func markMemoryEntriesUsed(ctx context.Context, store ConversationStore, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	usageStore, ok := store.(memoryUsageStore)
	if !ok {
		return nil
	}
	return usageStore.MarkMemoryEntriesUsed(ctx, ids, time.Now().UTC())
}

func runCompactionPasses(
	ctx context.Context,
	e *Engine,
	sessionID string,
	turnContextHeadID string,
	userMessage string,
	items []model.ConversationItem,
	summaryBundle SummaryBundle,
	memoryRefs []string,
	compactions []types.ConversationCompaction,
	recentWindowItems []model.ConversationItem,
	contextHeadSummaryUpTo int64,
	working contextstate.WorkingSet,
) (contextstate.WorkingSet, SummaryBundle, error) {
	switch working.Action.Kind {
	case contextstate.CompactionActionRolling:
		return applySummaryCompaction(
			ctx,
			e,
			sessionID,
			turnContextHeadID,
			userMessage,
			items,
			summaryBundle,
			memoryRefs,
			compactions,
			contextHeadSummaryUpTo,
			working,
			len(compactions)+1,
			types.ConversationCompactionKindRolling,
			"rolling_summary",
		)
	case contextstate.CompactionActionMicrocompact:
		nextWorking, nextBundle, nextCompactions, _, err := applyMicrocompactPass(
			ctx,
			e,
			sessionID,
			turnContextHeadID,
			userMessage,
			items,
			summaryBundle,
			memoryRefs,
			compactions,
			recentWindowItems,
			working,
		)
		if err != nil {
			return contextstate.WorkingSet{}, SummaryBundle{}, err
		}
		if !shouldApplyBoundaryCompaction(nextWorking, e.ctxManager.Config()) {
			return nextWorking, nextBundle, nil
		}
		return applySummaryCompaction(
			ctx,
			e,
			sessionID,
			turnContextHeadID,
			userMessage,
			items,
			nextBundle,
			memoryRefs,
			nextCompactions,
			contextHeadSummaryUpTo,
			nextWorking,
			len(nextCompactions)+1,
			types.ConversationCompactionKindRolling,
			"microcompact_escalated_to_rolling",
		)
	default:
		return working, summaryBundle, nil
	}
}

func applyMicrocompactPass(
	ctx context.Context,
	e *Engine,
	sessionID string,
	turnContextHeadID string,
	userMessage string,
	items []model.ConversationItem,
	summaryBundle SummaryBundle,
	memoryRefs []string,
	compactions []types.ConversationCompaction,
	recentWindowItems []model.ConversationItem,
	working contextstate.WorkingSet,
) (contextstate.WorkingSet, SummaryBundle, []types.ConversationCompaction, bool, error) {
	candidatePayload, _, ok := buildAppliedMicrocompact(items, working.Action.MicrocompactPositions, working.CompactionStart)
	if !ok {
		return working, summaryBundle, compactions, false, nil
	}
	contextHeadID, err := resolveConversationWriteContextHeadID(ctx, e.store, turnContextHeadID)
	if err != nil {
		return contextstate.WorkingSet{}, SummaryBundle{}, nil, false, err
	}
	startPosition := firstPayloadPosition(candidatePayload)
	endPosition := lastPayloadPosition(candidatePayload)
	startItemID, endItemID, err := resolveConversationCompactionItemIDBounds(ctx, e.store, sessionID, contextHeadID, startPosition, endPosition)
	if err != nil {
		return contextstate.WorkingSet{}, SummaryBundle{}, nil, false, err
	}
	if err := e.store.InsertConversationCompactionWithContextHead(ctx, types.ConversationCompaction{
		ID:              types.NewID("compact"),
		SessionID:       sessionID,
		ContextHeadID:   contextHeadID,
		Kind:            types.ConversationCompactionKindMicro,
		Generation:      len(compactions) + 1,
		StartItemID:     startItemID,
		EndItemID:       endItemID,
		StartPosition:   startPosition,
		EndPosition:     endPosition,
		SummaryPayload:  encodeMicrocompactPayload(candidatePayload),
		Reason:          "microcompact_tool_results",
		ProviderProfile: string(e.model.Capabilities().Profile),
		CreatedAt:       time.Now().UTC(),
	}); err != nil {
		return contextstate.WorkingSet{}, SummaryBundle{}, nil, false, err
	}

	nextBundle, nextCompactions, err := loadContextHeadSummaryBundle(ctx, e.store, sessionID, contextHeadID)
	if err != nil {
		return contextstate.WorkingSet{}, SummaryBundle{}, nil, false, err
	}
	persistedMicroItems := activeMicrocompactItems(nextCompactions)
	working = e.ctxManager.Build(userMessage, items, nextBundle, memoryRefs)
	working = setPromptItems(
		working,
		persistedMicroItems,
		recentRawItemsFromMicrocompact(items, candidatePayload.RecentStart, recentWindowItems),
		true,
		userMessage,
	)
	working.CompactionApplied = true
	return working, nextBundle, nextCompactions, true, nil
}

func shouldApplyBoundaryCompaction(working contextstate.WorkingSet, cfg contextstate.Config) bool {
	return working.EstimatedTokens > cfg.MaxEstimatedTokens
}

func applySummaryCompaction(
	ctx context.Context,
	e *Engine,
	sessionID string,
	turnContextHeadID string,
	userMessage string,
	items []model.ConversationItem,
	summaryBundle SummaryBundle,
	memoryRefs []string,
	compactions []types.ConversationCompaction,
	contextHeadSummaryUpTo int64,
	working contextstate.WorkingSet,
	generation int,
	kind types.ConversationCompactionKind,
	reason string,
) (contextstate.WorkingSet, SummaryBundle, error) {
	cutoff := working.CompactionStart
	if cutoff < 0 {
		cutoff = 0
	}
	if cutoff > len(items) {
		cutoff = len(items)
	}
	cutoff = model.NearestSafeConversationBoundary(items, cutoff)
	if cutoff == 0 {
		return working, summaryBundle, nil
	}

	summary, err := e.compactor.Compact(ctx, items[:cutoff])
	if err != nil {
		return contextstate.WorkingSet{}, SummaryBundle{}, err
	}
	contextHeadID, err := resolveConversationWriteContextHeadID(ctx, e.store, turnContextHeadID)
	if err != nil {
		return contextstate.WorkingSet{}, SummaryBundle{}, err
	}
	startPosition := 0
	endPosition := cutoff
	startItemID, endItemID, err := resolveConversationCompactionItemIDBounds(ctx, e.store, sessionID, contextHeadID, startPosition, endPosition)
	if err != nil {
		return contextstate.WorkingSet{}, SummaryBundle{}, err
	}
	if err := e.store.InsertConversationCompactionWithContextHead(ctx, types.ConversationCompaction{
		ID:             types.NewID("compact"),
		SessionID:      sessionID,
		ContextHeadID:  contextHeadID,
		Kind:           kind,
		Generation:     generation,
		StartItemID:    startItemID,
		EndItemID:      endItemID,
		StartPosition:  startPosition,
		EndPosition:    endPosition,
		SummaryPayload: marshalCompactionSummary(summary),
		MetadataJSON: encodeBoundaryMetadata(newBoundaryMetadata(
			generation,
			cutoff,
			contextHeadSummaryUpTo,
			len(items),
			reason,
			string(e.model.Capabilities().Profile),
			len(activeMicrocompactItems(compactions)) > 0,
			items,
		)),
		Reason:          reason,
		ProviderProfile: string(e.model.Capabilities().Profile),
		CreatedAt:       time.Now().UTC(),
	}); err != nil {
		return contextstate.WorkingSet{}, SummaryBundle{}, err
	}

	summaryBundle, compactions, err = loadContextHeadSummaryBundle(ctx, e.store, sessionID, contextHeadID)
	if err != nil {
		return contextstate.WorkingSet{}, SummaryBundle{}, err
	}
	persistedMicroItems := activeMicrocompactItems(compactions)
	recentWindowItems, recentWindowOverride := recentRawItemsForCompactionWindow(items, compactions)
	working = e.ctxManager.Build(userMessage, items, summaryBundle, memoryRefs)
	working = setPromptItems(working, persistedMicroItems, recentWindowItems, recentWindowOverride, userMessage)
	working.CompactionApplied = true
	return working, summaryBundle, nil
}

func newBoundaryMetadata(generation int, cutoff int, contextHeadSummaryUpTo int64, sourceItemCount int, reason string, providerProfile string, hasRecentMicrocompact bool, items []model.ConversationItem) types.CompactionBoundaryMetadata {
	preservedUserCount := 0
	for _, item := range items[cutoff:] {
		if item.Kind == model.ConversationItemUserMessage {
			preservedUserCount++
		}
	}
	return types.CompactionBoundaryMetadata{
		Version:                   1,
		PromptLayoutVersion:       1,
		Generation:                generation,
		CompactedStart:            0,
		CompactedEnd:              cutoff,
		PreservedRecentStart:      cutoff,
		PreservedUserMessageCount: preservedUserCount,
		IsPreTurn:                 true,
		ContextHeadSummaryUpTo:    contextHeadSummaryUpTo,
		SourceItemCount:           sourceItemCount,
		Reason:                    reason,
		ProviderProfile:           providerProfile,
		HasRecentMicrocompact:     hasRecentMicrocompact,
	}
}

func marshalCompactionSummary(summary model.Summary) string {
	raw, err := json.Marshal(summary)
	if err != nil {
		return summary.RangeLabel
	}
	return string(raw)
}

func setPromptItems(working contextstate.WorkingSet, carryForwardItems []model.ConversationItem, recentRawItems []model.ConversationItem, overrideRecentRaw bool, userMessage string) contextstate.WorkingSet {
	working.CarryForwardItems = cloneConversationItemsForPrompt(carryForwardItems)
	if overrideRecentRaw {
		working.RecentRawItems = cloneConversationItemsForPrompt(recentRawItems)
	}
	recentItems := working.RecentRawItems
	if len(recentItems) == 0 {
		recentItems = working.RecentItems
	}
	working.PromptItems = appendPromptItems(working.CarryForwardItems, recentItems)
	working.EstimatedTokens = contextstate.EstimatePromptTokens(userMessage, working.PromptItems, working.Summaries, working.MemoryRefs)
	return working
}

func loadContextHeadSummaryUpTo(ctx context.Context, store ConversationStore, sessionID, contextHeadID string) int64 {
	if store == nil || strings.TrimSpace(sessionID) == "" || strings.TrimSpace(contextHeadID) == "" {
		return 0
	}
	memory, ok, err := store.GetContextHeadSummary(ctx, sessionID, contextHeadID)
	if err != nil || !ok || memory.UpToItemID < 0 {
		return 0
	}
	return memory.UpToItemID
}

func appendPromptItems(carryForwardItems, recentItems []model.ConversationItem) []model.ConversationItem {
	if len(carryForwardItems) == 0 {
		return cloneConversationItemsForPrompt(recentItems)
	}

	out := make([]model.ConversationItem, 0, len(carryForwardItems)+len(recentItems))
	out = append(out, cloneConversationItemsForPrompt(carryForwardItems)...)
	out = append(out, cloneConversationItemsForPrompt(recentItems)...)
	return out
}

func buildAppliedMicrocompact(items []model.ConversationItem, positions []int, recentStart int) (persistedMicrocompactPayload, []model.ConversationItem, bool) {
	payload, err := buildMicrocompactPayload(items, positions, recentStart)
	if err != nil || len(payload.Items) == 0 {
		return persistedMicrocompactPayload{}, nil, false
	}
	promptItems := appendPromptItems(payload.Items, items[recentStart:])
	return payload, promptItems, len(promptItems) > 0
}

func firstPayloadPosition(payload persistedMicrocompactPayload) int {
	if len(payload.SourcePositions) == 0 {
		return 0
	}
	return payload.SourcePositions[0]
}

func lastPayloadPosition(payload persistedMicrocompactPayload) int {
	if len(payload.SourcePositions) == 0 {
		return 0
	}
	return payload.SourcePositions[len(payload.SourcePositions)-1]
}
