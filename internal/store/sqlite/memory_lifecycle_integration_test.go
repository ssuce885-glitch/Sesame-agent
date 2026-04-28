package sqlite

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go-agent/internal/memory"
	"go-agent/internal/types"
)

func TestMemoryLifecycle_ActiveToDeprecatedToColdToRecall(t *testing.T) {
	ctx := context.Background()
	store, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	now := time.Now().UTC()
	inputs := []struct {
		id      string
		age     time.Duration
		content string
	}{
		{id: "memory_1d", age: 1 * 24 * time.Hour, content: "Use go test ./... for running tests"},
		{id: "memory_15d", age: 15 * 24 * time.Hour, content: "Always format with gofmt before commit"},
		{id: "memory_30d", age: 30 * 24 * time.Hour, content: "Database migrations need manual review"},
		{id: "memory_60d", age: 60 * 24 * time.Hour, content: "Old legacy pattern avoid global state"},
		{id: "memory_90d", age: 90 * 24 * time.Hour, content: "Very old note about python scripts"},
	}

	activeEntries := map[string]types.MemoryEntry{}
	deprecatedEntries := map[string]types.MemoryEntry{}
	scores := map[string]float64{}
	for _, input := range inputs {
		timestamp := now.Add(-input.age)
		entry := types.MemoryEntry{
			ID:          input.id,
			Scope:       types.MemoryScopeWorkspace,
			WorkspaceID: "/ws",
			Visibility:  types.MemoryVisibilityShared,
			Status:      types.MemoryStatusActive,
			Content:     input.content,
			Confidence:  0.9,
			CreatedAt:   timestamp,
			UpdatedAt:   timestamp,
			LastUsedAt:  timestamp,
		}
		if err := store.InsertMemoryEntry(ctx, entry); err != nil {
			t.Fatalf("InsertMemoryEntry(%s) error = %v", entry.ID, err)
		}

		score := memory.EffectiveScore(entry, now)
		scores[entry.ID] = score
		t.Logf("%s effective score = %.4f", entry.ID, score)
		if score < memory.DeprecationThreshold {
			deprecatedEntries[entry.ID] = entry
		} else {
			activeEntries[entry.ID] = entry
		}
	}

	if score := scores["memory_1d"]; score < memory.DeprecationThreshold {
		t.Fatalf("1-day-old score = %.4f, want >= %.2f", score, memory.DeprecationThreshold)
	}
	if score := scores["memory_90d"]; score >= memory.DeprecationThreshold {
		t.Fatalf("90-day-old score = %.4f, want < %.2f", score, memory.DeprecationThreshold)
	}

	deprecatedIDs := make([]string, 0, len(deprecatedEntries))
	for _, entry := range deprecatedEntries {
		deprecatedIDs = append(deprecatedIDs, entry.ID)
		if err := store.InsertColdIndexEntry(ctx, types.ColdIndexEntry{
			ID:          "cold_memory_deprecated_" + entry.ID,
			WorkspaceID: "/ws",
			SourceType:  "memory_deprecated",
			SourceID:    entry.ID,
			SearchText:  entry.Content,
			SummaryLine: truncateColdSummaryLine(entry.Content),
			Visibility:  entry.Visibility,
			OccurredAt:  entry.CreatedAt,
			CreatedAt:   now,
			ContextRef: types.ColdContextRef{
				SessionID:     "session_1",
				ContextHeadID: "head_1",
				ItemCount:     0,
			},
		}); err != nil {
			t.Fatalf("InsertColdIndexEntry(%s) error = %v", entry.ID, err)
		}
	}
	if err := store.DeprecateMemoryEntries(ctx, deprecatedIDs); err != nil {
		t.Fatalf("DeprecateMemoryEntries() error = %v", err)
	}

	coldResults, coldTotal, err := store.SearchColdIndex(ctx, types.ColdSearchQuery{
		WorkspaceID: "/ws",
		SourceTypes: []string{"memory_deprecated"},
		Limit:       10,
	})
	if err != nil {
		t.Fatalf("SearchColdIndex(memory_deprecated) error = %v", err)
	}
	if coldTotal != len(deprecatedEntries) || len(coldResults) != len(deprecatedEntries) {
		t.Fatalf("cold search returned %d/%d entries, want %d/%d", len(coldResults), coldTotal, len(deprecatedEntries), len(deprecatedEntries))
	}

	coldBySourceID := make(map[string]types.ColdIndexEntry, len(coldResults))
	for _, result := range coldResults {
		coldBySourceID[result.SourceID] = result
	}
	for id, entry := range deprecatedEntries {
		result, ok := coldBySourceID[id]
		if !ok {
			t.Fatalf("deprecated entry %s missing from cold search results", id)
		}
		if !strings.Contains(result.SearchText, entry.Content) {
			t.Fatalf("cold search text for %s = %q, want original content %q", id, result.SearchText, entry.Content)
		}
	}
	for id := range activeEntries {
		if _, ok := coldBySourceID[id]; ok {
			t.Fatalf("active entry %s unexpectedly present in cold search results", id)
		}
	}

	affected, err := store.CleanupDeprecatedMemories(ctx, now.Add(-90*24*time.Hour))
	if err != nil {
		t.Fatalf("CleanupDeprecatedMemories() error = %v", err)
	}
	if affected != 1 {
		t.Fatalf("CleanupDeprecatedMemories() affected = %d, want 1", affected)
	}
	if _, found, err := store.GetMemoryEntry(ctx, "memory_90d"); err != nil {
		t.Fatalf("GetMemoryEntry(memory_90d) error = %v", err)
	} else if found {
		t.Fatalf("90-day-old deprecated memory still exists after cleanup")
	}
	if _, found, err := store.GetColdIndexEntry(ctx, "cold_memory_deprecated_memory_90d"); err != nil {
		t.Fatalf("GetColdIndexEntry(memory_90d cold row) error = %v", err)
	} else if !found {
		t.Fatalf("90-day-old cold index row missing after memory cleanup")
	}
	for _, id := range []string{"memory_60d", "memory_30d"} {
		entry, found, err := store.GetMemoryEntry(ctx, id)
		if err != nil {
			t.Fatalf("GetMemoryEntry(%s) error = %v", id, err)
		}
		if !found {
			t.Fatalf("%s missing after deprecated memory cleanup", id)
		}
		if _, wasDeprecated := deprecatedEntries[id]; wasDeprecated && entry.Status != types.MemoryStatusDeprecated {
			t.Fatalf("%s status = %q, want deprecated", id, entry.Status)
		}
	}
}

func truncateColdSummaryLine(content string) string {
	if len(content) <= 200 {
		return content
	}
	return content[:200]
}
