package sqlite

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"go-agent/internal/types"
)

func TestColdSearchScoringOrder(t *testing.T) {
	ctx := context.Background()
	store, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	now := time.Now().UTC()
	entries := []types.ColdIndexEntry{
		{
			ID:          "cold_ar",
			WorkspaceID: "/ws",
			Visibility:  types.MemoryVisibilityShared,
			SourceType:  "archive",
			SourceID:    "ar",
			SearchText:  "deployment pipeline kubernetes configuration production",
			SummaryLine: "archive entry",
			OccurredAt:  now.Add(-7 * 24 * time.Hour),
			CreatedAt:   now.Add(-7 * 24 * time.Hour),
		},
		{
			ID:          "cold_md",
			WorkspaceID: "/ws",
			Visibility:  types.MemoryVisibilityShared,
			SourceType:  "memory_deprecated",
			SourceID:    "md",
			SearchText:  "deployment pipeline staging environment",
			SummaryLine: "memory deprecated entry",
			OccurredAt:  now.Add(-2 * 24 * time.Hour),
			CreatedAt:   now.Add(-2 * 24 * time.Hour),
		},
		{
			ID:          "cold_rp",
			WorkspaceID: "/ws",
			Visibility:  types.MemoryVisibilityShared,
			SourceType:  "report",
			SourceID:    "rp",
			SearchText:  "deployment daily summary",
			SummaryLine: "report entry",
			OccurredAt:  now.Add(-24 * time.Hour),
			CreatedAt:   now.Add(-24 * time.Hour),
		},
		{
			ID:          "cold_dg",
			WorkspaceID: "/ws",
			Visibility:  types.MemoryVisibilityShared,
			SourceType:  "digest",
			SourceID:    "dg",
			SearchText:  "unrelated topic about food recipes",
			SummaryLine: "digest entry",
			OccurredAt:  now,
			CreatedAt:   now,
		},
	}
	for _, entry := range entries {
		if err := store.InsertColdIndexEntry(ctx, entry); err != nil {
			t.Fatalf("InsertColdIndexEntry(%s) error = %v", entry.ID, err)
		}
	}

	searchResults, total, err := store.SearchColdIndex(ctx, types.ColdSearchQuery{
		WorkspaceID: "/ws",
		TextQuery:   "deployment pipeline kubernetes",
		Limit:       10,
	})
	if err != nil {
		t.Fatalf("SearchColdIndex(deployment pipeline kubernetes) error = %v", err)
	}
	if total != len(searchResults) {
		t.Fatalf("SearchColdIndex(deployment pipeline kubernetes) returned %d entries with total %d, want equal under limit", len(searchResults), total)
	}
	assertScoringColdIDsPresent(t, searchResults, "cold_ar", "cold_md", "cold_rp")
	assertScoringColdIDAbsent(t, searchResults, "cold_dg")

	deploymentResults, deploymentTotal, err := store.SearchColdIndex(ctx, types.ColdSearchQuery{
		WorkspaceID: "/ws",
		TextQuery:   "deployment",
		Limit:       10,
	})
	if err != nil {
		t.Fatalf("SearchColdIndex(deployment) error = %v", err)
	}
	if deploymentTotal != 3 || len(deploymentResults) != 3 {
		t.Fatalf("SearchColdIndex(deployment) returned %d/%d entries, want 3/3", len(deploymentResults), deploymentTotal)
	}
	assertScoringColdIDAbsent(t, deploymentResults, "cold_dg")
	if deploymentResults[0].ID == "" {
		t.Fatalf("SearchColdIndex(deployment) first result has empty ID")
	}

	browseResults, browseTotal, err := store.SearchColdIndex(ctx, types.ColdSearchQuery{
		WorkspaceID: "/ws",
		Limit:       10,
	})
	if err != nil {
		t.Fatalf("SearchColdIndex(browse all) error = %v", err)
	}
	if browseTotal != 4 || len(browseResults) != 4 {
		t.Fatalf("SearchColdIndex(browse all) returned %d/%d entries, want 4/4", len(browseResults), browseTotal)
	}
	assertScoringColdIDsPresent(t, browseResults, "cold_ar", "cold_md", "cold_rp", "cold_dg")
	for _, result := range browseResults {
		if result.OccurredAt.IsZero() || result.CreatedAt.IsZero() {
			t.Fatalf("SearchColdIndex(browse all) entry %s has zero timestamps: %#v", result.ID, result)
		}
	}

	wantSummaryByID := map[string]string{
		"cold_ar": "archive entry",
		"cold_md": "memory deprecated entry",
		"cold_rp": "report entry",
		"cold_dg": "digest entry",
	}
	for _, entry := range entries {
		stored, found, err := store.GetColdIndexEntry(ctx, entry.ID)
		if err != nil {
			t.Fatalf("GetColdIndexEntry(%s) error = %v", entry.ID, err)
		}
		if !found {
			t.Fatalf("GetColdIndexEntry(%s) found = false, want true", entry.ID)
		}
		if stored.SummaryLine != wantSummaryByID[entry.ID] {
			t.Fatalf("GetColdIndexEntry(%s) SummaryLine = %q, want %q", entry.ID, stored.SummaryLine, wantSummaryByID[entry.ID])
		}
		if stored.SourceType != entry.SourceType || stored.WorkspaceID != "/ws" {
			t.Fatalf("GetColdIndexEntry(%s) = %#v, want source type %q in /ws", entry.ID, stored, entry.SourceType)
		}
	}
}

func assertScoringColdIDsPresent(t *testing.T, entries []types.ColdIndexEntry, wantIDs ...string) {
	t.Helper()
	got := scoringColdIDSet(entries)
	for _, wantID := range wantIDs {
		if _, ok := got[wantID]; !ok {
			t.Fatalf("cold search IDs = %v, missing %q", scoringColdIDs(entries), wantID)
		}
	}
}

func assertScoringColdIDAbsent(t *testing.T, entries []types.ColdIndexEntry, wantAbsent string) {
	t.Helper()
	got := scoringColdIDSet(entries)
	if _, ok := got[wantAbsent]; ok {
		t.Fatalf("cold search IDs = %v, want %q absent", scoringColdIDs(entries), wantAbsent)
	}
}

func scoringColdIDSet(entries []types.ColdIndexEntry) map[string]struct{} {
	ids := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		ids[entry.ID] = struct{}{}
	}
	return ids
}

func scoringColdIDs(entries []types.ColdIndexEntry) []string {
	ids := make([]string, 0, len(entries))
	for _, entry := range entries {
		ids = append(ids, entry.ID)
	}
	return ids
}
