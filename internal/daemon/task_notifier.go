package daemon

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go-agent/internal/reporting"
	rolectx "go-agent/internal/roles"
	"go-agent/internal/scheduler"
	"go-agent/internal/session"
	"go-agent/internal/store/sqlite"
	"go-agent/internal/stream"
	"go-agent/internal/task"
	"go-agent/internal/types"
	"go-agent/internal/workspace"
)

type storeAndBusSink struct {
	store eventSinkStore
	bus   *stream.Bus
}

type taskTerminalNotifier struct {
	store     taskTerminalStore
	bus       *stream.Bus
	watcher   simpleAutomationWatcherInstaller
	scheduler *scheduler.Service
	reporting *reporting.Service
	manager   *session.Manager
	now       func() time.Time
}

type eventSinkStore interface {
	AppendEventWithState(context.Context, types.Event) (types.Event, error)
	FinalizeTurn(context.Context, *types.TurnUsage, []types.Event) ([]types.Event, error)
}

type taskTerminalStore interface {
	reporting.Store
	eventSinkStore
	GetTaskRecord(context.Context, string) (types.TaskRecord, bool, error)
	UpsertTaskRecord(context.Context, types.TaskRecord) error
	CountQueuedReportDeliveries(context.Context, string) (int, error)
	GetSimpleAutomationRunByTaskID(context.Context, string) (types.SimpleAutomationRun, bool, error)
	UpsertSimpleAutomationRun(context.Context, types.SimpleAutomationRun) error
	GetAutomation(context.Context, string) (types.AutomationSpec, bool, error)
	EnsureSpecialistSession(context.Context, string, string, string, []string) (types.Session, types.ContextHead, bool, error)
	GetCurrentContextHeadID(context.Context) (string, bool, error)
	InsertTurn(context.Context, types.Turn) error
}

