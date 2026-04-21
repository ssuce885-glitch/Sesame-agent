package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	return executePreparedLoop(ctx, e, in, emitter, state)
}

func effectiveTurnMessage(turn types.Turn) string {
	if normalizeTurnKind(turn.Kind) == types.TurnKindChildReportBatch {
		return "Review the child reports and continue the main conversation."
	}
	return turn.UserMessage
}

func turnEntryUserItem(in Input) model.ConversationItem {
	if in.Resume != nil {
		return model.ConversationItem{}
	}
	return model.UserMessageItem(effectiveTurnMessage(in.Turn))
}

func resumeToolResultItem(resume *types.TurnResume) (model.ConversationItem, model.ToolResult) {
	if resume == nil {
		return model.ConversationItem{}, model.ToolResult{}
	}
	content := fmt.Sprintf("Permission request resolved: %s.", resume.Decision)
	isError := resume.Decision == types.PermissionDecisionDeny
	if resume.DecisionScope != "" {
		content += " Scope: " + resume.DecisionScope + "."
	}
	if resume.RequestedProfile != "" {
		content += " Requested profile: " + resume.RequestedProfile + "."
	}
	if resume.Reason != "" {
		content += " Reason: " + resume.Reason + "."
	}
	if resume.EffectivePermissionProfile != "" {
		content += " Effective profile: " + resume.EffectivePermissionProfile + "."
	}
	result := model.ToolResult{
		ToolCallID: resume.ToolCallID,
		ToolName:   resume.ToolName,
		Content:    content,
		StructuredJSON: marshalStructuredToolResult(map[string]any{
			"status":                       map[bool]string{true: "denied", false: "resolved"}[isError],
			"decision":                     resume.Decision,
			"decision_scope":               resume.DecisionScope,
			"requested_profile":            resume.RequestedProfile,
			"effective_permission_profile": resume.EffectivePermissionProfile,
		}),
		IsError: isError,
	}
	return model.ToolResultItem(result), result
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

	summaryBundle, compactions, err := loadHeadMemoryBundle(ctx, e.store, sessionID, contextHeadID)
	if err != nil {
		return 0, contextstate.WorkingSet{}, err
	}
	hasHeadMemory := summaryBundle.HeadMemory != nil
	headMemoryUpTo := loadHeadMemoryUpTo(ctx, e.store, sessionID, contextHeadID)

	entries, err := e.store.ListMemoryEntriesByWorkspace(ctx, in.Session.WorkspaceRoot)
	if err != nil {
		return 0, contextstate.WorkingSet{}, err
	}

	memoryRefs := buildMemoryRefs(entries, hasHeadMemory, in.Session.WorkspaceRoot, turnMessage)

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
			headMemoryUpTo,
			working,
		)
		if err != nil {
			return 0, contextstate.WorkingSet{}, err
		}
	}

	return totalItems, working, nil
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
	headMemoryUpTo int64,
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
			headMemoryUpTo,
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
			headMemoryUpTo,
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

	nextBundle, nextCompactions, err := loadHeadMemoryBundle(ctx, e.store, sessionID, contextHeadID)
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
	headMemoryUpTo int64,
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
			headMemoryUpTo,
			len(items),
			reason,
			string(e.model.Capabilities().Profile),
			len(activeMicrocompactItems(compactions)) > 0,
		)),
		Reason:          reason,
		ProviderProfile: string(e.model.Capabilities().Profile),
		CreatedAt:       time.Now().UTC(),
	}); err != nil {
		return contextstate.WorkingSet{}, SummaryBundle{}, err
	}

	summaryBundle, compactions, err = loadHeadMemoryBundle(ctx, e.store, sessionID, contextHeadID)
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

func newBoundaryMetadata(generation int, cutoff int, headMemoryUpTo int64, sourceItemCount int, reason string, providerProfile string, hasRecentMicrocompact bool) types.CompactionBoundaryMetadata {
	return types.CompactionBoundaryMetadata{
		Version:               1,
		PromptLayoutVersion:   1,
		Generation:            generation,
		CompactedStart:        0,
		CompactedEnd:          cutoff,
		PreservedRecentStart:  cutoff,
		HeadMemoryUpTo:        headMemoryUpTo,
		SourceItemCount:       sourceItemCount,
		Reason:                reason,
		ProviderProfile:       providerProfile,
		HasRecentMicrocompact: hasRecentMicrocompact,
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

func loadHeadMemoryUpTo(ctx context.Context, store ConversationStore, sessionID, contextHeadID string) int64 {
	if store == nil || strings.TrimSpace(sessionID) == "" || strings.TrimSpace(contextHeadID) == "" {
		return 0
	}
	memory, ok, err := store.GetHeadMemory(ctx, sessionID, contextHeadID)
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
