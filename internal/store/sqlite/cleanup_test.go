package sqlite

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"go-agent/internal/types"
)

func TestCleanupRetentionPolicies(t *testing.T) {
	ctx := context.Background()
	store := openCleanupTestStore(t)
	now := time.Date(2026, 4, 28, 12, 0, 0, 0, time.UTC)
	old := now.Add(-100 * 24 * time.Hour)
	recent := now.Add(-10 * 24 * time.Hour)
	workspaceRoot := "/workspace"
	otherWorkspaceRoot := "/other"

	insertCleanupSession(t, ctx, store, "session_workspace", workspaceRoot, now)
	insertCleanupSession(t, ctx, store, "session_other", otherWorkspaceRoot, now)

	insertCleanupMemory(t, ctx, store, "memory_old_deprecated", types.MemoryStatusDeprecated, old)
	insertCleanupMemory(t, ctx, store, "memory_recent_deprecated", types.MemoryStatusDeprecated, recent)
	insertCleanupMemory(t, ctx, store, "memory_old_active", types.MemoryStatusActive, old)
	affected, err := store.CleanupDeprecatedMemories(ctx, now.Add(-90*24*time.Hour))
	if err != nil {
		t.Fatalf("CleanupDeprecatedMemories() error = %v", err)
	}
	if affected != 1 {
		t.Fatalf("deprecated memories affected = %d, want 1", affected)
	}
	assertMissingRow(t, ctx, store, "memory_entries", "id", "memory_old_deprecated")
	assertPresentRow(t, ctx, store, "memory_entries", "id", "memory_recent_deprecated")
	assertPresentRow(t, ctx, store, "memory_entries", "id", "memory_old_active")

	insertCleanupReport(t, ctx, store, "report_old", "session_workspace", workspaceRoot, old)
	insertCleanupReport(t, ctx, store, "report_recent", "session_workspace", workspaceRoot, recent)
	insertCleanupReport(t, ctx, store, "report_other", "session_other", otherWorkspaceRoot, old)
	affected, err = store.CleanupOldReports(ctx, workspaceRoot, now.Add(-30*24*time.Hour))
	if err != nil {
		t.Fatalf("CleanupOldReports() error = %v", err)
	}
	if affected != 1 {
		t.Fatalf("reports affected = %d, want 1", affected)
	}
	assertMissingRow(t, ctx, store, "reports", "id", "report_old")
	assertPresentRow(t, ctx, store, "reports", "id", "report_recent")
	assertPresentRow(t, ctx, store, "reports", "id", "report_other")

	insertCleanupReportDelivery(t, ctx, store, "delivery_old", "report_recent", "session_workspace", workspaceRoot, old)
	insertCleanupReportDelivery(t, ctx, store, "delivery_recent", "report_recent", "session_workspace", workspaceRoot, recent)
	insertCleanupReportDelivery(t, ctx, store, "delivery_other", "report_other", "session_other", otherWorkspaceRoot, old)
	affected, err = store.CleanupOldReportDeliveries(ctx, workspaceRoot, now.Add(-14*24*time.Hour))
	if err != nil {
		t.Fatalf("CleanupOldReportDeliveries() error = %v", err)
	}
	if affected != 1 {
		t.Fatalf("report deliveries affected = %d, want 1", affected)
	}
	assertMissingRow(t, ctx, store, "report_deliveries", "id", "delivery_old")
	assertPresentRow(t, ctx, store, "report_deliveries", "id", "delivery_recent")
	assertPresentRow(t, ctx, store, "report_deliveries", "id", "delivery_other")

	insertCleanupDigest(t, ctx, store, "digest_old", "session_workspace", old)
	insertCleanupDigest(t, ctx, store, "digest_recent", "session_workspace", recent)
	insertCleanupDigest(t, ctx, store, "digest_other", "session_other", old)
	affected, err = store.CleanupOldDigestRecords(ctx, workspaceRoot, now.Add(-30*24*time.Hour))
	if err != nil {
		t.Fatalf("CleanupOldDigestRecords() error = %v", err)
	}
	if affected != 1 {
		t.Fatalf("digest records affected = %d, want 1", affected)
	}
	assertMissingRow(t, ctx, store, "digest_records", "id", "digest_old")
	assertPresentRow(t, ctx, store, "digest_records", "id", "digest_recent")
	assertPresentRow(t, ctx, store, "digest_records", "id", "digest_other")

	insertCleanupChildResult(t, ctx, store, "child_old", old)
	insertCleanupChildResult(t, ctx, store, "child_recent", recent)
	affected, err = store.CleanupOldChildAgentResults(ctx, now.Add(-60*24*time.Hour))
	if err != nil {
		t.Fatalf("CleanupOldChildAgentResults() error = %v", err)
	}
	if affected != 1 {
		t.Fatalf("child agent results affected = %d, want 1", affected)
	}
	assertMissingRow(t, ctx, store, "child_agent_results", "id", "child_old")
	assertPresentRow(t, ctx, store, "child_agent_results", "id", "child_recent")
}