type simpleAutomationWatcherInstaller interface {
	Reinstall(context.Context, types.AutomationSpec) (types.AutomationWatcherRuntime, error)
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
	reportingService.SetReportReadySink(func(ctx context.Context, sessionID, turnID string, item types.ReportDeliveryItem) error {
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
	if err := n.reconcileSimpleAutomationTask(ctx, completed); err != nil {
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
		targetSessionID, err := n.resolveMainAgentReportSession(ctx, completed)
		if err != nil {
			return err
		}
		return n.deliverTaskReport(ctx, completed, targetSessionID, "", now)
	}

	if strings.TrimSpace(completed.ParentSessionID) == "" {
		return nil
	}

	if err := n.deliverTaskReport(ctx, completed, completed.ParentSessionID, "", now); err != nil {
		return err
	}

	queuedCount, err := n.store.CountQueuedReportDeliveries(ctx, completed.ParentSessionID)
	if err != nil {
		return err
	}
	noticeText := "report queued"
	if queuedCount > 1 {
		noticeText = fmt.Sprintf("%d reports queued", queuedCount)
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

func (n taskTerminalNotifier) reconcileSimpleAutomationTask(ctx context.Context, completed task.Task) error {
	if n.store == nil || strings.TrimSpace(completed.ID) == "" {
		return nil
	}
	if !strings.EqualFold(strings.TrimSpace(completed.Kind), "automation_simple") {
		return nil
	}

	run, ok, err := n.store.GetSimpleAutomationRunByTaskID(ctx, completed.ID)
	if err != nil || !ok {
		return err
	}

	now := n.currentTime()
	run.LastStatus = automationRunStatusFromTask(completed)
	run.LastSummary = simpleAutomationTaskOutcomeSummary(completed, run.LastSummary)
	if strings.TrimSpace(run.TaskID) == "" {
		run.TaskID = strings.TrimSpace(completed.ID)
	}
	if run.CreatedAt.IsZero() {
		run.CreatedAt = now
	}
	run.UpdatedAt = now
	if run.UpdatedAt.Before(run.CreatedAt) {
		run.UpdatedAt = run.CreatedAt
	}
	if err := n.store.UpsertSimpleAutomationRun(ctx, run); err != nil {
		return err
	}

	spec, ok, err := n.store.GetAutomation(ctx, run.AutomationID)
	if err != nil || !ok {
		return err
	}
	if !strings.EqualFold(strings.TrimSpace(string(spec.Mode)), string(types.AutomationModeSimple)) {
		return nil
	}
	if shouldResumeSimpleAutomationWatcher(spec, run.LastStatus) {
		if err := n.resumeSimpleAutomationWatcher(ctx, spec); err != nil {
			return err
		}
	}

	workspaceRoot := firstNonEmptyTrimmed(spec.WorkspaceRoot, completed.WorkspaceRoot)
	primaryTarget := resolveSimpleAutomationPrimaryTarget(spec)
	if err := n.deliverSimpleAutomationReport(ctx, completed, workspaceRoot, primaryTarget, now); err != nil {
		return err
	}

	if !shouldEscalateSimpleAutomationOutcome(spec, run.LastStatus) {
		return nil
	}
	escalationTarget := resolveSimpleAutomationEscalationTarget(spec)
	if escalationTarget == primaryTarget {
		return nil
	}
	return n.deliverSimpleAutomationReport(ctx, completed, workspaceRoot, escalationTarget, now)
}

func (n taskTerminalNotifier) resumeSimpleAutomationWatcher(ctx context.Context, spec types.AutomationSpec) error {
	if n.watcher == nil {
		return nil
	}
	_, err := n.watcher.Reinstall(ctx, spec)
	return err
}

func shouldResumeSimpleAutomationWatcher(spec types.AutomationSpec, status string) bool {
	if spec.State != types.AutomationStateActive {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "success":
		return strings.EqualFold(strings.TrimSpace(spec.SimplePolicy.OnSuccess), "continue")
	case "failure":
		return strings.EqualFold(strings.TrimSpace(spec.SimplePolicy.OnFailure), "continue")
	case "blocked":
		return strings.EqualFold(strings.TrimSpace(spec.SimplePolicy.OnBlocked), "continue")
	default:
		return false
	}
}

func (n taskTerminalNotifier) deliverSimpleAutomationReport(ctx context.Context, completed task.Task, workspaceRoot, reportTarget string, now time.Time) error {
	if n.store == nil {
		return nil
	}
	sessionID, err := n.resolveSimpleAutomationTargetSession(ctx, workspaceRoot, reportTarget)
	if err != nil || strings.TrimSpace(sessionID) == "" {
		return err
	}
	return n.deliverTaskReport(ctx, completed, sessionID, "", now)
}

func (n taskTerminalNotifier) deliverTaskReport(ctx context.Context, completed task.Task, targetSessionID, targetRoleID string, now time.Time) error {
	if n.store == nil || strings.TrimSpace(targetSessionID) == "" {
		return nil
	}
	workspaceRoot := strings.TrimSpace(completed.WorkspaceRoot)
	if workspaceRoot == "" {
		if sessionRow, ok, err := n.store.GetSession(ctx, targetSessionID); err != nil {
			return err
		} else if ok {
			workspaceRoot = strings.TrimSpace(sessionRow.WorkspaceRoot)
		}
	}
	report, ok := reporting.ReportFromTaskOutcome(workspaceRoot, completed, now)
	if !ok {
		return nil
	}
	report.SessionID = strings.TrimSpace(targetSessionID)
	report.TargetSessionID = strings.TrimSpace(targetSessionID)
	report.TargetRoleID = strings.TrimSpace(targetRoleID)
	report.Audience = reportAudienceForTargetRole(targetRoleID)
	report.SourceTurnID = strings.TrimSpace(completed.ParentTurnID)
	report.SourceRoleID = sourceRoleIDFromTask(completed)
	report.SourceSessionID = strings.TrimSpace(completed.ParentSessionID)
	if sourceSessionID, err := n.resolveTaskSourceSessionID(ctx, completed, workspaceRoot); err != nil {
		return err
	} else if sourceSessionID != "" {
		report.SourceSessionID = sourceSessionID
	}

	delivery := reporting.DeliveryFromReport(report, now)
	item := types.ReportDeliveryItemFromRecordDelivery(report, delivery)
	if err := n.store.UpsertReport(ctx, report); err != nil {
		return err
	}
	if err := n.store.UpsertReportDelivery(ctx, delivery); err != nil {
		return err
	}
	if err := n.emitReportReady(ctx, targetSessionID, completed.ParentTurnID, item); err != nil {
		return err
	}
	return n.enqueueSyntheticReportTurn(ctx, targetSessionID)
}

func (n taskTerminalNotifier) emitReportReady(ctx context.Context, sessionID, turnID string, item types.ReportDeliveryItem) error {
	if n.store == nil || n.bus == nil || strings.TrimSpace(sessionID) == "" {
		return nil
	}
	eventSink := storeAndBusSink{store: n.store, bus: n.bus}
	event, err := types.NewEvent(sessionID, turnID, types.EventReportReady, item)
	if err != nil {
		return err
	}
	return eventSink.Emit(ctx, event)
}

func (n taskTerminalNotifier) resolveSimpleAutomationTargetSession(ctx context.Context, workspaceRoot, target string) (string, error) {
	if n.store == nil {
		return "", nil
	}
	workspaceRoot = strings.TrimSpace(workspaceRoot)
	if workspaceRoot == "" {
		return "", nil
	}
	target = strings.TrimSpace(target)
	if target == "" {
		target = "main_agent"
	}
	if target != "main_agent" {
		return "", fmt.Errorf("simple automation report target must be main_agent")
	}
	sessionRow, _, _, err := n.store.EnsureRoleSession(ctx, workspaceRoot, types.SessionRoleMainParent)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(sessionRow.ID), nil
}

func (n taskTerminalNotifier) resolveMainAgentReportSession(ctx context.Context, completed task.Task) (string, error) {
	if n.store == nil {
		return "", nil
	}
	workspaceRoot := strings.TrimSpace(completed.WorkspaceRoot)
	if workspaceRoot == "" && strings.TrimSpace(completed.ParentSessionID) != "" {
		sessionRow, ok, err := n.store.GetSession(ctx, completed.ParentSessionID)
		if err != nil {
			return "", err
		}
		if ok {
			workspaceRoot = strings.TrimSpace(sessionRow.WorkspaceRoot)
		}
	}
	if workspaceRoot == "" {
		return strings.TrimSpace(completed.ParentSessionID), nil
	}
	sessionRow, _, _, err := n.store.EnsureRoleSession(ctx, workspaceRoot, types.SessionRoleMainParent)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(sessionRow.ID), nil
}

func resolveSimpleAutomationPrimaryTarget(spec types.AutomationSpec) string {
	target := strings.TrimSpace(spec.ReportTarget)
	if target == "" {
		target = "main_agent"
	}
	return target
}

func resolveSimpleAutomationEscalationTarget(spec types.AutomationSpec) string {
	target := strings.TrimSpace(spec.EscalationTarget)
	if target == "" {
		target = "main_agent"
	}
	return target
}

func shouldEscalateSimpleAutomationOutcome(spec types.AutomationSpec, status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "failure":
		return strings.EqualFold(strings.TrimSpace(spec.SimplePolicy.OnFailure), "escalate")
	case "blocked":
		return strings.EqualFold(strings.TrimSpace(spec.SimplePolicy.OnBlocked), "escalate")
	default:
		return false
	}
}

func simpleAutomationTaskOutcomeSummary(completed task.Task, fallback string) string {
	result, ready := completed.FinalResult()
	resultSummary := ""
	if ready {
		resultSummary = clampTaskResultPreview(result.Text)
	}
	return firstNonEmptyTrimmed(completed.OutcomeSummary, resultSummary, completed.Error, fallback)
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

func automationRunStatusFromTask(completed task.Task) string {
	switch completed.Outcome {
	case types.ChildAgentOutcomeBlocked:
		return "blocked"
	case types.ChildAgentOutcomeFailure:
		return "failure"
	case types.ChildAgentOutcomeSuccess:
		return "success"
	}
	switch completed.Status {
	case task.TaskStatusFailed, task.TaskStatusStopped:
		return "failure"
	default:
		return "success"
	}
}

func sourceRoleIDFromTask(completed task.Task) string {
	roleID := strings.TrimSpace(completed.TargetRole)
	if roleID == "" || roleID == string(types.SessionRoleMainParent) {
		return ""
	}
	return roleID
}

func reportAudienceForTargetRole(targetRoleID string) types.ReportAudience {
	if strings.TrimSpace(targetRoleID) == "" {
		return types.ReportAudienceMainAgent
	}
	return types.ReportAudienceRole
}

func (n taskTerminalNotifier) resolveTaskSourceSessionID(ctx context.Context, completed task.Task, workspaceRoot string) (string, error) {
	if n.store == nil {
		return "", nil
	}
	roleID := sourceRoleIDFromTask(completed)
	if roleID == "" {
		return strings.TrimSpace(completed.ParentSessionID), nil
	}
	workspaceRoot = strings.TrimSpace(workspaceRoot)
	if workspaceRoot == "" {
		return "", nil
	}
	roleID, err := rolectx.CanonicalRoleID(roleID)
	if err != nil {
		return "", err
	}
	spec, err := loadInstalledSpecialistRole(workspaceRoot, roleID)
	if err != nil {
		return "", err
	}
	sessionRow, _, _, err := n.store.EnsureSpecialistSession(ctx, workspaceRoot, roleID, spec.Prompt, spec.SkillNames)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(sessionRow.ID), nil
}

func loadInstalledSpecialistRole(workspaceRoot, roleID string) (rolectx.Spec, error) {
	workspaceRoot = strings.TrimSpace(workspaceRoot)
	roleID, err := rolectx.CanonicalRoleID(roleID)
	if err != nil {
		return rolectx.Spec{}, err
	}
	catalog, err := rolectx.LoadCatalog(workspaceRoot)
	if err != nil {
		return rolectx.Spec{}, err
	}
	spec, ok := catalog.ByID[roleID]
	if !ok {
		return rolectx.Spec{}, fmt.Errorf("specialist role is not installed: %s", roleID)
	}
	return spec, nil
}

func (n taskTerminalNotifier) enqueueSyntheticReportTurn(ctx context.Context, sessionID string) error {
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
		if state.QueuedReportBatches > 0 {
			return nil
		}
	}

	now := n.currentTime()
	turn := types.Turn{
		ID:        types.NewID("turn"),
		SessionID: sessionID,
		Kind:      types.TurnKindReportBatch,
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

func firstNonEmptyTrimmed(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
