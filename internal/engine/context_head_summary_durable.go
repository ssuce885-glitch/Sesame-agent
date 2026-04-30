package engine

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"strings"
	"time"

	contextstate "go-agent/internal/context"
	"go-agent/internal/memory"
	"go-agent/internal/model"
	"go-agent/internal/types"
)

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

func buildMemoryRefsAndUsage(entries []types.MemoryEntry, contextHeadSummaryPresent bool, workspaceRoot string, query string) ([]string, []string) {
	recalled := memory.Recall(query, entries, memoryRecallCandidateLimit)
	out := make([]string, 0, 1+workspaceDetailRecallMaxCount+globalMemoryRecallMaxCount)
	seen := map[string]struct{}{}
	usedMemoryIDs := make([]string, 0, len(recalled)+1)
	seenMemoryIDs := map[string]struct{}{}
	workspaceDetailTokens := 0
	globalTokens := 0
	workspaceDetailCount := 0
	globalCount := 0

	if !contextHeadSummaryPresent {
		if workspaceMemory, ok := findMemoryEntry(entries, types.MemoryScopeWorkspace, types.MemoryKindWorkspaceOverview, workspaceRoot); ok {
			ref := strings.TrimSpace(workspaceMemory.Content)
			if allowMemoryRef(ref, seen, 0, workspaceOverviewTokenBudget) {
				out = append(out, ref)
				usedMemoryIDs = appendMemoryUsageID(usedMemoryIDs, seenMemoryIDs, workspaceMemory)
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
			if contextHeadSummaryPresent {
				continue
			}
			if allowMemoryRef(ref, seen, 0, workspaceOverviewTokenBudget) {
				out = append(out, ref)
				usedMemoryIDs = appendMemoryUsageID(usedMemoryIDs, seenMemoryIDs, entry)
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
			usedMemoryIDs = appendMemoryUsageID(usedMemoryIDs, seenMemoryIDs, entry)
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
			usedMemoryIDs = appendMemoryUsageID(usedMemoryIDs, seenMemoryIDs, entry)
			workspaceDetailCount++
			workspaceDetailTokens += cost
		}
	}

	return dedupeSummaryStrings(out), usedMemoryIDs
}

func appendMemoryUsageID(ids []string, seen map[string]struct{}, entry types.MemoryEntry) []string {
	id := strings.TrimSpace(entry.ID)
	if id == "" {
		return ids
	}
	if _, ok := seen[id]; ok {
		return ids
	}
	seen[id] = struct{}{}
	return append(ids, id)
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

func buildWorkspaceDurableMemory(memoryRecord types.ContextHeadSummary, summary model.Summary, roleID string) (types.MemoryEntry, bool) {
	workspaceRoot := strings.TrimSpace(memoryRecord.WorkspaceRoot)
	if workspaceRoot == "" || isZeroSummary(summary) {
		return types.MemoryEntry{}, false
	}

	content := formatWorkspaceDurableMemory(summary)
	if content == "" {
		return types.MemoryEntry{}, false
	}

	now := time.Now().UTC()
	entry := types.MemoryEntry{
		ID:                  durableWorkspaceOverviewID(workspaceRoot, roleID),
		Scope:               types.MemoryScopeWorkspace,
		Kind:                types.MemoryKindWorkspaceOverview,
		WorkspaceID:         workspaceRoot,
		SourceSessionID:     memoryRecord.SessionID,
		SourceContextHeadID: memoryRecord.ContextHeadID,
		OwnerRoleID:         roleID,
		Visibility:          types.MemoryVisibilityShared,
		Status:              types.MemoryStatusActive,
		Content:             content,
		SourceRefs:          dedupeSummaryStrings([]string{"session:" + memoryRecord.SessionID, "head:" + memoryRecord.ContextHeadID, "turn:" + memoryRecord.SourceTurnID}),
		Confidence:          0,
		CreatedAt:           now,
		UpdatedAt:           now,
	}
	entry.Confidence = memory.ComputeConfidence(entry, now)
	return entry, true
}

func buildWorkspaceDetailMemories(memoryRecord types.ContextHeadSummary, summary model.Summary, roleID string) []types.MemoryEntry {
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
			detailVisibility := types.MemoryVisibilityShared
			if roleID != "" {
				// Detail memories (file focus, open threads, tool outcomes) are
				// role-private by default: a specialist's internal observations
				// should not leak into another role's context. The workspace
				// overview (buildWorkspaceDurableMemory) remains shared so that
				// main_parent and peer roles see a summary, but not raw details.
				detailVisibility = types.MemoryVisibilityPrivate
			}
			entry := types.MemoryEntry{
				ID:                  durableWorkspaceDetailID(workspaceRoot, roleID, bucket.kind, content),
				Scope:               types.MemoryScopeWorkspace,
				Kind:                durableWorkspaceDetailKind(bucket.kind),
				WorkspaceID:         workspaceRoot,
				SourceSessionID:     memoryRecord.SessionID,
				SourceContextHeadID: memoryRecord.ContextHeadID,
				OwnerRoleID:         roleID,
				Visibility:          detailVisibility,
				Status:              types.MemoryStatusActive,
				Content:             bucket.prefix + content,
				SourceRefs:          dedupeSummaryStrings([]string{"session:" + memoryRecord.SessionID, "head:" + memoryRecord.ContextHeadID, "turn:" + memoryRecord.SourceTurnID}),
				Confidence:          0,
				CreatedAt:           now,
				UpdatedAt:           now,
			}
			entry.Confidence = memory.ComputeConfidence(entry, now)
			out = append(out, entry)
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

func durableWorkspaceOverviewID(workspaceRoot string, roleID string) string {
	prefix := durableWorkspaceOwnerPrefix(workspaceRoot, roleID)
	if prefix == "" {
		return ""
	}
	return prefix + "_overview"
}

func durableWorkspaceDetailID(workspaceRoot string, roleID string, kind string, content string) string {
	prefix := durableWorkspaceOwnerPrefix(workspaceRoot, roleID)
	if prefix == "" {
		return ""
	}
	sum := sha1.Sum([]byte(strings.TrimSpace(strings.ToLower(content))))
	return prefix + "_" + kind + "_" + hex.EncodeToString(sum[:6])
}

func durableWorkspaceOwnerPrefix(workspaceRoot string, roleID string) string {
	prefix := durableWorkspaceMemoryPrefix(workspaceRoot)
	if prefix == "" {
		return ""
	}
	roleID = strings.TrimSpace(roleID)
	if roleID == "" {
		return prefix
	}
	sum := sha1.Sum([]byte(roleID))
	return prefix + "_role_" + hex.EncodeToString(sum[:6])
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

func buildGlobalDurableMemories(memoryRecord types.ContextHeadSummary, summary model.Summary, roleID string) []types.MemoryEntry {
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
		entry := types.MemoryEntry{
			ID:                  durableGlobalMemoryID(candidate),
			Scope:               types.MemoryScopeGlobal,
			Kind:                types.MemoryKindGlobalPreference,
			WorkspaceID:         "",
			SourceSessionID:     memoryRecord.SessionID,
			SourceContextHeadID: memoryRecord.ContextHeadID,
			OwnerRoleID:         "",
			Visibility:          types.MemoryVisibilityShared,
			Status:              types.MemoryStatusActive,
			Content:             "[Global durable memory] " + candidate,
			SourceRefs:          dedupeSummaryStrings([]string{"session:" + memoryRecord.SessionID, "head:" + memoryRecord.ContextHeadID, "turn:" + memoryRecord.SourceTurnID}),
			Confidence:          0,
			CreatedAt:           now,
			UpdatedAt:           now,
		}
		entry.Confidence = memory.ComputeConfidence(entry, now)
		out = append(out, entry)
	}
	return out
}

type visibleMemoryStore interface {
	ListVisibleMemoryEntries(context.Context, string, string) ([]types.MemoryEntry, error)
	DeleteMemoryEntries(context.Context, []string) error
}

func pruneWorkspaceDurableMemories(ctx context.Context, store visibleMemoryStore, workspaceRoot, roleID string, desired []types.MemoryEntry) (int, error) {
	workspaceRoot = strings.TrimSpace(workspaceRoot)
	if store == nil || workspaceRoot == "" {
		return 0, nil
	}

	existing, err := store.ListVisibleMemoryEntries(ctx, workspaceRoot, roleID)
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
		if strings.TrimSpace(entry.OwnerRoleID) != strings.TrimSpace(roleID) {
			continue
		}
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
