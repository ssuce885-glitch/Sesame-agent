package sqlite

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"go-agent/internal/types"
)

func TestColdIndexSearchUsesFTSFiltersAndVisibility(t *testing.T) {
	ctx := context.Background()
	store, err := Open(filepath.Join(t.TempDir(), "sesame.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	occurredAt := time.Now().UTC().Add(-2 * time.Hour)
	entry := types.ColdIndexEntry{
		ID:           "cold_1",
		WorkspaceID:  "/workspace",
		OwnerRoleID:  "research",
		Visibility:   types.MemoryVisibilityPrivate,
		SourceType:   "archive",
		SourceID:     "archive_1",
		SearchText:   "disk cleanup failed with permission denied in internal/store/sqlite/cold_index.go",
		SummaryLine:  "[turns 1-4] Fixed disk cleanup permission failure",
		FilesChanged: []string{"internal/store/sqlite/cold_index.go"},
		ToolsUsed:    []string{"go test"},
		ErrorTypes:   []string{"permission_denied"},
		OccurredAt:   occurredAt,
		CreatedAt:    occurredAt,
		ContextRef: types.ColdContextRef{
			SessionID:     "session_1",
			ContextHeadID: "head_1",
			TurnStartPos:  0,
			TurnEndPos:    2,
			ItemCount:     2,
		},
	}
	if err := store.InsertColdIndexEntry(ctx, entry); err != nil {
		t.Fatalf("InsertColdIndexEntry() error = %v", err)
	}

	mainEntries, mainTotal, err := store.SearchColdIndex(ctx, types.ColdSearchQuery{
		WorkspaceID: "/workspace",
		TextQuery:   "disk cleanup",
		Limit:       10,
	})
	if err != nil {
		t.Fatalf("SearchColdIndex(main) error = %v", err)
	}
	if mainTotal != 0 || len(mainEntries) != 0 {
		t.Fatalf("main search returned %d/%d private role entries, want none", len(mainEntries), mainTotal)
	}

	roleEntries, roleTotal, err := store.SearchColdIndex(ctx, types.ColdSearchQuery{
		WorkspaceID:  "/workspace",
		RoleID:       "research",
		TextQuery:    "disk cleanup",
		FilesTouched: []string{"internal/store/sqlite/cold_index.go"},
		ToolsUsed:    []string{"go test"},
		ErrorTypes:   []string{"permission_denied"},
		SourceTypes:  []string{"archive"},
		Since:        occurredAt.Add(-time.Hour),
		Until:        occurredAt.Add(time.Hour),
		Limit:        10,
	})
	if err != nil {
		t.Fatalf("SearchColdIndex(role) error = %v", err)
	}
	if roleTotal != 1 || len(roleEntries) != 1 {
		t.Fatalf("role search returned %d/%d entries, want 1/1", len(roleEntries), roleTotal)
	}
	if roleEntries[0].ID != entry.ID || roleEntries[0].ContextRef.SessionID != "session_1" {
		t.Fatalf("role search entry = %#v, want cold_1 with context ref", roleEntries[0])
	}
}

func TestInsertColdIndexEntryReplacesExistingEntryAndFTS(t *testing.T) {
	ctx := context.Background()
	store, err := Open(filepath.Join(t.TempDir(), "sesame.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	occurredAt := time.Now().UTC()
	entry := types.ColdIndexEntry{
		ID:          "cold_archive_archive_1",
		WorkspaceID: "/workspace",
		Visibility:  types.MemoryVisibilityShared,
		SourceType:  "archive",
		SourceID:    "archive_1",
		SearchText:  "obsolete alpha content",
		SummaryLine: "old summary",
		OccurredAt:  occurredAt,
		CreatedAt:   occurredAt,
	}
	if err := store.InsertColdIndexEntry(ctx, entry); err != nil {
		t.Fatalf("InsertColdIndexEntry(old) error = %v", err)
	}

	entry.SearchText = "fresh beta content"
	entry.SummaryLine = "new summary"
	if err := store.InsertColdIndexEntry(ctx, entry); err != nil {
		t.Fatalf("InsertColdIndexEntry(new) error = %v", err)
	}

	oldEntries, oldTotal, err := store.SearchColdIndex(ctx, types.ColdSearchQuery{
		WorkspaceID: "/workspace",
		TextQuery:   "alpha",
		Limit:       10,
	})
	if err != nil {
		t.Fatalf("SearchColdIndex(old) error = %v", err)
	}
	if oldTotal != 0 || len(oldEntries) != 0 {
		t.Fatalf("old search returned %d/%d entries, want 0/0", len(oldEntries), oldTotal)
	}

	newEntries, newTotal, err := store.SearchColdIndex(ctx, types.ColdSearchQuery{
		WorkspaceID: "/workspace",
		TextQuery:   "beta",
		Limit:       10,
	})
	if err != nil {
		t.Fatalf("SearchColdIndex(new) error = %v", err)
	}
	if newTotal != 1 || len(newEntries) != 1 {
		t.Fatalf("new search returned %d/%d entries, want 1/1", len(newEntries), newTotal)
	}
	if newEntries[0].ID != entry.ID || newEntries[0].SummaryLine != "new summary" {
		t.Fatalf("new search entry = %#v, want updated cold entry", newEntries[0])
	}
}
