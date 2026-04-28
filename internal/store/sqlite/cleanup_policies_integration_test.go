package sqlite

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"go-agent/internal/types"
)

func TestCleanupPoliciesEndToEnd(t *testing.T) {
	ctx := context.Background()
	store, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	now := time.Now().UTC()
	workspaceRoot := "/ws"
	if err := store.InsertSession(ctx, types.Session{
		ID:            "sess_cl",
		WorkspaceRoot: workspaceRoot,
		State:         types.SessionStateIdle,
		CreatedAt:     now,
		UpdatedAt:     now,
	}); err != nil {
		t.Fatalf("InsertSession(sess_cl) error = %v", err)
	}

	insertCleanupPolicyMemory(t, ctx, store, "mem_new", types.MemoryStatusActive, now.Add(-1*24*time.Hour))
	insertCleanupPolicyMemory(t, ctx, store, "mem_old_dep", types.MemoryStatusDeprecated, now.Add(-100*24*time.Hour))
	insertCleanupPolicyMemory(t, ctx, store, "mem_recent_dep", types.MemoryStatusDeprecated, now.Add(-60*24*time.Hour))

	insertCleanupPolicyColdEntry(t, ctx, store, types.ColdIndexEntry{
		ID:          "cold_mem_mem_old_dep",
		WorkspaceID: workspaceRoot,
		SourceType:  "memory_deprecated",
		SourceID:    "mem_old_dep",
		SearchText:  "old deprecated memory content",
		SummaryLine: "old deprecated memory",
		Visibility:  types.MemoryVisibilityShared,
		OccurredAt:  now.Add(-100 * 24 * time.Hour),
		CreatedAt:   now,
	})
	insertCleanupPolicyColdEntry(t, ctx, store, types.ColdIndexEntry{
		ID:          "cold_mem_mem_recent_dep",
		WorkspaceID: workspaceRoot,
		SourceType:  "memory_deprecated",
		SourceID:    "mem_recent_dep",
		SearchText:  "recent deprecated memory content",
		SummaryLine: "recent deprecated memory",
		Visibility:  types.MemoryVisibilityShared,
		OccurredAt:  now.Add(-60 * 24 * time.Hour),
		CreatedAt:   now,
	})

	oldReportAt := now.Add(-40 * 24 * time.Hour)
	recentReportAt := now.Add(-5 * 24 * time.Hour)
	insertCleanupPolicyReport(t, ctx, store, "report_old", workspaceRoot, oldReportAt, "old report cleanup summary")
	insertCleanupPolicyReport(t, ctx, store, "report_recent", workspaceRoot, recentReportAt, "recent report cleanup summary")
	insertCleanupPolicyColdEntry(t, ctx, store, types.ColdIndexEntry{
		ID:          "cold_report_report_old",
		WorkspaceID: workspaceRoot,
		SourceType:  "report",
		SourceID:    "report_old",
		SearchText:  "old report cleanup summary",
		SummaryLine: "old report cleanup summary",
		Visibility:  types.MemoryVisibilityShared,
		OccurredAt:  oldReportAt,
		CreatedAt:   now,
	})
	insertCleanupPolicyColdEntry(t, ctx, store, types.ColdIndexEntry{
		ID:          "cold_report_report_recent",
		WorkspaceID: workspaceRoot,
		SourceType:  "report",
		SourceID:    "report_recent",
		SearchText:  "recent report cleanup summary",
		SummaryLine: "recent report cleanup summary",
		Visibility:  types.MemoryVisibilityShared,
		OccurredAt:  recentReportAt,
		CreatedAt:   now,
	})

	oldDigestAt := now.Add(-40 * 24 * time.Hour)
	recentDigestAt := now.Add(-5 * 24 * time.Hour)
	insertCleanupPolicyDigest(t, ctx, store, "digest_old", oldDigestAt, "old digest cleanup summary")
	insertCleanupPolicyDigest(t, ctx, store, "digest_recent", recentDigestAt, "recent digest cleanup summary")
	insertCleanupPolicyColdEntry(t, ctx, store, types.ColdIndexEntry{
		ID:          "cold_digest_digest_old",
		WorkspaceID: workspaceRoot,
		SourceType:  "digest",
		SourceID:    "digest_old",
		SearchText:  "old digest cleanup summary",
		SummaryLine: "old digest cleanup summary",
		Visibility:  types.MemoryVisibilityShared,
		OccurredAt:  oldDigestAt,
		CreatedAt:   now,
	})
	insertCleanupPolicyColdEntry(t, ctx, store, types.ColdIndexEntry{
		ID:          "cold_digest_digest_recent",
		WorkspaceID: workspaceRoot,
		SourceType:  "digest",
		SourceID:    "digest_recent",
		SearchText:  "recent digest cleanup summary",
		SummaryLine: "recent digest cleanup summary",
		Visibility:  types.MemoryVisibilityShared,
		OccurredAt:  recentDigestAt,
		CreatedAt:   now,
	})

	insertCleanupPolicyReportDelivery(t, ctx, store, "del_old", "report_recent", workspaceRoot, now.Add(-20*24*time.Hour))
	insertCleanupPolicyReportDelivery(t, ctx, store, "del_recent", "report_recent", workspaceRoot, now.Add(-2*24*time.Hour))
	insertCleanupPolicyChildResult(t, ctx, store, "child_old", now.Add(-70*24*time.Hour))
	insertCleanupPolicyChildResult(t, ctx, store, "child_recent", now.Add(-10*24*time.Hour))

	for position := 1; position <= 12; position++ {
		insertCleanupPolicyCompaction(t, ctx, store, "head_a", position, now.Add(time.Duration(position)*time.Minute))
	}
	for position := 1; position <= 3; position++ {
		insertCleanupPolicyCompaction(t, ctx, store, "head_b", position, now.Add(time.Duration(position)*time.Minute))
	}

	affected, err := store.CleanupDeprecatedMemories(ctx, now.Add(-90*24*time.Hour))
	assertCleanupAffected(t, "CleanupDeprecatedMemories", 1, affected, err)
	affected, err = store.CleanupOldReports(ctx, workspaceRoot, now.Add(-30*24*time.Hour))
	assertCleanupAffected(t, "CleanupOldReports", 1, affected, err)
	affected, err = store.CleanupOldDigestRecords(ctx, workspaceRoot, now.Add(-30*24*time.Hour))
	assertCleanupAffected(t, "CleanupOldDigestRecords", 1, affected, err)
	affected, err = store.CleanupOldReportDeliveries(ctx, workspaceRoot, now.Add(-14*24*time.Hour))
	assertCleanupAffected(t, "CleanupOldReportDeliveries", 1, affected, err)
	affected, err = store.CleanupOldChildAgentResults(ctx, now.Add(-60*24*time.Hour))
	assertCleanupAffected(t, "CleanupOldChildAgentResults", 1, affected, err)
	affected, err = store.CleanupOldConversationCompactions(ctx, 10)
	assertCleanupAffected(t, "CleanupOldConversationCompactions", 2, affected, err)

	assertCleanupColdEntryFound(t, ctx, store, "cold_mem_mem_old_dep")
	assertCleanupColdEntryFound(t, ctx, store, "cold_report_report_old")
	assertCleanupColdEntryFound(t, ctx, store, "cold_digest_digest_old")
	assertCleanupColdSearchIDs(t, ctx, store, types.ColdSearchQuery{
		WorkspaceID: "/ws",
		SourceTypes: []string{"memory_deprecated"},
		TextQuery:   "old deprecated memory content",
		Until:       now.Add(-90 * 24 * time.Hour),
		Limit:       10,
	}, "cold_mem_mem_old_dep")
	assertCleanupColdSearchIDs(t, ctx, store, types.ColdSearchQuery{
		WorkspaceID: "/ws",
		SourceTypes: []string{"memory_deprecated"},
		TextQuery:   "recent deprecated memory content",
		Since:       now.Add(-90 * 24 * time.Hour),
		Limit:       10,
	}, "cold_mem_mem_recent_dep")

	assertCleanupMemoryFound(t, ctx, store, "mem_recent_dep", types.MemoryStatusDeprecated)
	assertCleanupMemoryFound(t, ctx, store, "mem_new", types.MemoryStatusActive)
	assertCleanupReportIDs(t, ctx, store, []string{"report_recent"}, []string{"report_old"})
	assertCleanupDigestFound(t, ctx, store, "digest_recent")
	assertCleanupDeliveryIDs(t, ctx, store, []string{"del_recent"}, []string{"del_old"})
	assertCleanupChildFound(t, ctx, store, "child_recent")
	assertCleanupCompactions(t, ctx, store, "head_a", 10, []int{1, 2}, []int{3, 12})
	assertCleanupCompactions(t, ctx, store, "head_b", 3, nil, []int{1, 3})
}

