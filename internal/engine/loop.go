package engine

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	contextstate "go-agent/internal/context"
	"go-agent/internal/model"
	rolectx "go-agent/internal/roles"
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
	e.ctxManager.SetCircuitBreakerOpen(e.compactionCircuitOpen())
	working := e.ctxManager.Build(turnMessage, items, summaryBundle, memoryRefs)
	working = setPromptItems(working, persistedMicroItems, recentWindowItems, recentWindowOverride, turnMessage)
	if e.compactor != nil || working.Action.Kind == contextstate.CompactionActionArchive {
		working, summaryBundle, err = runCompactionPasses(
			ctx,
			e,
			sessionID,
			in.Session.WorkspaceRoot,
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
	workspaceRoot string,
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
	case contextstate.CompactionActionArchive:
		return applyTrackedArchiveCompaction(
			ctx,
			e,
			sessionID,
			workspaceRoot,
			turnContextHeadID,
			userMessage,
			items,
			summaryBundle,
			memoryRefs,
			compactions,
			contextHeadSummaryUpTo,
			working,
			len(compactions)+1,
			"forced_archive_eviction",
		)
	case contextstate.CompactionActionRolling:
		if e.compactionCircuitOpen() {
			return working, summaryBundle, nil
		}
		return applyTrackedSummaryCompaction(
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
		if e.compactionCircuitOpen() {
			return working, summaryBundle, nil
		}
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
		return applyTrackedSummaryCompaction(
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

func applyTrackedSummaryCompaction(
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
	nextWorking, nextBundle, err := applySummaryCompaction(
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
		generation,
		kind,
		reason,
	)
	e.recordSummaryCompactionResult(err)
	return nextWorking, nextBundle, err
}

func applyTrackedArchiveCompaction(
	ctx context.Context,
	e *Engine,
	sessionID string,
	workspaceRoot string,
	turnContextHeadID string,
	userMessage string,
	items []model.ConversationItem,
	summaryBundle SummaryBundle,
	memoryRefs []string,
	compactions []types.ConversationCompaction,
	contextHeadSummaryUpTo int64,
	working contextstate.WorkingSet,
	generation int,
	reason string,
) (contextstate.WorkingSet, SummaryBundle, error) {
	nextWorking, nextBundle, err := applyArchiveCompaction(
		ctx,
		e,
		sessionID,
		workspaceRoot,
		turnContextHeadID,
		userMessage,
		items,
		summaryBundle,
		memoryRefs,
		compactions,
		contextHeadSummaryUpTo,
		working,
		generation,
		reason,
	)
	e.resetCompactionCircuit()
	if e != nil && e.ctxManager != nil {
		e.ctxManager.SetCircuitBreakerOpen(false)
	}
	return nextWorking, nextBundle, err
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
	return working.EstimatedTokens > cfg.EffectiveMaxEstimatedTokens()
}

func applyArchiveCompaction(
	ctx context.Context,
	e *Engine,
	sessionID string,
	workspaceRoot string,
	turnContextHeadID string,
	userMessage string,
	items []model.ConversationItem,
	summaryBundle SummaryBundle,
	memoryRefs []string,
	compactions []types.ConversationCompaction,
	contextHeadSummaryUpTo int64,
	working contextstate.WorkingSet,
	generation int,
	reason string,
) (contextstate.WorkingSet, SummaryBundle, error) {
	cutoff := working.Action.RangeEnd
	if cutoff <= 0 {
		cutoff = working.CompactionStart
	}
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

	archiveItems := items[:cutoff]
	extraction := contextstate.ArchiveExtraction{}
	var extractErr error
	if e.archiver != nil {
		extraction, extractErr = e.archiver.ExtractArchive(ctx, archiveItems)
	}
	if e.archiver == nil || extractErr != nil {
		extraction = contextstate.ComputedArchiveFallback(archiveItems)
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

	createdAt := time.Now().UTC()
	archiveEntry := types.ConversationArchiveEntry{
		ID:             types.NewID("archive"),
		SessionID:      sessionID,
		RangeLabel:     extraction.RangeLabel,
		TurnStart:      1,
		TurnEnd:        cutoff,
		ItemCount:      len(archiveItems),
		Summary:        extraction.Summary,
		Decisions:      extraction.Decisions,
		FilesChanged:   extraction.FilesChanged,
		ErrorsAndFixes: extraction.ErrorsAndFixes,
		ToolsUsed:      extraction.ToolsUsed,
		Keywords:       extraction.Keywords,
		IsComputed:     extraction.IsComputed,
		CreatedAt:      createdAt.Format(time.RFC3339Nano),
	}
	if err := e.store.InsertConversationArchiveEntry(ctx, archiveEntry); err != nil {
		return contextstate.WorkingSet{}, SummaryBundle{}, err
	}
	workspaceID := strings.TrimSpace(workspaceRoot)
	if workspaceID == "" {
		workspaceID = sessionID
	}
	if err := e.store.InsertColdIndexEntry(ctx, types.ColdIndexEntry{
		ID:           "cold_archive_" + archiveEntry.ID,
		WorkspaceID:  workspaceID,
		OwnerRoleID:  rolectx.SpecialistRoleIDFromContext(ctx),
		Visibility:   types.MemoryVisibilityShared,
		SourceType:   "archive",
		SourceID:     archiveEntry.ID,
		SearchText:   buildColdSearchText(extraction),
		SummaryLine:  buildColdSummaryLine(extraction),
		FilesChanged: extraction.FilesChanged,
		ToolsUsed:    extraction.ToolsUsed,
		ErrorTypes:   extractColdErrorTypes(extraction.ErrorsAndFixes),
		OccurredAt:   createdAt,
		CreatedAt:    createdAt,
		ContextRef: types.ColdContextRef{
			SessionID:     sessionID,
			ContextHeadID: contextHeadID,
			TurnStartPos:  startPosition,
			TurnEndPos:    endPosition,
			ItemCount:     len(archiveItems),
		},
	}); err != nil {
		return contextstate.WorkingSet{}, SummaryBundle{}, err
	}

	providerProfile := providerProfileForEngine(e)
	if err := e.store.InsertConversationCompactionWithContextHead(ctx, types.ConversationCompaction{
		ID:             types.NewID("compact"),
		SessionID:      sessionID,
		ContextHeadID:  contextHeadID,
		Kind:           types.ConversationCompactionKindArchive,
		Generation:     generation,
		StartItemID:    startItemID,
		EndItemID:      endItemID,
		StartPosition:  startPosition,
		EndPosition:    endPosition,
		SummaryPayload: marshalArchiveExtraction(extraction),
		MetadataJSON: encodeBoundaryMetadata(newBoundaryMetadata(
			generation,
			cutoff,
			contextHeadSummaryUpTo,
			len(items),
			reason,
			providerProfile,
			len(activeMicrocompactItems(compactions)) > 0,
			items,
		)),
		Reason:          reason,
		ProviderProfile: providerProfile,
		CreatedAt:       createdAt,
	}); err != nil {
		return contextstate.WorkingSet{}, SummaryBundle{}, err
	}

	if e.ctxManager != nil {
		e.ctxManager.SetCircuitBreakerOpen(false)
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
	if e.ctxManager != nil {
		maxBatch := e.ctxManager.Config().MaxCompactionBatchItems
		lastCompactedEnd := lastBoundaryCompactionEnd(compactions)
		maxCompactionEnd := lastCompactedEnd + maxBatch
		if maxBatch > 0 && cutoff > maxCompactionEnd {
			cutoff = maxCompactionEnd
		}
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

func marshalArchiveExtraction(extraction contextstate.ArchiveExtraction) string {
	toolOutcomes := make([]string, 0, len(extraction.ToolsUsed)+len(extraction.ErrorsAndFixes))
	for _, tool := range extraction.ToolsUsed {
		tool = strings.TrimSpace(tool)
		if tool != "" {
			toolOutcomes = append(toolOutcomes, "Tool used: "+tool)
		}
	}
	for _, note := range extraction.ErrorsAndFixes {
		note = strings.TrimSpace(note)
		if note != "" {
			toolOutcomes = append(toolOutcomes, "Error/fix: "+note)
		}
	}

	openThreads := make([]string, 0, 2)
	if summary := strings.TrimSpace(extraction.Summary); summary != "" {
		openThreads = append(openThreads, "Archive summary: "+summary)
	}
	if len(extraction.Keywords) > 0 {
		openThreads = append(openThreads, "Keywords: "+strings.Join(extraction.Keywords, ", "))
	}

	return marshalCompactionSummary(model.Summary{
		RangeLabel:       extraction.RangeLabel,
		ImportantChoices: extraction.Decisions,
		FilesTouched:     extraction.FilesChanged,
		ToolOutcomes:     toolOutcomes,
		OpenThreads:      openThreads,
	})
}

func buildColdSearchText(extraction contextstate.ArchiveExtraction) string {
	parts := []string{
		extraction.RangeLabel,
		extraction.Summary,
		strings.Join(extraction.Decisions, " "),
		strings.Join(extraction.FilesChanged, " "),
		strings.Join(extraction.ToolsUsed, " "),
		strings.Join(extraction.Keywords, " "),
		strings.Join(extraction.ErrorsAndFixes, " "),
	}
	return strings.Join(parts, " ")
}

func buildColdSummaryLine(extraction contextstate.ArchiveExtraction) string {
	summary := truncateColdSummary(extraction.Summary, 200)
	rangeLabel := strings.TrimSpace(extraction.RangeLabel)
	if rangeLabel == "" {
		return summary
	}
	if summary == "" {
		return "[" + rangeLabel + "]"
	}
	return "[" + rangeLabel + "] " + summary
}

func truncateColdSummary(value string, maxRunes int) string {
	value = strings.TrimSpace(value)
	if maxRunes <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= maxRunes {
		return value
	}
	return strings.TrimSpace(string(runes[:maxRunes])) + "..."
}

func extractColdErrorTypes(errorsAndFixes []string) []string {
	patterns := map[string][]string{
		"permission_denied": {"permission denied", "access denied", "not permitted", "unauthorized", "forbidden"},
		"timeout":           {"timeout", "timed out", "deadline exceeded"},
		"not_found":         {"not found", "no such file", "does not exist", "missing"},
		"failed":            {"failed", "failure", "error"},
		"build_failed":      {"build failed", "compile failed", "compilation failed"},
		"test_failed":       {"test failed", "tests failed", "failing test"},
	}
	seen := map[string]struct{}{}
	var out []string
	for _, note := range errorsAndFixes {
		note = strings.ToLower(strings.TrimSpace(note))
		if note == "" {
			continue
		}
		for errorType, needles := range patterns {
			for _, needle := range needles {
				if strings.Contains(note, needle) {
					if _, ok := seen[errorType]; !ok {
						seen[errorType] = struct{}{}
						out = append(out, errorType)
					}
					break
				}
			}
		}
	}
	return out
}

func providerProfileForEngine(e *Engine) string {
	if e == nil || e.model == nil {
		return ""
	}
	return string(e.model.Capabilities().Profile)
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
