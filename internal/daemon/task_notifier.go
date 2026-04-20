package daemon

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go-agent/internal/automation"
	"go-agent/internal/reporting"
	"go-agent/internal/scheduler"
	"go-agent/internal/session"
	"go-agent/internal/store/sqlite"
	"go-agent/internal/stream"
	"go-agent/internal/task"
	"go-agent/internal/types"
	"go-agent/internal/workspace"
)

type storeAndBusSink struct {
	store *sqlite.Store
	bus   *stream.Bus
}

type taskTerminalNotifier struct {
	store     *sqlite.Store
	bus       *stream.Bus
	scheduler *scheduler.Service
	reporting *reporting.Service
	delivery  *automation.DeliveryService
	watcher   automation.DispatchWatcherSyncer
	manager   *session.Manager
	now       func() time.Time
}

func (s storeAndBusSink) Emit(ctx context.Context, event types.Event) error {
	persisted, err := s.store.AppendEventWithState(ctx, event)
	if err != nil {
		return err
	}
	s.bus.Publish(persisted)
	return nil
}

func (s storeAndBusSink) FinalizeTurn(ctx context.Context, usage *types.TurnUsage, events []types.Event) error {
	persisted, err := s.store.FinalizeTurn(ctx, usage, events)
	if err != nil {
		return err
	}
	for _, event := range persisted {
		s.bus.Publish(event)
	}
	return nil
}

func buildTaskTerminalNotifier(store *sqlite.Store, bus *stream.Bus, workspaceRoot string) *taskTerminalNotifier {
	if store == nil || bus == nil {
		return nil
	}
	reportingService := reporting.NewService(store)
	reportingService.SetWorkspaceRoot(workspaceRoot)
	reportingService.SetReportReadySink(func(ctx context.Context, sessionID, turnID string, item types.ReportMailboxItem) error {
		sessionID = strings.TrimSpace(sessionID)
		if sessionID == "" {
			return nil
		}
		eventSink := storeAndBusSink{store: store, bus: bus}
		event, err := types.NewEvent(sessionID, turnID, types.EventReportReady, item)
		if err != nil {
			return err
		}
		return eventSink.Emit(ctx, event)
	})
	return &taskTerminalNotifier{
		store:     store,
		bus:       bus,
		reporting: reportingService,
	}
}

