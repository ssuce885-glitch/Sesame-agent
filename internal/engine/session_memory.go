package engine

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"

	contextstate "go-agent/internal/context"
	"go-agent/internal/memory"
	"go-agent/internal/model"
	"go-agent/internal/types"
)

const (
	sessionMemoryRangeLabel          = "session memory"
	sessionMemoryBootstrapMinItems   = 8
	sessionMemoryBootstrapMinTokens  = 320
	sessionMemoryUpdateMinItems      = 6
	sessionMemoryUpdateMinTokens     = 240
	sessionMemorySignalThreshold     = 2
	sessionMemoryLongAssistantChars  = 320
	sessionMemorySummaryMaxCount     = 1
	conversationSummaryMaxCount      = 2
	rollingSummaryMaxCount           = conversationSummaryMaxCount
	workspaceDetailRecallMaxCount    = 2
	globalMemoryRecallMaxCount       = 1
	durableWorkspaceDetailCapPerKind = 4
	durableGlobalMemoryCap           = 2
	sessionMemorySummaryTokenBudget  = 256
	conversationSummaryTokenBudget   = 256
	boundarySummaryTokenBudget       = conversationSummaryTokenBudget
	rollingSummaryTokenBudget        = conversationSummaryTokenBudget
	workspaceOverviewTokenBudget     = 192
	workspaceDetailTokenBudget       = 224
	globalMemoryTokenBudget          = 128
	memoryRecallCandidateLimit       = 8
)

type headMemoryRefreshReport struct {
	Updated                  bool
	WorkspaceEntriesUpserted int
	GlobalEntriesUpserted    int
	WorkspaceEntriesPruned   int
}

type injectedMemoryRefKind string