func TestCleanupConversationCompactions(t *testing.T) {
	ctx := context.Background()
	store := openCleanupTestStore(t)
	now := time.Date(2026, 4, 28, 12, 0, 0, 0, time.UTC)

	for position := 1; position <= 12; position++ {
		insertCleanupCompaction(t, ctx, store, "session_compactions", "head_a", position, now.Add(time.Duration(position)*time.Minute))
	}
	for position := 1; position <= 3; position++ {
		insertCleanupCompaction(t, ctx, store, "session_compactions", "head_b", position, now.Add(time.Duration(position)*time.Minute))
	}
	affected, err := store.CleanupOldConversationCompactions(ctx, 10)
	if err != nil {
		t.Fatalf("CleanupOldConversationCompactions() error = %v", err)
	}
	if affected != 2 {
		t.Fatalf("conversation compactions affected = %d, want 2", affected)
	}
	assertCount(t, ctx, store, "conversation_compactions", "session_id = ? and context_head_id = ?", 10, "session_compactions", "head_a")
	assertCount(t, ctx, store, "conversation_compactions", "session_id = ? and context_head_id = ?", 3, "session_compactions", "head_b")
	assertMissingCompactionEndPosition(t, ctx, store, "session_compactions", "head_a", 1)
	assertMissingCompactionEndPosition(t, ctx, store, "session_compactions", "head_a", 2)
	assertPresentCompactionEndPosition(t, ctx, store, "session_compactions", "head_a", 3)
	assertPresentCompactionEndPosition(t, ctx, store, "session_compactions", "head_a", 12)
}

func openCleanupTestStore(t *testing.T) *Store {
	t.Helper()
	store, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})
	return store
}

func insertCleanupSession(t *testing.T, ctx context.Context, store *Store, id, workspaceRoot string, now time.Time) {
	t.Helper()
	if err := store.InsertSession(ctx, types.Session{
		ID:            id,
		WorkspaceRoot: workspaceRoot,
		State:         types.SessionStateIdle,
		CreatedAt:     now,
		UpdatedAt:     now,
	}); err != nil {
		t.Fatalf("InsertSession(%s) error = %v", id, err)
	}
}

func insertCleanupMemory(t *testing.T, ctx context.Context, store *Store, id string, status types.MemoryStatus, updatedAt time.Time) {
	t.Helper()
	if err := store.UpsertMemoryEntry(ctx, types.MemoryEntry{
		ID:          id,
		Scope:       types.MemoryScopeWorkspace,
		WorkspaceID: "/workspace",
		Status:      status,
		Content:     id,
		Confidence:  0.9,
		CreatedAt:   updatedAt,
		UpdatedAt:   updatedAt,
		LastUsedAt:  updatedAt,
	}); err != nil {
		t.Fatalf("UpsertMemoryEntry(%s) error = %v", id, err)
	}
}

func insertCleanupReport(t *testing.T, ctx context.Context, store *Store, id, sessionID, workspaceRoot string, observedAt time.Time) {
	t.Helper()
	if err := store.UpsertReport(ctx, types.ReportRecord{
		ID:            id,
		WorkspaceRoot: workspaceRoot,
		SessionID:     sessionID,
		SourceKind:    types.ReportSourceTaskResult,
		SourceID:      id,
		Envelope: types.ReportEnvelope{
			Status:   "completed",
			Severity: "info",
			Summary:  id,
		},
		ObservedAt: observedAt,
		CreatedAt:  observedAt,
		UpdatedAt:  observedAt,
	}); err != nil {
		t.Fatalf("UpsertReport(%s) error = %v", id, err)
	}
}