func (n taskTerminalNotifier) NotifyTaskTerminal(ctx context.Context, completed task.Task) error {
	if n.store == nil || strings.TrimSpace(completed.ID) == "" {
		return nil
	}

	now := n.currentTime()
	runtimeTask, ok, err := n.store.GetTaskRecord(ctx, completed.ID)
	if err != nil {
		return err
	}
	if ok {
		runtimeTask.State = runtimeTaskStateFromTaskStatus(completed.Status)
		runtimeTask.Title = firstNonEmptyTrimmed(runtimeTask.Title, completed.Command, completed.ExecutionTaskID, completed.ID)
		runtimeTask.Description = firstNonEmptyTrimmed(completed.Description, runtimeTask.Description)
		runtimeTask.Owner = firstNonEmptyTrimmed(completed.Owner, runtimeTask.Owner)
		runtimeTask.Kind = firstNonEmptyTrimmed(completed.Kind, runtimeTask.Kind)
		runtimeTask.ExecutionTaskID = firstNonEmptyTrimmed(runtimeTask.ExecutionTaskID, completed.ExecutionTaskID, completed.ID)
		runtimeTask.WorktreeID = firstNonEmptyTrimmed(completed.WorktreeID, runtimeTask.WorktreeID)
		runtimeTask.UpdatedAt = now
		if err := n.store.UpsertTaskRecord(ctx, runtimeTask); err != nil {
			return err
		}
	}
	if n.scheduler != nil {
		if err := n.scheduler.RecordTaskTerminal(ctx, completed); err != nil {
			return err
		}
	}
	if err := n.reconcileAutomationDispatchTask(ctx, completed); err != nil {
		return err
	}

	updatedBlock := timelineBlockFromCompletedTask(completed, runtimeTask, ok)
	eventSink := storeAndBusSink{store: n.store, bus: n.bus}
	if strings.TrimSpace(completed.ParentSessionID) != "" {
		taskEvent, err := types.NewEvent(completed.ParentSessionID, completed.ParentTurnID, types.EventTaskUpdated, updatedBlock)
		if err != nil {
			return err
		}
		if err := eventSink.Emit(ctx, taskEvent); err != nil {
			return err
		}
	}

	if reporting.ShouldQueueTaskReport(completed) {
		var (
			reportItems []types.ReportMailboxItem
			ok          bool
			err         error
		)
		if strings.TrimSpace(completed.ScheduledJobID) != "" {
			_, reportItems, ok, err = n.reporting.EnqueueScheduledJobReport(ctx, completed, now)
		} else {
			var reportItem types.ReportMailboxItem
			_, _, reportItem, ok, err = n.reporting.EnqueueTaskReport(ctx, completed, now)
			if ok {
				reportItems = append(reportItems, reportItem)
			}
		}
		if err != nil {
			return err
		}
		if !ok {
			return nil
		}
		for _, reportItem := range reportItems {
			targetSessionID := strings.TrimSpace(reportItem.SessionID)
			if targetSessionID == "" {
				continue
			}
			reportEvent, err := types.NewEvent(targetSessionID, completed.ParentTurnID, types.EventReportReady, reportItem)
			if err != nil {
				return err
			}
			if err := eventSink.Emit(ctx, reportEvent); err != nil {
				return err
			}
		}
		return nil
	}

	if strings.TrimSpace(completed.ParentSessionID) == "" {
		return nil
	}

	report, ok := childReportFromTask(completed, now)
	if !ok {
		return nil
	}
	if err := n.store.UpsertPendingChildReport(ctx, report); err != nil {
		return err
	}
	if err := n.enqueueSyntheticChildReportTurn(ctx, completed.ParentSessionID); err != nil {
		return err
	}

	pendingCount, err := n.store.CountPendingChildReports(ctx, completed.ParentSessionID)
	if err != nil {
		return err
	}
	noticeText := "child report queued"
	if pendingCount > 1 {
		noticeText = fmt.Sprintf("%d child reports queued", pendingCount)
	}
	noticeEvent, err := types.NewEvent(completed.ParentSessionID, completed.ParentTurnID, types.EventSystemNotice, types.NoticePayload{
		Text: noticeText,
	})
	if err != nil {
		return err
	}
	return eventSink.Emit(ctx, noticeEvent)
}

func (n taskTerminalNotifier) currentTime() time.Time {
	if n.now != nil {
		return n.now().UTC()
	}
	return time.Now().UTC()
}

func timelineBlockFromCompletedTask(completed task.Task, runtimeTask types.Task, hasRuntimeTask bool) types.TimelineBlock {
	if hasRuntimeTask {
		block := types.TimelineBlockFromTask(runtimeTask)
		if block.Title == "" {
			block.Title = firstNonEmptyTrimmed(completed.Command, completed.ExecutionTaskID, completed.ID)
		}
		if block.Text == "" {
			block.Text = firstNonEmptyTrimmed(completed.Description, completed.Owner)
		}
		return block
	}
	return types.TimelineBlock{
		ID:         completed.ID,
		TurnID:     completed.ParentTurnID,
		Kind:       "task_block",
		Status:     string(runtimeTaskStateFromTaskStatus(completed.Status)),
		Title:      firstNonEmptyTrimmed(completed.Command, completed.ExecutionTaskID, completed.ID),
		Text:       firstNonEmptyTrimmed(completed.Description, completed.Owner),
		TaskID:     completed.ID,
		WorktreeID: completed.WorktreeID,
	}
}

