package sqlite

import (
	"context"
	"testing"
	"time"

	"go-agent/internal/types"
)

func TestMarkMemoryEntriesUsedDedupesAndBumps(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	old := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	usedAt := time.Date(2026, 2, 3, 4, 5, 6, 7, time.UTC)

	for _, entry := range []types.MemoryEntry{
		{
			ID:          "mem_1",
			Scope:       types.MemoryScopeWorkspace,
			WorkspaceID: "workspace",
			Kind:        types.MemoryKindWorkspaceChoice,
			Content:     "choice one",
			SourceRefs:  []string{"source:one"},
			Confidence:  0.9,
			LastUsedAt:  old,
			UsageCount:  2,
			CreatedAt:   old,
			UpdatedAt:   old,
		},
		{
			ID:          "mem_2",
			Scope:       types.MemoryScopeWorkspace,
			WorkspaceID: "workspace",
			Kind:        types.MemoryKindWorkspaceChoice,
			Content:     "choice two",
			SourceRefs:  []string{"source:two"},
			Confidence:  0.8,
			LastUsedAt:  old,
			CreatedAt:   old,
			UpdatedAt:   old,
		},
	} {
		if err := store.UpsertMemoryEntry(ctx, entry); err != nil {
			t.Fatalf("UpsertMemoryEntry(%s) error = %v", entry.ID, err)
		}
	}

	if err := store.MarkMemoryEntriesUsed(ctx, []string{"mem_1", " mem_1 ", "", "mem_2"}, usedAt); err != nil {
		t.Fatalf("MarkMemoryEntriesUsed() error = %v", err)
	}

	entries, err := store.ListVisibleMemoryEntries(ctx, "workspace", "")
	if err != nil {
		t.Fatalf("ListVisibleMemoryEntries() error = %v", err)
	}
	got := make(map[string]types.MemoryEntry, len(entries))
	for _, entry := range entries {
		got[entry.ID] = entry
	}

	if got["mem_1"].UsageCount != 3 {
		t.Fatalf("mem_1 UsageCount = %d, want 3", got["mem_1"].UsageCount)
	}
	if !got["mem_1"].LastUsedAt.Equal(usedAt) {
		t.Fatalf("mem_1 LastUsedAt = %s, want %s", got["mem_1"].LastUsedAt, usedAt)
	}
	if got["mem_2"].UsageCount != 1 {
		t.Fatalf("mem_2 UsageCount = %d, want 1", got["mem_2"].UsageCount)
	}
	if !got["mem_2"].LastUsedAt.Equal(usedAt) {
		t.Fatalf("mem_2 LastUsedAt = %s, want %s", got["mem_2"].LastUsedAt, usedAt)
	}
}
