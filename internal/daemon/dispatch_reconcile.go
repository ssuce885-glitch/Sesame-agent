package daemon

import (
	"context"
	"strings"
	"time"

	"go-agent/internal/automation"
	"go-agent/internal/task"
	"go-agent/internal/types"
)

func (n taskTerminalNotifier) reconcileAutomationDispatchTask(ctx context.Context, completed task.Task) error {
	if n.store == nil || strings.TrimSpace(completed.ID) == "" {
		return nil
	}

	attempt, ok, err := n.store.FindDispatchAttemptByTaskID(ctx, completed.ID)
	if err != nil || !ok {
		return err
	}

	now := n.currentTime()
	attempt.Outcome = canonicalDispatchTaskOutcome(completed)
	attempt.OutcomeSummary = firstNonEmptyTrimmed(completed.OutcomeSummary, attempt.OutcomeSummary)

	request, continuation, awaitingApproval, err := n.pendingPermissionForTask(ctx, completed.ID)
	if err != nil {
		return err
	}
	if awaitingApproval && attempt.Outcome == types.ChildAgentOutcomeBlocked {
		if err := automation.ReplaceDispatchHoldWithApprovalHold(ctx, n.store, attempt.AutomationID, attempt.DispatchID, request.ID, now); err != nil {
			return err
		}
		attempt.Status = types.DispatchAttemptStatusAwaitingApproval
		attempt.PermissionRequestID = strings.TrimSpace(request.ID)
		attempt.ContinuationID = strings.TrimSpace(continuation.ID)
		attempt.Error = ""
		attempt.FinishedAt = time.Time{}
		attempt.OutcomeSummary = firstNonEmptyTrimmed(attempt.OutcomeSummary, request.Reason, "approval required")
		attempt.UpdatedAt = now
		if err := n.store.UpsertDispatchAttempt(ctx, attempt); err != nil {
			return err
		}
		if err := updateIncidentPhaseState(ctx, n.store, attempt.IncidentID, attempt.Phase, now, func(phase *types.IncidentPhaseState) {
			phase.Status = types.IncidentPhaseStatusAwaitingApproval
		}); err != nil {
			return err
		}
		return updateAutomationIncidentStatus(ctx, n.store, attempt.IncidentID, types.AutomationIncidentStatusActive, now)
	}

	succeeded := automationDispatchTaskSucceeded(completed)
	attempt.Status = types.DispatchAttemptStatusCompleted
	attempt.Error = ""
	if !succeeded {
		attempt.Status = types.DispatchAttemptStatusFailed
		attempt.Error = firstNonEmptyTrimmed(completed.Error, completed.OutcomeSummary, "task failed")
	}
	attempt.FinishedAt = now
	attempt.UpdatedAt = now
	if err := n.store.UpsertDispatchAttempt(ctx, attempt); err != nil {
		return err
	}

	reconciler := sessionRunnerAdapter{
		store:   n.store,
		watcher: n.watcher,
	}
	if err := reconciler.releaseWatcherHoldForDispatchOutcome(ctx, attempt, succeeded); err != nil {
		return err
	}
	if succeeded {
		if err := reconciler.applyDispatchOutcome(ctx, attempt, dispatchOutcomeCompleted, now); err != nil {
			return err
		}
		return n.deliverAutomationDispatchResult(ctx, attempt, completed, now)
	}
	return reconciler.applyDispatchOutcome(ctx, attempt, dispatchOutcomeFailed, now)
}

func (n taskTerminalNotifier) pendingPermissionForTask(ctx context.Context, taskID string) (types.PermissionRequest, types.TurnContinuation, bool, error) {
	if n.store == nil || strings.TrimSpace(taskID) == "" {
		return types.PermissionRequest{}, types.TurnContinuation{}, false, nil
	}
	requests, err := n.store.ListPermissionRequestsByTask(ctx, taskID)
	if err != nil {
		return types.PermissionRequest{}, types.TurnContinuation{}, false, err
	}
	var selected types.PermissionRequest
	found := false
	for _, request := range requests {
		if request.Status != types.PermissionRequestStatusRequested {
			continue
		}
		if !found || request.CreatedAt.After(selected.CreatedAt) {
			selected = request
			found = true
		}
	}
	if !found {
		return types.PermissionRequest{}, types.TurnContinuation{}, false, nil
	}
	continuation, ok, err := n.store.GetTurnContinuationByPermissionRequest(ctx, selected.ID)
	if err != nil || !ok {
		return types.PermissionRequest{}, types.TurnContinuation{}, false, err
	}
	return selected, continuation, true, nil
}

func (n taskTerminalNotifier) deliverAutomationDispatchResult(ctx context.Context, attempt types.DispatchAttempt, completed task.Task, now time.Time) error {
	if n.delivery == nil {
		return nil
	}
	result, ready := completed.FinalResult()
	if !ready || strings.TrimSpace(result.Text) == "" {
		return nil
	}
	_, err := n.delivery.DeliverDispatchResult(ctx, attempt, types.ChildAgentResult{
		ResultID:   types.NewID("child_result"),
		AgentID:    firstNonEmptyTrimmed(attempt.ChildAgentID, "automation_dispatch"),
		ContractID: strings.TrimSpace(attempt.OutputContractRef),
		TaskID:     strings.TrimSpace(completed.ID),
		ObservedAt: result.ObservedAt,
		Envelope: types.ReportEnvelope{
			Source:  string(types.ReportMailboxSourceChildAgentResult),
			Status:  "completed",
			Title:   firstNonEmptyTrimmed(attempt.ChildAgentID, "Automation dispatch result"),
			Summary: summarizeDispatchResult(result.Text),
			Sections: []types.ReportSectionContent{{
				ID:    "report_body",
				Title: "Result",
				Text:  strings.TrimSpace(result.Text),
			}},
		},
		CreatedAt: now,
		UpdatedAt: now,
	})
	return err
}

func canonicalDispatchTaskOutcome(completed task.Task) types.ChildAgentOutcome {
	switch normalized := types.ChildAgentOutcome(strings.ToLower(strings.TrimSpace(string(completed.Outcome)))); normalized {
	case types.ChildAgentOutcomeSuccess, types.ChildAgentOutcomeFailure, types.ChildAgentOutcomeBlocked:
		return normalized
	}
	switch completed.Status {
	case task.TaskStatusCompleted:
		return types.ChildAgentOutcomeSuccess
	case task.TaskStatusFailed, task.TaskStatusStopped:
		return types.ChildAgentOutcomeFailure
	default:
		return ""
	}
}

func automationDispatchTaskSucceeded(completed task.Task) bool {
	return completed.Status == task.TaskStatusCompleted && canonicalDispatchTaskOutcome(completed) == types.ChildAgentOutcomeSuccess
}

func summarizeDispatchResult(text string) string {
	trimmed := strings.TrimSpace(text)
	if len(trimmed) <= 160 {
		return trimmed
	}
	return strings.TrimSpace(trimmed[:157]) + "..."
}