func childReportFromTask(completed task.Task, now time.Time) (types.ChildReport, bool) {
	result, ready := completed.FinalResult()
	if !ready && strings.TrimSpace(completed.OutcomeSummary) == "" && strings.TrimSpace(completed.Error) == "" && completed.Status == task.TaskStatusRunning {
		return types.ChildReport{}, false
	}
	report := types.ChildReport{
		ID:            completed.ID,
		SessionID:     completed.ParentSessionID,
		ParentTurnID:  completed.ParentTurnID,
		TaskID:        completed.ID,
		TaskType:      string(completed.Type),
		TaskKind:      completed.Kind,
		Source:        childReportSourceFromTask(completed),
		Status:        childReportStatusFromTask(completed),
		Objective:     firstNonEmptyTrimmed(completed.Description, completed.Command),
		ResultReady:   ready,
		Command:       completed.Command,
		Description:   completed.Description,
		ResultKind:    string(result.Kind),
		ResultText:    result.Text,
		ResultPreview: clampTaskResultPreview(firstNonEmptyTrimmed(result.Text, completed.OutcomeSummary, completed.Error)),
		ObservedAt:    firstNonZeroTime(result.ObservedAt, now),
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	return report, true
}

func runtimeTaskStateFromTaskStatus(status task.TaskStatus) types.TaskState {
	switch status {
	case task.TaskStatusRunning:
		return types.TaskStateRunning
	case task.TaskStatusCompleted:
		return types.TaskStateCompleted
	case task.TaskStatusStopped:
		return types.TaskStateCancelled
	case task.TaskStatusFailed:
		return types.TaskStateFailed
	default:
		return types.TaskStatePending
	}
}

func childReportSourceFromTask(completed task.Task) types.ChildReportSource {
	if strings.TrimSpace(completed.ScheduledJobID) != "" || strings.EqualFold(strings.TrimSpace(completed.Kind), "scheduled_report") {
		return types.ChildReportSourceCron
	}
	if strings.TrimSpace(completed.ParentSessionID) != "" {
		return types.ChildReportSourceChat
	}
	return types.ChildReportSourceAutomation
}

func childReportStatusFromTask(completed task.Task) types.ChildReportStatus {
	switch completed.Outcome {
	case types.ChildAgentOutcomeBlocked:
		return types.ChildReportStatusBlocked
	case types.ChildAgentOutcomeFailure:
		return types.ChildReportStatusFailure
	case types.ChildAgentOutcomeSuccess:
		return types.ChildReportStatusSuccess
	}
	switch completed.Status {
	case task.TaskStatusFailed, task.TaskStatusStopped:
		return types.ChildReportStatusFailure
	default:
		return types.ChildReportStatusSuccess
	}
}

func (n taskTerminalNotifier) enqueueSyntheticChildReportTurn(ctx context.Context, sessionID string) error {
	if n.store == nil || n.manager == nil || strings.TrimSpace(sessionID) == "" {
		return nil
	}
	sessionRow, ok, err := n.store.GetSession(ctx, sessionID)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	ctx = workspace.WithWorkspaceRoot(ctx, strings.TrimSpace(sessionRow.WorkspaceRoot))

	if state, ok := n.manager.GetRuntimeState(sessionID); ok {
		if state.QueuedChildReportBatches > 0 {
			return nil
		}
	}

	now := n.currentTime()
	turn := types.Turn{
		ID:        types.NewID("turn"),
		SessionID: sessionID,
		Kind:      types.TurnKindChildReportBatch,
		State:     types.TurnStateCreated,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if headID, ok, err := n.store.GetCurrentContextHeadID(ctx); err == nil && ok {
		turn.ContextHeadID = strings.TrimSpace(headID)
	}
	if err := n.store.InsertTurn(ctx, turn); err != nil {
		return err
	}
	_, err = n.manager.SubmitTurn(ctx, sessionID, session.SubmitTurnInput{Turn: turn})
	return err
}

func clampTaskResultPreview(text string) string {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return ""
	}
	const maxLen = 480
	runes := []rune(trimmed)
	if len(runes) <= maxLen {
		return trimmed
	}
	return string(runes[:maxLen]) + "..."
}

func firstNonZeroTime(values ...time.Time) time.Time {
	for _, value := range values {
		if !value.IsZero() {
			return value.UTC()
		}
	}
	return time.Time{}
}

func firstNonEmptyTrimmed(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
