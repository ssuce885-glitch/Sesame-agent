package sqlite

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"go-agent/internal/types"
)

func TestStoreClaimsPendingReportMailboxItemsPerTurn(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "agentd.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	observedAt := time.Date(2026, 4, 10, 9, 15, 0, 0, time.UTC)
	item := types.ReportMailboxItem{
		ID:         "task_result:task_report_1",
		SessionID:  "sess_report_mailbox",
		SourceKind: types.ReportMailboxSourceTaskResult,
		SourceID:   "task_report_1",
		ObservedAt: observedAt,
		Envelope: types.ReportEnvelope{
			Source:  "task_result",
			Status:  "completed",
			Title:   "Morning report",
			Summary: "all checks passed",
			Sections: []types.ReportSectionContent{{
				ID:    "body",
				Title: "Report",
				Text:  "all checks passed",
			}},
		},
	}
	if err := store.UpsertReportMailboxItem(context.Background(), item); err != nil {
		t.Fatalf("UpsertReportMailboxItem() error = %v", err)
	}

	pendingCount, err := store.CountPendingReportMailboxItems(context.Background(), "sess_report_mailbox")
	if err != nil {
		t.Fatalf("CountPendingReportMailboxItems() error = %v", err)
	}
	if pendingCount != 1 {
		t.Fatalf("pending count = %d, want 1", pendingCount)
	}

	listed, err := store.ListReportMailboxItems(context.Background(), "sess_report_mailbox")
	if err != nil {
		t.Fatalf("ListReportMailboxItems() error = %v", err)
	}
	if len(listed) != 1 {
		t.Fatalf("len(listed) = %d, want 1", len(listed))
	}
	if listed[0].InjectedTurnID != "" {
		t.Fatalf("InjectedTurnID = %q, want empty before claim", listed[0].InjectedTurnID)
	}
	if listed[0].ReportID != "task_result:task_report_1" {
		t.Fatalf("ReportID = %q, want task_result:task_report_1", listed[0].ReportID)
	}
	if listed[0].DeliveryState != types.ReportDeliveryStatePending {
		t.Fatalf("DeliveryState = %q, want %q", listed[0].DeliveryState, types.ReportDeliveryStatePending)
	}

	reports, err := store.ListReports(context.Background(), "sess_report_mailbox")
	if err != nil {
		t.Fatalf("ListReports() error = %v", err)
	}
	if len(reports) != 1 {
		t.Fatalf("len(reports) = %d, want 1", len(reports))
	}
	if reports[0].Envelope.Title != "Morning report" {
		t.Fatalf("report title = %q, want Morning report", reports[0].Envelope.Title)
	}

	deliveries, err := store.ListReportDeliveries(context.Background(), "sess_report_mailbox", types.ReportChannelMailbox)
	if err != nil {
		t.Fatalf("ListReportDeliveries() error = %v", err)
	}
	if len(deliveries) != 1 {
		t.Fatalf("len(deliveries) = %d, want 1", len(deliveries))
	}
	if deliveries[0].State != types.ReportDeliveryStatePending {
		t.Fatalf("delivery state = %q, want %q", deliveries[0].State, types.ReportDeliveryStatePending)
	}

	claimed, err := store.ClaimPendingReportMailboxItemsForTurn(context.Background(), "sess_report_mailbox", "turn_delivery")
	if err != nil {
		t.Fatalf("ClaimPendingReportMailboxItemsForTurn() error = %v", err)
	}
	if len(claimed) != 1 {
		t.Fatalf("len(claimed) = %d, want 1", len(claimed))
	}
	if claimed[0].InjectedTurnID != "turn_delivery" {
		t.Fatalf("InjectedTurnID = %q, want turn_delivery", claimed[0].InjectedTurnID)
	}

	pendingCount, err = store.CountPendingReportMailboxItems(context.Background(), "sess_report_mailbox")
	if err != nil {
		t.Fatalf("CountPendingReportMailboxItems(after claim) error = %v", err)
	}
	if pendingCount != 0 {
		t.Fatalf("pending count after claim = %d, want 0", pendingCount)
	}

	listed, err = store.ListReportMailboxItems(context.Background(), "sess_report_mailbox")
	if err != nil {
		t.Fatalf("ListReportMailboxItems(after claim) error = %v", err)
	}
	if len(listed) != 1 {
		t.Fatalf("len(listed after claim) = %d, want 1", len(listed))
	}
	if listed[0].InjectedTurnID != "turn_delivery" {
		t.Fatalf("listed InjectedTurnID = %q, want turn_delivery", listed[0].InjectedTurnID)
	}
	if listed[0].DeliveryState != types.ReportDeliveryStateDelivered {
		t.Fatalf("listed DeliveryState = %q, want %q", listed[0].DeliveryState, types.ReportDeliveryStateDelivered)
	}
}