func insertCleanupPolicyMemory(t *testing.T, ctx context.Context, store *Store, id string, status types.MemoryStatus, timestamp time.Time) {
	t.Helper()
	if err := store.InsertMemoryEntry(ctx, types.MemoryEntry{
		ID:          id,
		Scope:       types.MemoryScopeWorkspace,
		WorkspaceID: "/ws",
		Visibility:  types.MemoryVisibilityShared,
		Status:      status,
		Content:     id + " cleanup content",
		Confidence:  0.9,
		LastUsedAt:  timestamp,
		CreatedAt:   timestamp,
		UpdatedAt:   timestamp,
	}); err != nil {
		t.Fatalf("InsertMemoryEntry(%s) error = %v", id, err)
	}
}

func insertCleanupPolicyColdEntry(t *testing.T, ctx context.Context, store *Store, entry types.ColdIndexEntry) {
	t.Helper()
	if err := store.InsertColdIndexEntry(ctx, entry); err != nil {
		t.Fatalf("InsertColdIndexEntry(%s) error = %v", entry.ID, err)
	}
}

func insertCleanupPolicyReport(t *testing.T, ctx context.Context, store *Store, id, workspaceRoot string, observedAt time.Time, summary string) {
	t.Helper()
	if err := store.UpsertReport(ctx, types.ReportRecord{
		ID:            id,
		WorkspaceRoot: workspaceRoot,
		SessionID:     "sess_cl",
		SourceKind:    types.ReportSourceTaskResult,
		SourceID:      id,
		Envelope: types.ReportEnvelope{
			Status:   "completed",
			Severity: "info",
			Summary:  summary,
		},
		ObservedAt: observedAt,
		CreatedAt:  observedAt,
		UpdatedAt:  observedAt,
	}); err != nil {
		t.Fatalf("UpsertReport(%s) error = %v", id, err)
	}
}

