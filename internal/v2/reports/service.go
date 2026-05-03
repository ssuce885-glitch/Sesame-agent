package reports

import (
	"context"
	"time"

	"go-agent/internal/types"
	"go-agent/internal/v2/contracts"
)

type Service struct {
	store contracts.Store
}

func NewService(s contracts.Store) *Service { return &Service{store: s} }

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
	return s.store.Reports().Create(ctx, report)
}

func severityFromOutcome(outcome string) string {
	if outcome == "failure" {
		return "error"
	}
	return "info"
}