func insertCleanupReportDelivery(t *testing.T, ctx context.Context, store *Store, id, reportID, sessionID, workspaceRoot string, observedAt time.Time) {
	t.Helper()
	if err := store.UpsertReportDelivery(ctx, types.ReportDelivery{
		ID:            id,
		WorkspaceRoot: workspaceRoot,
		SessionID:     sessionID,
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

func insertCleanupDigest(t *testing.T, ctx context.Context, store *Store, id, sessionID string, windowEnd time.Time) {
	t.Helper()
	if err := store.UpsertDigestRecord(ctx, types.DigestRecord{
		DigestID:    id,
		SessionID:   sessionID,
		GroupID:     "group_1",
		WindowStart: windowEnd.Add(-time.Hour),
		WindowEnd:   windowEnd,
		Envelope: types.ReportEnvelope{
			Status:   "completed",
			Severity: "info",
			Summary:  id,
		},
		CreatedAt: windowEnd,
		UpdatedAt: windowEnd,
	}); err != nil {
		t.Fatalf("UpsertDigestRecord(%s) error = %v", id, err)
	}
}

func insertCleanupChildResult(t *testing.T, ctx context.Context, store *Store, id string, observedAt time.Time) {
	t.Helper()
	if err := store.UpsertChildAgentResult(ctx, types.ChildAgentResult{
		ResultID:   id,
		SessionID:  "session_workspace",
		AgentID:    "agent_1",
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

func insertCleanupCompaction(t *testing.T, ctx context.Context, store *Store, sessionID, headID string, endPosition int, createdAt time.Time) {
	t.Helper()
	if err := store.InsertConversationCompactionWithContextHead(ctx, types.ConversationCompaction{
		ID:              fmt.Sprintf("compact_%s_%d", headID, endPosition),
		SessionID:       sessionID,
		ContextHeadID:   headID,
		Kind:            types.ConversationCompactionKindArchive,
		Generation:      endPosition,
		StartItemID:     int64(endPosition),
		EndItemID:       int64(endPosition),
		StartPosition:   endPosition - 1,
		EndPosition:     endPosition,
		SummaryPayload:  "{}",
		MetadataJSON:    "{}",
		Reason:          "test",
		ProviderProfile: "test",
		CreatedAt:       createdAt,
	}); err != nil {
		t.Fatalf("InsertConversationCompactionWithContextHead(%s, %s, %d) error = %v", sessionID, headID, endPosition, err)
	}
}

func assertPresentRow(t *testing.T, ctx context.Context, store *Store, table, column, value string) {
	t.Helper()
	if !rowExists(t, ctx, store, table, column, value) {
		t.Fatalf("%s row with %s=%q missing", table, column, value)
	}
}

func assertMissingRow(t *testing.T, ctx context.Context, store *Store, table, column, value string) {
	t.Helper()
	if rowExists(t, ctx, store, table, column, value) {
		t.Fatalf("%s row with %s=%q still present", table, column, value)
	}
}

func rowExists(t *testing.T, ctx context.Context, store *Store, table, column, value string) bool {
	t.Helper()
	var count int
	if err := store.DB().QueryRowContext(ctx, fmt.Sprintf("select count(*) from %s where %s = ?", table, column), value).Scan(&count); err != nil {
		t.Fatalf("count %s.%s error = %v", table, column, err)
	}
	return count > 0
}

func assertCount(t *testing.T, ctx context.Context, store *Store, table, where string, want int, args ...any) {
	t.Helper()
	var count int
	query := fmt.Sprintf("select count(*) from %s where %s", table, where)
	if err := store.DB().QueryRowContext(ctx, query, args...).Scan(&count); err != nil {
		t.Fatalf("count %s error = %v", table, err)
	}
	if count != want {
		t.Fatalf("%s count = %d, want %d", table, count, want)
	}
}

func assertPresentCompactionEndPosition(t *testing.T, ctx context.Context, store *Store, sessionID, headID string, endPosition int) {
	t.Helper()
	assertCompactionEndPosition(t, ctx, store, sessionID, headID, endPosition, true)
}

func assertMissingCompactionEndPosition(t *testing.T, ctx context.Context, store *Store, sessionID, headID string, endPosition int) {
	t.Helper()
	assertCompactionEndPosition(t, ctx, store, sessionID, headID, endPosition, false)
}

func assertCompactionEndPosition(t *testing.T, ctx context.Context, store *Store, sessionID, headID string, endPosition int, want bool) {
	t.Helper()
	var count int
	if err := store.DB().QueryRowContext(ctx, `
		select count(*)
		from conversation_compactions
		where session_id = ? and context_head_id = ? and end_position = ?
	`, sessionID, headID, endPosition).Scan(&count); err != nil {
		t.Fatalf("count conversation_compactions error = %v", err)
	}
	if (count > 0) != want {
		t.Fatalf("conversation compaction end_position %d present = %v, want %v", endPosition, count > 0, want)
	}
}