const (
	injectedMemoryRefWorkspaceOverview injectedMemoryRefKind = "workspace_overview"
	injectedMemoryRefWorkspaceDetail   injectedMemoryRefKind = "workspace_detail"
	injectedMemoryRefGlobal            injectedMemoryRefKind = "global"
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

func durableWorkspaceMemoryID(workspaceRoot string) string {
	return durableWorkspaceOverviewID(workspaceRoot)
}

func durableWorkspaceMemoryPrefix(workspaceRoot string) string {
	workspaceRoot = strings.TrimSpace(workspaceRoot)
	if workspaceRoot == "" {
		return ""
	}
	sum := sha1.Sum([]byte(workspaceRoot))
	return "mem_workspace_" + hex.EncodeToString(sum[:8])
}

func findMemoryEntry(entries []types.MemoryEntry, scope types.MemoryScope, kind types.MemoryKind, workspaceRoot string) (types.MemoryEntry, bool) {
	for _, entry := range entries {
		if entry.Scope != scope || entry.Kind != kind {
			continue
		}
		if scope == types.MemoryScopeWorkspace && strings.TrimSpace(entry.WorkspaceID) != strings.TrimSpace(workspaceRoot) {
			continue
		}
		if strings.TrimSpace(entry.Content) == "" {
			continue
		}
		return entry, true
	}
	return types.MemoryEntry{}, false
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

func buildMemoryRefs(entries []types.MemoryEntry, sessionMemoryPresent bool, workspaceRoot string, query string) []string {
	recalled := memory.Recall(query, entries, memoryRecallCandidateLimit)
	out := make([]string, 0, 1+workspaceDetailRecallMaxCount+globalMemoryRecallMaxCount)
	seen := map[string]struct{}{}
	workspaceDetailTokens := 0
	globalTokens := 0
	workspaceDetailCount := 0
	globalCount := 0

	if !sessionMemoryPresent {
		if workspaceMemory, ok := findMemoryEntry(entries, types.MemoryScopeWorkspace, types.MemoryKindWorkspaceOverview, workspaceRoot); ok {
			ref := strings.TrimSpace(workspaceMemory.Content)
			if allowMemoryRef(ref, seen, 0, workspaceOverviewTokenBudget) {
				out = append(out, ref)
			}
		}
	}

	for _, entry := range recalled {
		ref := strings.TrimSpace(entry.Content)
		if ref == "" {
			continue
		}

		switch {
		case entry.Scope == types.MemoryScopeWorkspace && entry.Kind == types.MemoryKindWorkspaceOverview:
			if sessionMemoryPresent {
				continue
			}
			if allowMemoryRef(ref, seen, 0, workspaceOverviewTokenBudget) {
				out = append(out, ref)
			}
		case entry.Scope == types.MemoryScopeGlobal && entry.Kind == types.MemoryKindGlobalPreference:
			if globalCount >= globalMemoryRecallMaxCount {
				continue
			}
			cost := estimateMemoryRefTokens(ref)
			if globalCount > 0 && globalMemoryTokenBudget > 0 && globalTokens+cost > globalMemoryTokenBudget {
				continue
			}
			if _, ok := seen[ref]; ok {
				continue
			}
			seen[ref] = struct{}{}
			out = append(out, ref)
			globalCount++
			globalTokens += cost
		case entry.Scope == types.MemoryScopeWorkspace && isWorkspaceDetailMemoryKind(entry.Kind):
			if workspaceDetailCount >= workspaceDetailRecallMaxCount {
				continue
			}
			cost := estimateMemoryRefTokens(ref)
			if workspaceDetailCount > 0 && workspaceDetailTokenBudget > 0 && workspaceDetailTokens+cost > workspaceDetailTokenBudget {
				continue
			}
			if _, ok := seen[ref]; ok {
				continue
			}
			seen[ref] = struct{}{}
			out = append(out, ref)
			workspaceDetailCount++
			workspaceDetailTokens += cost
		}
	}

	return dedupeSummaryStrings(out)
}

func allowMemoryRef(ref string, seen map[string]struct{}, usedTokens int, tokenBudget int) bool {
	if ref == "" {
		return false
	}
	if _, ok := seen[ref]; ok {
		return false
	}
	cost := estimateMemoryRefTokens(ref)
	if len(seen) > 0 && tokenBudget > 0 && usedTokens+cost > tokenBudget {
		return false
	}
	seen[ref] = struct{}{}
	return true
}

func estimateSummaryInjectionTokens(summary model.Summary) int {
	return contextstate.EstimatePromptTokens("", nil, SummaryBundle{Rolling: []model.Summary{summary}}, nil)
}

func estimateMemoryRefTokens(ref string) int {
	return contextstate.EstimatePromptTokens("", nil, SummaryBundle{}, []string{ref})
}

func buildWorkspaceDurableMemory(memory types.HeadMemory, summary model.Summary) (types.MemoryEntry, bool) {
	workspaceRoot := strings.TrimSpace(memory.WorkspaceRoot)
	if workspaceRoot == "" || isZeroSummary(summary) {
		return types.MemoryEntry{}, false
	}

	content := formatWorkspaceDurableMemory(summary)
	if content == "" {
		return types.MemoryEntry{}, false
	}

	now := time.Now().UTC()
	return types.MemoryEntry{
		ID:                  durableWorkspaceOverviewID(workspaceRoot),
		Scope:               types.MemoryScopeWorkspace,
		Kind:                types.MemoryKindWorkspaceOverview,
		WorkspaceID:         workspaceRoot,
		SourceSessionID:     memory.SessionID,
		SourceContextHeadID: memory.ContextHeadID,
		Content:             content,
		SourceRefs:          dedupeSummaryStrings([]string{"session:" + memory.SessionID, "head:" + memory.ContextHeadID, "turn:" + memory.SourceTurnID}),
		Confidence:          0.85,
		CreatedAt:           now,
		UpdatedAt:           now,
	}, true
}

func buildWorkspaceDetailMemories(memoryRecord types.HeadMemory, summary model.Summary) []types.MemoryEntry {
	workspaceRoot := strings.TrimSpace(memoryRecord.WorkspaceRoot)
	if workspaceRoot == "" || isZeroSummary(summary) {
		return nil
	}

	type bucket struct {
		kind   string
		prefix string
		values []string
	}
	buckets := []bucket{
		{kind: "choice", prefix: "[Workspace durable memory] Choice: ", values: summary.ImportantChoices},
		{kind: "file", prefix: "[Workspace durable memory] File focus: ", values: summary.FilesTouched},
		{kind: "thread", prefix: "[Workspace durable memory] Open thread: ", values: summary.OpenThreads},
		{kind: "tool", prefix: "[Workspace durable memory] Tool outcome: ", values: summary.ToolOutcomes},
	}

	now := time.Now().UTC()
	out := make([]types.MemoryEntry, 0, 8)
	for _, bucket := range buckets {
		values := dedupeSummaryStrings(bucket.values)
		if len(values) > durableWorkspaceDetailCapPerKind {
			values = values[:durableWorkspaceDetailCapPerKind]
		}
		for _, value := range values {
			content := strings.TrimSpace(value)
			if content == "" {
				continue
			}
			out = append(out, types.MemoryEntry{
				ID:                  durableWorkspaceDetailID(workspaceRoot, bucket.kind, content),
				Scope:               types.MemoryScopeWorkspace,
				Kind:                durableWorkspaceDetailKind(bucket.kind),
				WorkspaceID:         workspaceRoot,
				SourceSessionID:     memoryRecord.SessionID,
				SourceContextHeadID: memoryRecord.ContextHeadID,
				Content:             bucket.prefix + content,
				SourceRefs:          dedupeSummaryStrings([]string{"session:" + memoryRecord.SessionID, "head:" + memoryRecord.ContextHeadID, "turn:" + memoryRecord.SourceTurnID}),
				Confidence:          0.8,
				CreatedAt:           now,
				UpdatedAt:           now,
			})
		}
	}
	return out
}

func formatWorkspaceDurableMemory(summary model.Summary) string {
	summary = normalizeSummaryForPrompt(summary)
	parts := make([]string, 0, 5)
	if len(summary.UserGoals) > 0 {
		parts = append(parts, "Goals: "+strings.Join(summary.UserGoals, "; "))
	}
	if len(summary.ImportantChoices) > 0 {
		parts = append(parts, "Choices: "+strings.Join(summary.ImportantChoices, "; "))
	}
	if len(summary.FilesTouched) > 0 {
		parts = append(parts, "Files: "+strings.Join(summary.FilesTouched, "; "))
	}
	if len(summary.ToolOutcomes) > 0 {
		parts = append(parts, "Tool outcomes: "+strings.Join(summary.ToolOutcomes, "; "))
	}
	if len(summary.OpenThreads) > 0 {
		parts = append(parts, "Open threads: "+strings.Join(summary.OpenThreads, "; "))
	}
	if len(parts) == 0 {
		return ""
	}
	return "[Workspace durable memory]\n" + strings.Join(parts, "\n")
}

func durableWorkspaceOverviewID(workspaceRoot string) string {
	prefix := durableWorkspaceMemoryPrefix(workspaceRoot)
	if prefix == "" {
		return ""
	}
	return prefix + "_overview"
}

func durableWorkspaceDetailID(workspaceRoot string, kind string, content string) string {
	prefix := durableWorkspaceMemoryPrefix(workspaceRoot)
	if prefix == "" {
		return ""
	}
	sum := sha1.Sum([]byte(strings.TrimSpace(strings.ToLower(content))))
	return prefix + "_" + kind + "_" + hex.EncodeToString(sum[:6])
}

func durableWorkspaceDetailKind(kind string) types.MemoryKind {
	switch strings.TrimSpace(kind) {
	case "choice":
		return types.MemoryKindWorkspaceChoice
	case "file":
		return types.MemoryKindWorkspaceFileFocus
	case "thread":
		return types.MemoryKindWorkspaceOpenThread
	case "tool":
		return types.MemoryKindWorkspaceToolOutcome
	default:
		return ""
	}
}

func isWorkspaceDurableMemoryEntry(entry types.MemoryEntry, workspaceRoot string) bool {
	prefix := durableWorkspaceMemoryPrefix(workspaceRoot)
	if prefix == "" {
		return false
	}
	if entry.Scope != types.MemoryScopeWorkspace {
		return false
	}
	return strings.HasPrefix(entry.ID, prefix)
}

func isWorkspaceDetailMemoryKind(kind types.MemoryKind) bool {
	switch kind {
	case types.MemoryKindWorkspaceChoice,
		types.MemoryKindWorkspaceFileFocus,
		types.MemoryKindWorkspaceOpenThread,
		types.MemoryKindWorkspaceToolOutcome:
		return true
	default:
		return false
	}
}

func buildGlobalDurableMemories(memoryRecord types.HeadMemory, summary model.Summary) []types.MemoryEntry {
	candidates := durableGlobalCandidates(summary)
	if len(candidates) == 0 {
		return nil
	}
	if len(candidates) > durableGlobalMemoryCap {
		candidates = candidates[:durableGlobalMemoryCap]
	}

	now := time.Now().UTC()
	out := make([]types.MemoryEntry, 0, len(candidates))
	for _, candidate := range candidates {
		if memory.Classify(memory.Candidate{Content: candidate}) != types.MemoryScopeGlobal {
			continue
		}
		out = append(out, types.MemoryEntry{
			ID:                  durableGlobalMemoryID(candidate),
			Scope:               types.MemoryScopeGlobal,
			Kind:                types.MemoryKindGlobalPreference,
			WorkspaceID:         "",
			SourceSessionID:     memoryRecord.SessionID,
			SourceContextHeadID: memoryRecord.ContextHeadID,
			Content:             "[Global durable memory] " + candidate,
			SourceRefs:          dedupeSummaryStrings([]string{"session:" + memoryRecord.SessionID, "head:" + memoryRecord.ContextHeadID, "turn:" + memoryRecord.SourceTurnID}),
			Confidence:          0.9,
			CreatedAt:           now,
			UpdatedAt:           now,
		})
	}
	return out
}

func pruneWorkspaceDurableMemories(ctx context.Context, store ConversationStore, workspaceRoot string, desired []types.MemoryEntry) (int, error) {
	workspaceRoot = strings.TrimSpace(workspaceRoot)
	if store == nil || workspaceRoot == "" {
		return 0, nil
	}

	existing, err := store.ListMemoryEntriesByWorkspace(ctx, workspaceRoot)
	if err != nil {
		return 0, err
	}

	desiredIDs := make(map[string]struct{}, len(desired))
	for _, entry := range desired {
		if strings.TrimSpace(entry.ID) == "" {
			continue
		}
		desiredIDs[entry.ID] = struct{}{}
	}

	stale := make([]string, 0, len(existing))
	for _, entry := range existing {
		if !isWorkspaceDurableMemoryEntry(entry, workspaceRoot) {
			continue
		}
		if _, ok := desiredIDs[entry.ID]; ok {
			continue
		}
		stale = append(stale, entry.ID)
	}
	if len(stale) == 0 {
		return 0, nil
	}
	if err := store.DeleteMemoryEntries(ctx, stale); err != nil {
		return 0, err
	}
	return len(stale), nil
}

func durableGlobalCandidates(summary model.Summary) []string {
	summary = normalizeSummaryForPrompt(summary)
	candidates := make([]string, 0, len(summary.ImportantChoices)+len(summary.UserGoals))
	candidates = append(candidates, summary.ImportantChoices...)
	candidates = append(candidates, summary.UserGoals...)
	return dedupeSummaryStrings(candidates)
}

func durableGlobalMemoryID(content string) string {
	content = strings.TrimSpace(strings.ToLower(content))
	if content == "" {
		return ""
	}
	sum := sha1.Sum([]byte(content))
	return "mem_global_" + hex.EncodeToString(sum[:8])
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

func encodeSessionMemorySummary(summary model.Summary) string {
	raw, err := json.Marshal(summary)
	if err != nil {
		return ""
	}
	return string(raw)
}

func decodeSessionMemorySummary(raw string) (model.Summary, bool, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return model.Summary{}, false, nil
	}

	var summary model.Summary
	if err := json.Unmarshal([]byte(raw), &summary); err != nil {
		return model.Summary{
			RangeLabel:  sessionMemoryRangeLabel,
			OpenThreads: []string{raw},
		}, true, nil
	}
	if strings.TrimSpace(summary.RangeLabel) == "" {
		summary.RangeLabel = sessionMemoryRangeLabel
	}
	return summary, true, nil
}

type compactionSummaryPayload struct {
	RangeLabel       string   `json:"range_label"`
	UserGoals        []string `json:"user_goals"`
	ImportantChoices []string `json:"important_choices"`
	FilesTouched     []string `json:"files_touched"`
	ToolOutcomes     []string `json:"tool_outcomes"`
	OpenThreads      []string `json:"open_threads"`
}

func decodeCompactionSummaryPayload(raw string) (model.Summary, bool, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return model.Summary{}, false, nil
	}

	var standard model.Summary
	if err := json.Unmarshal([]byte(raw), &standard); err != nil {
		return model.Summary{}, false, err
	}

	var snake compactionSummaryPayload
	if err := json.Unmarshal([]byte(raw), &snake); err != nil {
		return model.Summary{}, false, err
	}

	summary := standard
	if strings.TrimSpace(summary.RangeLabel) == "" {
		summary.RangeLabel = snake.RangeLabel
	}
	if len(summary.UserGoals) == 0 {
		summary.UserGoals = append([]string(nil), snake.UserGoals...)
	}
	if len(summary.ImportantChoices) == 0 {
		summary.ImportantChoices = append([]string(nil), snake.ImportantChoices...)
	}
	if len(summary.FilesTouched) == 0 {
		summary.FilesTouched = append([]string(nil), snake.FilesTouched...)
	}
	if len(summary.ToolOutcomes) == 0 {
		summary.ToolOutcomes = append([]string(nil), snake.ToolOutcomes...)
	}
	if len(summary.OpenThreads) == 0 {
		summary.OpenThreads = append([]string(nil), snake.OpenThreads...)
	}

	if isZeroSummary(summary) {
		return model.Summary{}, false, nil
	}
	return summary, true, nil
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
