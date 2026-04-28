package sqlite

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go-agent/internal/types"
)

func TestColdIndexVisibilityModel(t *testing.T) {
	ctx := context.Background()
	store, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	now := time.Now().UTC()
	entries := []types.ColdIndexEntry{
		{
			ID:          "cold_vis_a",
			WorkspaceID: "/ws",
			Visibility:  types.MemoryVisibilityShared,
			SourceType:  "archive",
			SourceID:    "a1",
			SearchText:  "shared main entry about deployment",
			SummaryLine: "shared main entry about deployment",
			OccurredAt:  now,
			CreatedAt:   now,
		},
		{
			ID:          "cold_vis_b",
			WorkspaceID: "/ws",
			OwnerRoleID: "research",
			Visibility:  types.MemoryVisibilityPrivate,
			SourceType:  "archive",
			SourceID:    "b1",
			SearchText:  "private research entry about deployment",
			SummaryLine: "private research entry about deployment",
			OccurredAt:  now,
			CreatedAt:   now,
		},
		{
			ID:          "cold_vis_c",
			WorkspaceID: "/ws",
			OwnerRoleID: "research",
			Visibility:  types.MemoryVisibilityShared,
			SourceType:  "archive",
			SourceID:    "c1",
			SearchText:  "shared research entry about deployment",
			SummaryLine: "shared research entry about deployment",
			OccurredAt:  now,
			CreatedAt:   now,
		},
		{
			ID:          "cold_vis_d",
			WorkspaceID: "/ws",
			Visibility:  types.MemoryVisibilityPromoted,
			SourceType:  "archive",
			SourceID:    "d1",
			SearchText:  "promoted main entry about deployment",
			SummaryLine: "promoted main entry about deployment",
			OccurredAt:  now,
			CreatedAt:   now,
		},
	}
	for _, entry := range entries {
		if err := store.InsertColdIndexEntry(ctx, entry); err != nil {
			t.Fatalf("InsertColdIndexEntry(%s) error = %v", entry.ID, err)
		}
	}

	mainIDs := assertColdVisibilitySearchIDs(t, ctx, store, "/ws", "", "cold_vis_a", "cold_vis_c", "cold_vis_d")
	assertColdVisibilityMissingID(t, mainIDs, "cold_vis_b")

	researchIDs := assertColdVisibilitySearchIDs(t, ctx, store, "/ws", "research", "cold_vis_a", "cold_vis_b", "cold_vis_c", "cold_vis_d")
	for _, id := range []string{"cold_vis_a", "cold_vis_b", "cold_vis_c", "cold_vis_d"} {
		assertColdVisibilityHasID(t, researchIDs, id)
	}

	otherIDs := assertColdVisibilitySearchIDs(t, ctx, store, "/ws", "other", "cold_vis_a", "cold_vis_c", "cold_vis_d")
	assertColdVisibilityMissingID(t, otherIDs, "cold_vis_b")

	entryE := types.ColdIndexEntry{
		ID:          "cold_vis_e",
		WorkspaceID: "/ws2",
		Visibility:  types.MemoryVisibilityShared,
		SourceType:  "archive",
		SourceID:    "e1",
		SearchText:  "deployment",
		SummaryLine: "deployment",
		OccurredAt:  now,
		CreatedAt:   now,
	}
	if err := store.InsertColdIndexEntry(ctx, entryE); err != nil {
		t.Fatalf("InsertColdIndexEntry(%s) error = %v", entryE.ID, err)
	}

	wsIDs := assertColdVisibilitySearchIDs(t, ctx, store, "/ws", "", "cold_vis_a", "cold_vis_c", "cold_vis_d")
	assertColdVisibilityMissingID(t, wsIDs, "cold_vis_e")

	ws2IDs := assertColdVisibilitySearchIDs(t, ctx, store, "/ws2", "", "cold_vis_e")
	assertColdVisibilityHasID(t, ws2IDs, "cold_vis_e")
}

func assertColdVisibilitySearchIDs(t *testing.T, ctx context.Context, store *Store, workspaceID, roleID string, wantIDs ...string) map[string]struct{} {
	t.Helper()
	entries, total, err := store.SearchColdIndex(ctx, types.ColdSearchQuery{
		WorkspaceID: workspaceID,
		RoleID:      strings.TrimSpace(roleID),
		TextQuery:   "deployment",
		Limit:       10,
	})
	if err != nil {
		t.Fatalf("SearchColdIndex(%q, %q) error = %v", workspaceID, roleID, err)
	}
	if total != len(wantIDs) || len(entries) != len(wantIDs) {
		t.Fatalf("SearchColdIndex(%q, %q) returned %d/%d entries, want %d/%d: %v", workspaceID, roleID, len(entries), total, len(wantIDs), len(wantIDs), coldVisibilityEntryIDs(entries))
	}

	got := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		got[entry.ID] = struct{}{}
	}
	for _, wantID := range wantIDs {
		assertColdVisibilityHasID(t, got, wantID)
	}
	return got
}

func assertColdVisibilityHasID(t *testing.T, ids map[string]struct{}, wantID string) {
	t.Helper()
	if _, ok := ids[wantID]; !ok {
		t.Fatalf("result IDs missing %q", wantID)
	}
}

func assertColdVisibilityMissingID(t *testing.T, ids map[string]struct{}, missingID string) {
	t.Helper()
	if _, ok := ids[missingID]; ok {
		t.Fatalf("result IDs contained %q", missingID)
	}
}

func coldVisibilityEntryIDs(entries []types.ColdIndexEntry) []string {
	ids := make([]string, 0, len(entries))
	for _, entry := range entries {
		ids = append(ids, entry.ID)
	}
	return ids
}
