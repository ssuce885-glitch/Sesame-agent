package sqlite

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go-agent/internal/types"
)

func TestReportDigestColdIndexFlow(t *testing.T) {
	ctx := context.Background()
	store, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	now := time.Now().UTC()
	if err := store.InsertSession(ctx, types.Session{
		ID:            "sess_rd",
		WorkspaceRoot: "/ws",
		State:         types.SessionStateIdle,
		CreatedAt:     now,
		UpdatedAt:     now,
	}); err != nil {
		t.Fatalf("InsertSession() error = %v", err)
	}

	reportEnvelope := types.ReportEnvelope{
		Title:   "Build Failure Investigation",
		Summary: "The build failed due to missing import in handler.go",
		Sections: []types.ReportSectionContent{{
			Text:  "Root cause: missing context import in handler.go",
			Items: []string{"Added import for context", "Ran go build - success"},
		}},
	}
	report := types.ReportRecord{
		ID:            "report_1",
		WorkspaceRoot: "/ws",
		SessionID:     "sess_rd",
		SourceKind:    types.ReportSourceTaskResult,
		SourceID:      "report_1",
		Envelope:      reportEnvelope,
		ObservedAt:    now,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := store.UpsertReport(ctx, report); err != nil {
		t.Fatalf("UpsertReport(report_1) error = %v", err)
	}
	reportSearchText := testReportEnvelopeSearchText(reportEnvelope)
	if err := store.InsertColdIndexEntry(ctx, types.ColdIndexEntry{
		ID:          "cold_report_report_1",
		WorkspaceID: "/ws",
		SourceType:  "report",
		SourceID:    "report_1",
		SearchText:  reportSearchText,
		SummaryLine: reportEnvelope.Summary,
		OccurredAt:  now,
		CreatedAt:   now,
		ContextRef: types.ColdContextRef{
			SessionID:    "sess_rd",
			TurnStartPos: 0,
			TurnEndPos:   1,
			ItemCount:    0,
		},
	}); err != nil {
		t.Fatalf("InsertColdIndexEntry(report_1) error = %v", err)
	}

	digestEnvelope := types.ReportEnvelope{
		Title:   "Daily Digest",
		Summary: "Activity summary for today",
		Sections: []types.ReportSectionContent{{
			Text:  "Memory usage normal",
			Items: []string{"3 memories deprecated", "2 new memories created"},
		}},
	}
	digest := types.DigestRecord{
		DigestID:    "digest_1",
		SessionID:   "sess_rd",
		GroupID:     "grp_1",
		WindowStart: now.Add(-1 * time.Hour),
		WindowEnd:   now,
		Envelope:    digestEnvelope,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := store.UpsertDigestRecord(ctx, digest); err != nil {
		t.Fatalf("UpsertDigestRecord(digest_1) error = %v", err)
	}
	digestSearchText := testReportEnvelopeSearchText(digestEnvelope)
	if err := store.InsertColdIndexEntry(ctx, types.ColdIndexEntry{
		ID:          "cold_digest_digest_1",
		WorkspaceID: "/ws",
		SourceType:  "digest",
		SourceID:    "digest_1",
		SearchText:  digestSearchText,
		SummaryLine: digestEnvelope.Summary,
		OccurredAt:  now,
		CreatedAt:   now,
		ContextRef: types.ColdContextRef{
			SessionID:    "sess_rd",
			TurnStartPos: 0,
			TurnEndPos:   1,
			ItemCount:    0,
		},
	}); err != nil {
		t.Fatalf("InsertColdIndexEntry(digest_1) error = %v", err)
	}

	assertReportDigestColdSearchIDs(t, ctx, store, types.ColdSearchQuery{
		WorkspaceID: "/ws",
		TextQuery:   "build failure",
		Limit:       10,
	}, "cold_report_report_1")
	assertReportDigestColdSearchIDs(t, ctx, store, types.ColdSearchQuery{
		WorkspaceID: "/ws",
		TextQuery:   "context import",
		Limit:       10,
	}, "cold_report_report_1")
	assertReportDigestColdSearchIDs(t, ctx, store, types.ColdSearchQuery{
		WorkspaceID: "/ws",
		TextQuery:   "Ran go build",
		Limit:       10,
	}, "cold_report_report_1")
	assertReportDigestColdSearchIDs(t, ctx, store, types.ColdSearchQuery{
		WorkspaceID: "/ws",
		SourceTypes: []string{"report"},
		TextQuery:   "handler",
		Limit:       10,
	}, "cold_report_report_1")
	assertReportDigestColdSearchIDs(t, ctx, store, types.ColdSearchQuery{
		WorkspaceID: "/ws",
		SourceTypes: []string{"digest"},
		TextQuery:   "memory",
		Limit:       10,
	}, "cold_digest_digest_1")
	assertReportDigestColdSearchIDs(t, ctx, store, types.ColdSearchQuery{
		WorkspaceID: "/ws",
		SourceTypes: []string{"report"},
		TextQuery:   "memory",
		Limit:       10,
	})
	assertReportDigestColdSearchIDs(t, ctx, store, types.ColdSearchQuery{
		WorkspaceID: "/ws",
		TextQuery:   "activity",
		Limit:       10,
	}, "cold_digest_digest_1")

	reportEntry, found, err := store.GetColdIndexEntry(ctx, "cold_report_report_1")
	if err != nil {
		t.Fatalf("GetColdIndexEntry(report_1) error = %v", err)
	}
	if !found {
		t.Fatalf("GetColdIndexEntry(report_1) found = false, want true")
	}
	assertReportDigestContextRef(t, reportEntry.ContextRef, "report")

	digestEntry, found, err := store.GetColdIndexEntry(ctx, "cold_digest_digest_1")
	if err != nil {
		t.Fatalf("GetColdIndexEntry(digest_1) error = %v", err)
	}
	if !found {
		t.Fatalf("GetColdIndexEntry(digest_1) found = false, want true")
	}
	assertReportDigestContextRef(t, digestEntry.ContextRef, "digest")

	oldObservedAt := now.Add(-40 * 24 * time.Hour)
	oldReportEnvelope := types.ReportEnvelope{
		Title:   "Old Build Failure Investigation",
		Summary: "Old build failed due to missing import in handler.go",
		Sections: []types.ReportSectionContent{{
			Text:  "Root cause: missing context import in handler.go",
			Items: []string{"Added import for context", "Ran go build - success"},
		}},
	}
	oldReport := types.ReportRecord{
		ID:            "report_old",
		WorkspaceRoot: "/ws",
		SessionID:     "sess_rd",
		SourceKind:    types.ReportSourceTaskResult,
		SourceID:      "report_old",
		Envelope:      oldReportEnvelope,
		ObservedAt:    oldObservedAt,
		CreatedAt:     oldObservedAt,
		UpdatedAt:     oldObservedAt,
	}
	if err := store.UpsertReport(ctx, oldReport); err != nil {
		t.Fatalf("UpsertReport(report_old) error = %v", err)
	}
	if err := store.InsertColdIndexEntry(ctx, types.ColdIndexEntry{
		ID:          "cold_report_report_old",
		WorkspaceID: "/ws",
		SourceType:  "report",
		SourceID:    "report_old",
		SearchText:  testReportEnvelopeSearchText(oldReportEnvelope),
		SummaryLine: oldReportEnvelope.Summary,
		OccurredAt:  oldObservedAt,
		CreatedAt:   oldObservedAt,
		ContextRef: types.ColdContextRef{
			SessionID:    "sess_rd",
			TurnStartPos: 0,
			TurnEndPos:   1,
			ItemCount:    0,
		},
	}); err != nil {
		t.Fatalf("InsertColdIndexEntry(report_old) error = %v", err)
	}

	assertReportDigestColdSearchIDs(t, ctx, store, types.ColdSearchQuery{
		WorkspaceID: "/ws",
		SourceTypes: []string{"report"},
		TextQuery:   "build",
		Limit:       10,
	}, "cold_report_report_1", "cold_report_report_old")

	affected, err := store.CleanupOldReports(ctx, "/ws", now.Add(-30*24*time.Hour))
	if err != nil {
		t.Fatalf("CleanupOldReports() error = %v", err)
	}
	if affected != 1 {
		t.Fatalf("CleanupOldReports() affected = %d, want 1", affected)
	}
	if _, found, err := store.GetColdIndexEntry(ctx, "cold_report_report_old"); err != nil {
		t.Fatalf("GetColdIndexEntry(report_old) after cleanup error = %v", err)
	} else if !found {
		t.Fatalf("GetColdIndexEntry(report_old) after cleanup found = false, want true")
	}
	if _, found, err := store.GetColdIndexEntry(ctx, "cold_report_report_1"); err != nil {
		t.Fatalf("GetColdIndexEntry(report_1) after cleanup error = %v", err)
	} else if !found {
		t.Fatalf("GetColdIndexEntry(report_1) after cleanup found = false, want true")
	}
}

func testReportEnvelopeSearchText(envelope types.ReportEnvelope) string {
	parts := []string{envelope.Title, envelope.Summary}
	for _, section := range envelope.Sections {
		parts = append(parts, section.Text)
		parts = append(parts, section.Items...)
	}
	return strings.Join(parts, " ")
}

func assertReportDigestColdSearchIDs(t *testing.T, ctx context.Context, store *Store, query types.ColdSearchQuery, wantIDs ...string) {
	t.Helper()
	entries, total, err := store.SearchColdIndex(ctx, query)
	if err != nil {
		t.Fatalf("SearchColdIndex(%#v) error = %v", query, err)
	}
	if total != len(wantIDs) || len(entries) != len(wantIDs) {
		t.Fatalf("SearchColdIndex(%#v) returned %d/%d entries, want %d/%d", query, len(entries), total, len(wantIDs), len(wantIDs))
	}
	got := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		got[entry.ID] = struct{}{}
	}
	for _, wantID := range wantIDs {
		if _, ok := got[wantID]; !ok {
			t.Fatalf("SearchColdIndex(%#v) IDs = %v, missing %q", query, coldEntryIDs(entries), wantID)
		}
	}
}

func assertReportDigestContextRef(t *testing.T, ref types.ColdContextRef, label string) {
	t.Helper()
	if ref.TurnStartPos != 0 || ref.TurnEndPos != 1 || ref.ItemCount != 0 {
		t.Fatalf("%s ContextRef = %#v, want turn range 0-1 with 0 items", label, ref)
	}
}

func coldEntryIDs(entries []types.ColdIndexEntry) []string {
	ids := make([]string, 0, len(entries))
	for _, entry := range entries {
		ids = append(ids, entry.ID)
	}
	return ids
}