func insertCleanupPolicyDigest(t *testing.T, ctx context.Context, store *Store, id string, windowEnd time.Time, summary string) {
	t.Helper()
	if err := store.UpsertDigestRecord(ctx, types.DigestRecord{
		DigestID:    id,
		SessionID:   "sess_cl",
		GroupID:     "group_cleanup",
		WindowStart: windowEnd.Add(-time.Hour),
		WindowEnd:   windowEnd,
		Envelope: types.ReportEnvelope{
			Status:   "completed",
			Severity: "info",
			Summary:  summary,
		},
		CreatedAt: windowEnd,
		UpdatedAt: windowEnd,
	}); err != nil {
		t.Fatalf("UpsertDigestRecord(%s) error = %v", id, err)
	}
}

func insertCleanupPolicyReportDelivery(t *testing.T, ctx context.Context, store *Store, id, reportID, workspaceRoot string, observedAt time.Time) {
	t.Helper()
	if err := store.UpsertReportDelivery(ctx, types.ReportDelivery{
		ID:            id,
		WorkspaceRoot: workspaceRoot,
		SessionID:     "sess_cl",
		ReportID:      reportID,
		Channel:       types.ReportChannelAgent,
		State:         types.ReportDeliveryStateQueued,
		ObservedAt:    observedAt,
		CreatedAt:     observedAt,
		UpdatedAt:     observedAt,
	}); err != nil {
		t.Fatalf("UpsertReportDelivery(%s) error = %v", id, err)
	}
}

func insertCleanupPolicyChildResult(t *testing.T, ctx context.Context, store *Store, id string, observedAt time.Time) {
	t.Helper()
	if err := store.UpsertChildAgentResult(ctx, types.ChildAgentResult{
		ResultID:   id,
		SessionID:  "sess_cl",
		AgentID:    "agent_cleanup",
		ObservedAt: observedAt,
		Envelope: types.ReportEnvelope{
			Status:   "completed",
			Severity: "info",
			Summary:  id,
		},
		CreatedAt: observedAt,
		UpdatedAt: observedAt,
	}); err != nil {
		t.Fatalf("UpsertChildAgentResult(%s) error = %v", id, err)
	}
}

func insertCleanupPolicyCompaction(t *testing.T, ctx context.Context, store *Store, headID string, endPosition int, createdAt time.Time) {
	t.Helper()
	if err := store.InsertConversationCompactionWithContextHead(ctx, types.ConversationCompaction{
		ID:              fmt.Sprintf("compact_%s_%02d", headID, endPosition),
		SessionID:       "sess_cl",
		ContextHeadID:   headID,
		Kind:            types.ConversationCompactionKindArchive,
		Generation:      endPosition,
		StartItemID:     int64(endPosition),
		EndItemID:       int64(endPosition),
		StartPosition:   endPosition - 1,
		EndPosition:     endPosition,
		SummaryPayload:  "{}",
		MetadataJSON:    "{}",
		Reason:          "cleanup test",
		ProviderProfile: "test",
		CreatedAt:       createdAt,
	}); err != nil {
		t.Fatalf("InsertConversationCompactionWithContextHead(%s, %d) error = %v", headID, endPosition, err)
	}
}

func assertCleanupAffected(t *testing.T, name string, want int64, got int64, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("%s() error = %v", name, err)
	}
	if got != want {
		t.Fatalf("%s() affected = %d, want %d", name, got, want)
	}
}

func assertCleanupColdEntryFound(t *testing.T, ctx context.Context, store *Store, id string) {
	t.Helper()
	if _, found, err := store.GetColdIndexEntry(ctx, id); err != nil {
		t.Fatalf("GetColdIndexEntry(%s) error = %v", id, err)
	} else if !found {
		t.Fatalf("GetColdIndexEntry(%s) found = false, want true", id)
	}
}

