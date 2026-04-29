package sqlite

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"go-agent/internal/types"
)

func TestRequeueClaimedReportDeliveriesForTurnReturnsRowCount(t *testing.T) {
	ctx := context.Background()
	store, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})

	now := time.Date(2026, time.April, 24, 10, 0, 0, 0, time.UTC)
	insertReportDeliveryTestReport(t, ctx, store, "report_1", "sess_main", now)
	insertReportDeliveryTestReport(t, ctx, store, "report_other", "sess_main", now)
	insertReportDeliveryTestReport(t, ctx, store, "report_archived", "sess_main", now)
	if err := store.UpsertReportDelivery(ctx, types.ReportDelivery{
		ID:             "delivery_1",
		SessionID:      "sess_main",
		ReportID:       "report_1",
		Channel:        types.ReportChannelAgent,
		State:          types.ReportDeliveryStateDelivered,
		ObservedAt:     now,
		InjectedTurnID: "turn_report",
		InjectedAt:     now,
		CreatedAt:      now,
		UpdatedAt:      now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertReportDelivery(ctx, types.ReportDelivery{
		ID:             "delivery_other",
		SessionID:      "sess_main",
		ReportID:       "report_other",
		Channel:        types.ReportChannelAgent,
		State:          types.ReportDeliveryStateDelivered,
		ObservedAt:     now,
		InjectedTurnID: "turn_other",
		InjectedAt:     now,
		CreatedAt:      now,
		UpdatedAt:      now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertReportDelivery(ctx, types.ReportDelivery{
		ID:             "delivery_archived",
		SessionID:      "sess_main",
		ReportID:       "report_archived",
		Channel:        types.ReportChannelAgent,
		State:          types.ReportDeliveryStateArchived,
		ObservedAt:     now,
		InjectedTurnID: "turn_report",
		InjectedAt:     now,
		CreatedAt:      now,
		UpdatedAt:      now,
	}); err != nil {
		t.Fatal(err)
	}

	requeued, err := store.RequeueClaimedReportDeliveriesForTurn(ctx, "turn_report")
	if err != nil {
		t.Fatal(err)
	}
	if requeued != 1 {
		t.Fatalf("requeued rows = %d, want 1", requeued)
	}
	queuedCount, err := store.CountQueuedReportDeliveries(ctx, "sess_main")
	if err != nil {
		t.Fatal(err)
	}
	if queuedCount != 1 {
		t.Fatalf("queued count = %d, want 1", queuedCount)
	}

	deliveries, err := store.ListReportDeliveries(ctx, "sess_main", types.ReportChannelAgent)
	if err != nil {
		t.Fatal(err)
	}
	byID := make(map[string]types.ReportDelivery, len(deliveries))
	for _, delivery := range deliveries {
		byID[delivery.ID] = delivery
	}
	if got := byID["delivery_1"]; got.State != types.ReportDeliveryStateQueued || got.InjectedTurnID != "" || !got.InjectedAt.IsZero() {
		t.Fatalf("delivery_1 after requeue = %#v, want queued with no injection", got)
	}
	if got := byID["delivery_other"]; got.State != types.ReportDeliveryStateDelivered || got.InjectedTurnID != "turn_other" {
		t.Fatalf("delivery_other after requeue = %#v, want unchanged", got)
	}
	if got := byID["delivery_archived"]; got.State != types.ReportDeliveryStateArchived || got.InjectedTurnID != "turn_report" {
		t.Fatalf("delivery_archived after requeue = %#v, want unchanged", got)
	}

	requeued, err = store.RequeueClaimedReportDeliveriesForTurn(ctx, "turn_report")
	if err != nil {
		t.Fatal(err)
	}
	if requeued != 0 {
		t.Fatalf("second requeued rows = %d, want 0", requeued)
	}
}

func insertReportDeliveryTestReport(t *testing.T, ctx context.Context, store *Store, id, sessionID string, now time.Time) {
	t.Helper()

	if err := store.UpsertReport(ctx, types.ReportRecord{
		ID:         id,
		SessionID:  sessionID,
		SourceKind: types.ReportSourceTaskResult,
		SourceID:   id,
		Envelope: types.ReportEnvelope{
			Source:  string(types.ReportSourceTaskResult),
			Status:  "completed",
			Title:   id,
			Summary: "done",
		},
		ObservedAt: now,
		CreatedAt:  now,
		UpdatedAt:  now,
	}); err != nil {
		t.Fatal(err)
	}
}
