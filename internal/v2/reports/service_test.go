package reports

import (
	"context"
	"strings"
	"testing"
	"time"

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

func TestServiceDeliverTaskReportSubmitsReportBatchWhenIdle(t *testing.T) {
	ctx := context.Background()
	s, err := store.OpenInMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	createTestSession(t, s, "main_session", "/workspace")

	mgr := &fakeReportSessionManager{ok: true}
	svc := NewService(s, mgr)
	task := contracts.Task{
		ID:        "task_idle_report",
		SessionID: "main_session",
		Kind:      "agent",
		State:     "completed",
		Outcome:   "success",
		FinalText: "Role completed the investigation.",
	}
	if err := svc.DeliverTaskReport(ctx, task); err != nil {
		t.Fatal(err)
	}

	reports, err := s.Reports().ListBySession(ctx, task.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if len(reports) != 1 {
		t.Fatalf("expected 1 report, got %d", len(reports))
	}
	if !reports[0].Delivered {
		t.Fatalf("expected report to be marked delivered: %+v", reports[0])
	}
	if len(mgr.submitted) != 1 {
		t.Fatalf("expected 1 submitted report batch, got %d", len(mgr.submitted))
	}
	submitted := mgr.submitted[0]
	if submitted.sessionID != task.SessionID {
		t.Fatalf("expected submit to %q, got %q", task.SessionID, submitted.sessionID)
	}
	if submitted.input.Turn.Kind != "report_batch" {
		t.Fatalf("expected report_batch turn, got %+v", submitted.input.Turn)
	}
	message := submitted.input.Turn.UserMessage
	for _, want := range []string{reports[0].ID, task.ID, "task_result", task.FinalText} {
		if !strings.Contains(message, want) {
			t.Fatalf("expected report batch message to contain %q:\n%s", want, message)
		}
	}
	storedTurn, err := s.Turns().Get(ctx, submitted.input.Turn.ID)
	if err != nil {
		t.Fatal(err)
	}
	if storedTurn.Kind != "report_batch" || storedTurn.SessionID != task.SessionID {
		t.Fatalf("unexpected stored turn: %+v", storedTurn)
	}
}

func TestServiceDeliverTaskReportLeavesQueuedWhenSessionBusy(t *testing.T) {
	ctx := context.Background()
	s, err := store.OpenInMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	createTestSession(t, s, "main_session", "/workspace")

	mgr := &fakeReportSessionManager{
		ok: true,
		queue: contracts.QueuePayload{
			ActiveTurnID:   "turn_active",
			ActiveTurnKind: "user_message",
		},
	}
	svc := NewService(s, mgr)
	task := contracts.Task{
		ID:        "task_busy_report",
		SessionID: "main_session",
		Kind:      "agent",
		State:     "completed",
		Outcome:   "success",
		FinalText: "done",
	}
	if err := svc.DeliverTaskReport(ctx, task); err != nil {
		t.Fatal(err)
	}

	reports, err := s.Reports().ListBySession(ctx, task.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if len(reports) != 1 {
		t.Fatalf("expected 1 report, got %d", len(reports))
	}
	if reports[0].Delivered {
		t.Fatalf("expected busy-session report to remain queued: %+v", reports[0])
	}
	if len(mgr.submitted) != 0 {
		t.Fatalf("expected no submitted report batches, got %d", len(mgr.submitted))
	}
}

func TestServiceFlushWorkspaceSubmitsQueuedReports(t *testing.T) {
	ctx := context.Background()
	s, err := store.OpenInMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	createTestSession(t, s, "main_session", "/workspace")
	createTestSession(t, s, "other_session", "/other")

	report := contracts.Report{
		ID:         "report_queued",
		SessionID:  "main_session",
		SourceKind: "task_result",
		SourceID:   "task_queued",
		Status:     "completed",
		Severity:   "info",
		Title:      "Task result: agent",
		Summary:    "queued output",
		CreatedAt:  time.Now().UTC(),
	}
	if err := s.Reports().Create(ctx, report); err != nil {
		t.Fatal(err)
	}

	mgr := &fakeReportSessionManager{ok: true}
	svc := NewService(s, mgr)
	if err := svc.FlushWorkspace(ctx, "/workspace"); err != nil {
		t.Fatal(err)
	}

	delivered, err := s.Reports().Get(ctx, report.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !delivered.Delivered {
		t.Fatalf("expected queued report to be delivered: %+v", delivered)
	}
	if len(mgr.submitted) != 1 {
		t.Fatalf("expected 1 submitted report batch, got %d", len(mgr.submitted))
	}
	if mgr.submitted[0].sessionID != "main_session" {
		t.Fatalf("expected main session submit, got %q", mgr.submitted[0].sessionID)
	}
}

func TestServiceSubmitQueuedKeepsReportsWhenSessionUnregistered(t *testing.T) {
	ctx := context.Background()
	s, err := store.OpenInMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	createTestSession(t, s, "main_session", "/workspace")

	report := contracts.Report{
		ID:         "report_waiting",
		SessionID:  "main_session",
		SourceKind: "task_result",
		SourceID:   "task_waiting",
		Status:     "completed",
		Severity:   "info",
		Title:      "Task result: agent",
		Summary:    "waiting output",
		CreatedAt:  time.Now().UTC(),
	}
	if err := s.Reports().Create(ctx, report); err != nil {
		t.Fatal(err)
	}

	mgr := &fakeReportSessionManager{ok: false}
	svc := NewService(s, mgr)
	if err := svc.SubmitQueued(ctx, "main_session"); err != nil {
		t.Fatal(err)
	}

	queued, err := s.Reports().Get(ctx, report.ID)
	if err != nil {
		t.Fatal(err)
	}
	if queued.Delivered {
		t.Fatalf("expected unregistered-session report to remain queued: %+v", queued)
	}
	if len(mgr.submitted) != 0 {
		t.Fatalf("expected no submitted report batches, got %d", len(mgr.submitted))
	}
}

type fakeReportSessionManager struct {
	queue     contracts.QueuePayload
	ok        bool
	submitted []submittedReportTurn
}

type submittedReportTurn struct {
	sessionID string
	input     contracts.SubmitTurnInput
}

func (m *fakeReportSessionManager) SubmitTurn(_ context.Context, sessionID string, input contracts.SubmitTurnInput) (string, error) {
	m.submitted = append(m.submitted, submittedReportTurn{sessionID: sessionID, input: input})
	return input.Turn.ID, nil
}

func (m *fakeReportSessionManager) QueuePayload(string) (contracts.QueuePayload, bool) {
	return m.queue, m.ok
}

func createTestSession(t *testing.T, s contracts.Store, id, workspaceRoot string) {
	t.Helper()
	now := time.Now().UTC()
	if err := s.Sessions().Create(context.Background(), contracts.Session{
		ID:                id,
		WorkspaceRoot:     workspaceRoot,
		SystemPrompt:      "system",
		PermissionProfile: "default",
		State:             "idle",
		CreatedAt:         now,
		UpdatedAt:         now,
	}); err != nil {
		t.Fatal(err)
	}
}
