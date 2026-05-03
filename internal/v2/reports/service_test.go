package reports

import (
	"context"
	"testing"

	"go-agent/internal/v2/contracts"
	"go-agent/internal/v2/store"
)

func TestServiceDeliverTaskReport(t *testing.T) {
	s, err := store.OpenInMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	svc := NewService(s)
	task := contracts.Task{
		ID:        "task_report001",
		SessionID: "sess_1",
		Kind:      "shell",
		State:     "failed",
		Outcome:   "failure",
		FinalText: "Task failed: exit status 1",
	}
	if err := svc.DeliverTaskReport(context.Background(), task); err != nil {
		t.Fatal(err)
	}

	reports, err := s.Reports().ListBySession(context.Background(), task.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if len(reports) != 1 {
		t.Fatalf("expected 1 report, got %d", len(reports))
	}
	report := reports[0]
	if report.SourceKind != "task_result" || report.SourceID != task.ID {
		t.Fatalf("unexpected report source: %+v", report)
	}
	if report.Status != "failed" || report.Severity != "error" {
		t.Fatalf("unexpected report status/severity: %+v", report)
	}
	if report.Summary != task.FinalText {
		t.Fatalf("expected summary %q, got %q", task.FinalText, report.Summary)
	}
}

func TestServiceDeliverTaskReportUsesReportSessionID(t *testing.T) {
	s, err := store.OpenInMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	svc := NewService(s)
	task := contracts.Task{
		ID:              "task_role_report",
		SessionID:       "specialist_session",
		ReportSessionID: "main_session",
		Kind:            "agent",
		State:           "completed",
		Outcome:         "success",
		FinalText:       "done",
	}
	if err := svc.DeliverTaskReport(context.Background(), task); err != nil {
		t.Fatal(err)
	}

	reports, err := s.Reports().ListBySession(context.Background(), "main_session")
	if err != nil {
		t.Fatal(err)
	}
	if len(reports) != 1 {
		t.Fatalf("expected report in main session, got %d", len(reports))
	}
	if reports[0].SourceID != task.ID {
		t.Fatalf("unexpected report: %+v", reports[0])
	}
	specialistReports, err := s.Reports().ListBySession(context.Background(), "specialist_session")
	if err != nil {
		t.Fatal(err)
	}
	if len(specialistReports) != 0 {
		t.Fatalf("expected no specialist reports, got %+v", specialistReports)
	}
}
