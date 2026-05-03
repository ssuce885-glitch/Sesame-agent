package reports

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"go-agent/internal/types"
	"go-agent/internal/v2/contracts"
)

type Service struct {
	store      contracts.Store
	sessionMgr reportSessionManager
	mu         sync.Mutex
}

type reportSessionManager interface {
	SubmitTurn(ctx context.Context, sessionID string, input contracts.SubmitTurnInput) (string, error)
	QueuePayload(sessionID string) (contracts.QueuePayload, bool)
}

func NewService(s contracts.Store, sessionMgr ...reportSessionManager) *Service {
	svc := &Service{store: s}
	if len(sessionMgr) > 0 {
		svc.sessionMgr = sessionMgr[0]
	}
	return svc
}

// DeliverTaskReport creates a report from a completed task and delivers it
// to the task's report target session. Role tasks use ReportSessionID to route
// results back to the parent/main session while preserving their specialist
// session for execution history.
func (s *Service) DeliverTaskReport(ctx context.Context, task contracts.Task) error {
	sessionID := task.ReportSessionID
	if sessionID == "" {
		sessionID = task.SessionID
	}
	report := contracts.Report{
		ID:         types.NewID("report"),
		SessionID:  sessionID,
		SourceKind: "task_result",
		SourceID:   task.ID,
		Status:     task.State,
		Severity:   severityFromOutcome(task.Outcome),
		Title:      "Task result: " + task.Kind,
		Summary:    task.FinalText,
		CreatedAt:  time.Now().UTC(),
	}
	if err := s.store.Reports().Create(ctx, report); err != nil {
		return err
	}
	return s.SubmitQueued(ctx, sessionID)
}

// FlushWorkspace tries to deliver queued reports for every session in a workspace.
func (s *Service) FlushWorkspace(ctx context.Context, workspaceRoot string) error {
	sessions, err := s.store.Sessions().ListByWorkspace(ctx, strings.TrimSpace(workspaceRoot))
	if err != nil {
		return err
	}
	for _, session := range sessions {
		if err := s.SubmitQueued(ctx, session.ID); err != nil {
			return err
		}
	}
	return nil
}

// SubmitQueued submits one synthetic report_batch turn when the target session is idle.
func (s *Service) SubmitQueued(ctx context.Context, sessionID string) error {
	if s.sessionMgr == nil || strings.TrimSpace(sessionID) == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	queue, ok := s.sessionMgr.QueuePayload(sessionID)
	if !ok {
		return nil
	}
	if queue.ActiveTurnID != "" || queue.QueueDepth > 0 || queue.QueuedReportBatches > 0 {
		return nil
	}
	reports, err := s.store.Reports().ListBySession(ctx, sessionID)
	if err != nil {
		return err
	}
	queued := make([]contracts.Report, 0, len(reports))
	for _, report := range reports {
		if !report.Delivered {
			queued = append(queued, report)
		}
	}
	if len(queued) == 0 {
		return nil
	}
	now := time.Now().UTC()
	turn := contracts.Turn{
		ID:          types.NewID("turn"),
		SessionID:   sessionID,
		Kind:        "report_batch",
		State:       "created",
		UserMessage: reportBatchMessage(queued),
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := s.store.Turns().Create(ctx, turn); err != nil {
		return err
	}
	if _, err := s.sessionMgr.SubmitTurn(ctx, sessionID, contracts.SubmitTurnInput{Turn: turn}); err != nil {
		_ = s.store.Turns().UpdateState(context.WithoutCancel(ctx), turn.ID, "failed")
		return err
	}
	for _, report := range queued {
		if err := s.store.Reports().MarkDelivered(ctx, report.ID); err != nil {
			return err
		}
	}
	return nil
}

func severityFromOutcome(outcome string) string {
	if outcome == "failure" {
		return "error"
	}
	return "info"
}

func reportBatchMessage(reports []contracts.Report) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Review these completed task reports and update the user-facing conversation if action is needed.\n\n")
	for i, report := range reports {
		fmt.Fprintf(&b, "## Report %d\n", i+1)
		fmt.Fprintf(&b, "- id: %s\n", report.ID)
		fmt.Fprintf(&b, "- source: %s %s\n", report.SourceKind, report.SourceID)
		fmt.Fprintf(&b, "- status: %s\n", report.Status)
		fmt.Fprintf(&b, "- severity: %s\n", report.Severity)
		if strings.TrimSpace(report.Title) != "" {
			fmt.Fprintf(&b, "- title: %s\n", strings.TrimSpace(report.Title))
		}
		fmt.Fprintf(&b, "\n%s\n\n", strings.TrimSpace(report.Summary))
	}
	return strings.TrimSpace(b.String())
}
