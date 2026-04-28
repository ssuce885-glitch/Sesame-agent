package sqlite

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"go-agent/internal/types"
)

func TestColdIndexIdempotentInsertReplace(t *testing.T) {
	ctx := context.Background()
	store, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	now := time.Now().UTC()
	entry := types.ColdIndexEntry{
		ID:          "cold_archive_abc123",
		WorkspaceID: "/ws",
		SourceType:  "archive",
		SourceID:    "abc123",
		SearchText:  "original design discussion about rate limiting",
		SummaryLine: "original summary",
		Visibility:  types.MemoryVisibilityShared,
		OccurredAt:  now,
		CreatedAt:   now,
	}
	if err := store.InsertColdIndexEntry(ctx, entry); err != nil {
		t.Fatalf("InsertColdIndexEntry(original) error = %v", err)
	}

	entries, total, err := store.SearchColdIndex(ctx, types.ColdSearchQuery{
		WorkspaceID: "/ws",
		TextQuery:   "original design",
		Limit:       10,
	})
	if err != nil {
		t.Fatalf("SearchColdIndex(original design) error = %v", err)
	}
	if total != 1 || len(entries) != 1 {
		t.Fatalf("SearchColdIndex(original design) returned %d/%d entries, want 1/1", len(entries), total)
	}
	if entries[0].SummaryLine != "original summary" {
		t.Fatalf("SearchColdIndex(original design) SummaryLine = %q, want original summary", entries[0].SummaryLine)
	}

	entry.SearchText = "updated design decision about circuit breaker"
	entry.SummaryLine = "updated summary"
	if err := store.InsertColdIndexEntry(ctx, entry); err != nil {
		t.Fatalf("InsertColdIndexEntry(updated) error = %v", err)
	}

	entries, total, err = store.SearchColdIndex(ctx, types.ColdSearchQuery{
		WorkspaceID: "/ws",
		TextQuery:   "original",
		Limit:       10,
	})
	if err != nil {
		t.Fatalf("SearchColdIndex(original) error = %v", err)
	}
	if total != 0 || len(entries) != 0 {
		t.Fatalf("SearchColdIndex(original) returned %d/%d entries, want 0/0", len(entries), total)
	}

	entries, total, err = store.SearchColdIndex(ctx, types.ColdSearchQuery{
		WorkspaceID: "/ws",
		TextQuery:   "circuit breaker",
		Limit:       10,
	})
	if err != nil {
		t.Fatalf("SearchColdIndex(circuit breaker) error = %v", err)
	}
	if total != 1 || len(entries) != 1 {
		t.Fatalf("SearchColdIndex(circuit breaker) returned %d/%d entries, want 1/1", len(entries), total)
	}
	if entries[0].ID != "cold_archive_abc123" || entries[0].SummaryLine != "updated summary" {
		t.Fatalf("SearchColdIndex(circuit breaker) entry = %#v, want updated abc123 entry", entries[0])
	}

	stored, found, err := store.GetColdIndexEntry(ctx, "cold_archive_abc123")
	if err != nil {
		t.Fatalf("GetColdIndexEntry(cold_archive_abc123) error = %v", err)
	}
	if !found {
		t.Fatalf("GetColdIndexEntry(cold_archive_abc123) found = false, want true")
	}
	if stored.SummaryLine != "updated summary" {
		t.Fatalf("GetColdIndexEntry(cold_archive_abc123) SummaryLine = %q, want updated summary", stored.SummaryLine)
	}

	var count int
	if err := store.DB().QueryRowContext(ctx, `select count(*) from cold_index where id = ?`, "cold_archive_abc123").Scan(&count); err != nil {
		t.Fatalf("count cold_index abc123 error = %v", err)
	}
	if count != 1 {
		t.Fatalf("cold_index rows for abc123 = %d, want 1", count)
	}

	second := types.ColdIndexEntry{
		ID:          "cold_archive_def456",
		WorkspaceID: "/ws",
		SourceType:  "archive",
		SourceID:    "def456",
		SearchText:  "completely different topic about database indexes",
		SummaryLine: "database indexes summary",
		Visibility:  types.MemoryVisibilityShared,
		OccurredAt:  now.Add(time.Minute),
		CreatedAt:   now.Add(time.Minute),
	}
	if err := store.InsertColdIndexEntry(ctx, second); err != nil {
		t.Fatalf("InsertColdIndexEntry(def456) error = %v", err)
	}

	entries, total, err = store.SearchColdIndex(ctx, types.ColdSearchQuery{
		WorkspaceID: "/ws",
		TextQuery:   "database indexes",
		Limit:       10,
	})
	if err != nil {
		t.Fatalf("SearchColdIndex(database indexes) error = %v", err)
	}
	if total != 1 || len(entries) != 1 {
		t.Fatalf("SearchColdIndex(database indexes) returned %d/%d entries, want 1/1", len(entries), total)
	}
	if entries[0].ID != "cold_archive_def456" {
		t.Fatalf("SearchColdIndex(database indexes) ID = %q, want cold_archive_def456", entries[0].ID)
	}

	entries, total, err = store.SearchColdIndex(ctx, types.ColdSearchQuery{
		WorkspaceID: "/ws",
		TextQuery:   "circuit breaker",
		Limit:       10,
	})
	if err != nil {
		t.Fatalf("SearchColdIndex(circuit breaker after def456) error = %v", err)
	}
	if total != 1 || len(entries) != 1 {
		t.Fatalf("SearchColdIndex(circuit breaker after def456) returned %d/%d entries, want 1/1", len(entries), total)
	}
	if entries[0].ID != "cold_archive_abc123" {
		t.Fatalf("SearchColdIndex(circuit breaker after def456) ID = %q, want cold_archive_abc123", entries[0].ID)
	}

	if err := store.DB().QueryRowContext(ctx, `select count(*) from cold_index`).Scan(&count); err != nil {
		t.Fatalf("count cold_index total error = %v", err)
	}
	if count != 2 {
		t.Fatalf("cold_index rows total = %d, want 2", count)
	}
}