func assertCleanupColdSearchIDs(t *testing.T, ctx context.Context, store *Store, query types.ColdSearchQuery, wantID string) {
	t.Helper()
	entries, total, err := store.SearchColdIndex(ctx, query)
	if err != nil {
		t.Fatalf("SearchColdIndex(%#v) error = %v", query, err)
	}
	if total != 1 || len(entries) != 1 {
		t.Fatalf("SearchColdIndex(%#v) returned %d/%d entries, want 1/1", query, len(entries), total)
	}
	if entries[0].ID != wantID {
		t.Fatalf("SearchColdIndex(%#v) ID = %q, want %q", query, entries[0].ID, wantID)
	}
}

func assertCleanupMemoryFound(t *testing.T, ctx context.Context, store *Store, id string, status types.MemoryStatus) {
	t.Helper()
	entry, found, err := store.GetMemoryEntry(ctx, id)
	if err != nil {
		t.Fatalf("GetMemoryEntry(%s) error = %v", id, err)
	}
	if !found {
		t.Fatalf("GetMemoryEntry(%s) found = false, want true", id)
	}
	if entry.Status != status {
		t.Fatalf("GetMemoryEntry(%s) status = %q, want %q", id, entry.Status, status)
	}
}

func assertCleanupReportIDs(t *testing.T, ctx context.Context, store *Store, wantPresent []string, wantMissing []string) {
	t.Helper()
	reports, err := store.ListReports(ctx, "sess_cl")
	if err != nil {
		t.Fatalf("ListReports(sess_cl) error = %v", err)
	}
	ids := make(map[string]struct{}, len(reports))
	for _, report := range reports {
		ids[report.ID] = struct{}{}
	}
	assertCleanupIDs(t, "reports", ids, wantPresent, wantMissing)
}

func assertCleanupDigestFound(t *testing.T, ctx context.Context, store *Store, id string) {
	t.Helper()
	if _, found, err := store.GetDigestRecord(ctx, id); err != nil {
		t.Fatalf("GetDigestRecord(%s) error = %v", id, err)
	} else if !found {
		t.Fatalf("GetDigestRecord(%s) found = false, want true", id)
	}
}

func assertCleanupDeliveryIDs(t *testing.T, ctx context.Context, store *Store, wantPresent []string, wantMissing []string) {
	t.Helper()
	deliveries, err := store.ListReportDeliveries(ctx, "sess_cl", types.ReportChannelAgent)
	if err != nil {
		t.Fatalf("ListReportDeliveries(sess_cl) error = %v", err)
	}
	ids := make(map[string]struct{}, len(deliveries))
	for _, delivery := range deliveries {
		ids[delivery.ID] = struct{}{}
	}
	assertCleanupIDs(t, "report_deliveries", ids, wantPresent, wantMissing)
}

func assertCleanupChildFound(t *testing.T, ctx context.Context, store *Store, id string) {
	t.Helper()
	if _, found, err := store.GetChildAgentResult(ctx, id); err != nil {
		t.Fatalf("GetChildAgentResult(%s) error = %v", id, err)
	} else if !found {
		t.Fatalf("GetChildAgentResult(%s) found = false, want true", id)
	}
}

func assertCleanupCompactions(t *testing.T, ctx context.Context, store *Store, headID string, wantCount int, wantMissing []int, wantPresent []int) {
	t.Helper()
	compactions, err := store.ListConversationCompactionsByStoredContextHead(ctx, "sess_cl", headID)
	if err != nil {
		t.Fatalf("ListConversationCompactionsByStoredContextHead(%s) error = %v", headID, err)
	}
	if len(compactions) != wantCount {
		t.Fatalf("compactions for %s = %d, want %d", headID, len(compactions), wantCount)
	}
	positions := make(map[int]struct{}, len(compactions))
	for _, compaction := range compactions {
		positions[compaction.EndPosition] = struct{}{}
	}
	for _, position := range wantMissing {
		if _, ok := positions[position]; ok {
			t.Fatalf("compaction %s end position %d present, want missing", headID, position)
		}
	}
	for _, position := range wantPresent {
		if _, ok := positions[position]; !ok {
			t.Fatalf("compaction %s end position %d missing, want present", headID, position)
		}
	}
}

func assertCleanupIDs(t *testing.T, label string, ids map[string]struct{}, wantPresent []string, wantMissing []string) {
	t.Helper()
	for _, id := range wantPresent {
		if _, ok := ids[id]; !ok {
			t.Fatalf("%s ID %q missing, want present", label, id)
		}
	}
	for _, id := range wantMissing {
		if _, ok := ids[id]; ok {
			t.Fatalf("%s ID %q present, want missing", label, id)
		}
	}
}
