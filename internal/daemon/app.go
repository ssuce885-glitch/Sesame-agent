package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	httpapi "go-agent/internal/api/http"
	"go-agent/internal/automation"
	"go-agent/internal/config"
	contextstate "go-agent/internal/context"
	"go-agent/internal/engine"
	"go-agent/internal/model"
	"go-agent/internal/permissions"
	"go-agent/internal/reporting"
	"go-agent/internal/scheduler"
	"go-agent/internal/session"
	"go-agent/internal/store/artifacts"
	"go-agent/internal/store/sqlite"
	"go-agent/internal/stream"
	"go-agent/internal/task"
	"go-agent/internal/tools"
	"go-agent/internal/types"
)

type sessionRunnerAdapter struct {
	engine   *engine.Engine
	sink     storeAndBusSink
	store    *sqlite.Store
	delivery *automation.DeliveryService
	watcher  automation.DispatchWatcherSyncer
	tasker   *task.Manager
	notifier *taskTerminalNotifier
	now      func() time.Time
}

type storeAndBusSink struct {
	store *sqlite.Store
	bus   *stream.Bus
}

type runtimeWiring struct {
	contextManagerConfig contextstate.Config
	runtime              *contextstate.Runtime
	compactor            contextstate.Compactor
}

type taskEventSink struct {
	observer    task.AgentTaskObserver
	currentText strings.Builder
}

type agentTaskExecutor struct {
	runner  *engine.Engine
	store   *sqlite.Store
	manager *session.Manager
	now     func() time.Time
}

type taskTerminalNotifier struct {
	store     *sqlite.Store
	bus       *stream.Bus
	scheduler *scheduler.Service
	reporting *reporting.Service
	delivery  *automation.DeliveryService
	watcher   automation.DispatchWatcherSyncer
	now       func() time.Time
}

type combinedEventSink struct {
	primary   engine.EventSink
	finalizer engine.TurnFinalizingSink
	observer  engine.EventSink
}

type managerTaskObserver struct {
	manager *task.Manager
	taskID  string
}

type multiTaskObserver struct {
	observers []task.AgentTaskObserver
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

func (a sessionRunnerAdapter) RunTurn(ctx context.Context, in session.RunInput) error {
	sink := engine.EventSink(a.sink)
	var observerSink *taskEventSink
	var dispatchObserver task.AgentTaskObserver
	var taskObserver task.AgentTaskObserver
	var observers []task.AgentTaskObserver
	var finalDispatchText string
	var taskCompleted bool
	dispatchAttempt, hasDispatchAttempt, err := a.dispatchAttemptForRun(ctx, in.Session.ID, in.TurnID)
	if err != nil {
		return err
	}
	if hasDispatchAttempt {
		in.Session.PermissionProfile = firstNonEmptyTrimmed(in.Session.PermissionProfile, "read_only")
	}
	if resumeTaskObserver, ok, err := a.taskObserverForResume(in.Resume); err != nil {
		return err
	} else if ok {
		taskObserver = resumeTaskObserver
		observers = append(observers, taskObserver)
	}
	if currentDispatchObserver, ok, err := a.dispatchObserverForRun(ctx, dispatchAttempt, hasDispatchAttempt); err != nil {
		return err
	} else if ok {
		dispatchObserver = currentDispatchObserver
		observers = append(observers, dispatchObserver)
	}
	if len(observers) > 0 {
		observer := task.AgentTaskObserver(multiTaskObserver{observers: observers})
		observerSink = &taskEventSink{observer: observer}
		sink = combinedEventSink{
			primary:   a.sink,
			finalizer: a.sink,
			observer:  observerSink,
		}
	}

	err = a.engine.RunTurn(ctx, engine.Input{
		Session: in.Session,
		Turn: types.Turn{
			ID:           in.TurnID,
			SessionID:    in.Session.ID,
			ClientTurnID: "",
			UserMessage:  in.Message,
		},
		TaskID:              firstNonEmptyTrimmed(taskIDFromResume(in.Resume)),
		Sink:                sink,
		Resume:              in.Resume,
		ActivatedSkillNames: append([]string(nil), dispatchAttempt.ActivatedSkillNames...),
	})
	if observerSink != nil && len(observers) > 0 && err == nil {
		if turn, ok, turnErr := a.store.GetTurn(ctx, in.TurnID); turnErr != nil {
			return turnErr
		} else if ok && turn.State != types.TurnStateAwaitingPermission {
			taskCompleted = true
			finalDispatchText = strings.TrimSpace(observerSink.FinalText())
		}
	}
	if err != nil && taskObserver != nil {
		if setErr := taskObserver.SetOutcome(types.ChildAgentOutcomeFailure, err.Error()); setErr != nil {
			return errors.Join(err, setErr)
		}
	}
	if taskObserver != nil && taskCompleted {
		if err := taskObserver.SetOutcome(types.ChildAgentOutcomeSuccess, ""); err != nil {
			return err
		}
	}
	if taskObserver != nil && finalDispatchText != "" {
		if err := taskObserver.SetFinalText(finalDispatchText); err != nil {
			return err
		}
	}
	if reconcileErr := a.reconcileDispatchRun(ctx, in.Session.ID, in.TurnID, err); reconcileErr != nil {
		if err != nil {
			return errors.Join(err, reconcileErr)
		}
		return reconcileErr
	}
	if dispatchObserver != nil && finalDispatchText != "" {
		if err := dispatchObserver.SetFinalText(finalDispatchText); err != nil {
			return err
		}
	}
	if taskObserver != nil {
		if notifyErr := a.notifyResumedTaskTerminal(ctx, in.Resume, in.Session.WorkspaceRoot); notifyErr != nil {
			if err != nil {
				return errors.Join(err, notifyErr)
			}
			return notifyErr
		}
	}
	return err
}

func (s *taskEventSink) Emit(_ context.Context, event types.Event) error {
	switch event.Type {
	case types.EventAssistantDelta:
		var payload types.AssistantDeltaPayload
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			return err
		}
		if s.observer != nil {
			if err := s.observer.AppendLog([]byte(payload.Text)); err != nil {
				return err
			}
		}
		s.currentText.WriteString(payload.Text)
		return nil
	case types.EventPermissionRequested:
		var payload types.PermissionRequestedPayload
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			return err
		}
		if s.observer != nil {
			summary := strings.TrimSpace(payload.Reason)
			if summary == "" {
				summary = "approval required"
			}
			if err := s.observer.SetOutcome(types.ChildAgentOutcomeBlocked, summary); err != nil {
				return err
			}
		}
		return nil
	case types.EventToolStarted:
		s.currentText.Reset()
		return nil
	case types.EventTurnFailed:
		var payload types.TurnFailedPayload
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			return err
		}
		if s.observer != nil {
			if err := s.observer.SetOutcome(types.ChildAgentOutcomeFailure, payload.Message); err != nil {
				return err
			}
		}
		if payload.Message == "" {
			return errors.New("turn failed")
		}
		return errors.New(payload.Message)
	default:
		return nil
	}
}

func (s *taskEventSink) FinalText() string {
	if s == nil {
		return ""
	}
	return s.currentText.String()
}

func (o managerTaskObserver) AppendLog(chunk []byte) error {
	if o.manager == nil || strings.TrimSpace(o.taskID) == "" {
		return nil
	}
	return o.manager.Append(o.taskID, chunk)
}

func (o managerTaskObserver) SetFinalText(text string) error {
	if o.manager == nil || strings.TrimSpace(o.taskID) == "" {
		return nil
	}
	return o.manager.SetFinalText(o.taskID, text)
}

func (o managerTaskObserver) SetOutcome(outcome types.ChildAgentOutcome, summary string) error {
	if o.manager == nil || strings.TrimSpace(o.taskID) == "" {
		return nil
	}
	return o.manager.SetOutcome(o.taskID, outcome, summary)
}

func (managerTaskObserver) SetRunContext(_, _ string) error {
	return nil
}

func (o multiTaskObserver) AppendLog(chunk []byte) error {
	for _, observer := range o.observers {
		if observer == nil {
			continue
		}
		if err := observer.AppendLog(chunk); err != nil {
			return err
		}
	}
	return nil
}

func (o multiTaskObserver) SetFinalText(text string) error {
	for _, observer := range o.observers {
		if observer == nil {
			continue
		}
		if err := observer.SetFinalText(text); err != nil {
			return err
		}
	}
	return nil
}

func (o multiTaskObserver) SetOutcome(outcome types.ChildAgentOutcome, summary string) error {
	for _, observer := range o.observers {
		if observer == nil {
			continue
		}
		if err := observer.SetOutcome(outcome, summary); err != nil {
			return err
		}
	}
	return nil
}

func (o multiTaskObserver) SetRunContext(sessionID, turnID string) error {
	for _, observer := range o.observers {
		if observer == nil {
			continue
		}
		if err := observer.SetRunContext(sessionID, turnID); err != nil {
			return err
		}
	}
	return nil
}

func buildAgentTaskExecutor(runner *engine.Engine, stores ...*sqlite.Store) *agentTaskExecutor {
	if runner == nil {
		return nil
	}
	var store *sqlite.Store
	if len(stores) > 0 {
		store = stores[0]
	}
	return &agentTaskExecutor{
		runner: runner,
		store:  store,
	}
}

func buildTaskTerminalNotifier(store *sqlite.Store, bus *stream.Bus, workspaceRoot string) *taskTerminalNotifier {
	if store == nil || bus == nil {
		return nil
	}
	reportingService := reporting.NewService(store)
	reportingService.SetWorkspaceRoot(workspaceRoot)
	reportingService.SetReportReadySink(func(ctx context.Context, sessionID, turnID string, item types.ReportMailboxItem) error {
		// Transport compatibility shim: report-ready notifications are still emitted to the currently
		// selected session, but durable mailbox persistence is workspace-scoped.
		selected, ok, err := store.GetSelectedSessionID(ctx)
		if err != nil || !ok || strings.TrimSpace(selected) == "" {
			return nil
		}
		sessionID = selected
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

func (s combinedEventSink) Emit(ctx context.Context, event types.Event) error {
	if s.primary != nil {
		if err := s.primary.Emit(ctx, event); err != nil {
			return err
		}
	}
	if s.observer != nil {
		if err := s.observer.Emit(ctx, event); err != nil {
			return err
		}
	}
	return nil
}

func (s combinedEventSink) FinalizeTurn(ctx context.Context, usage *types.TurnUsage, events []types.Event) error {
	if s.finalizer == nil {
		return nil
	}
	return s.finalizer.FinalizeTurn(ctx, usage, events)
}

func (a sessionRunnerAdapter) dispatchAttemptForRun(ctx context.Context, sessionID, turnID string) (types.DispatchAttempt, bool, error) {
	if a.store == nil {
		return types.DispatchAttempt{}, false, nil
	}
	return a.store.FindDispatchAttemptByBackgroundRun(ctx, sessionID, turnID)
}

func (a sessionRunnerAdapter) dispatchObserverForRun(ctx context.Context, attempt types.DispatchAttempt, ok bool) (task.AgentTaskObserver, bool, error) {
	if a.store == nil {
		return nil, false, nil
	}
	if !ok {
		return nil, false, nil
	}
	return automation.NewDispatchTaskObserverWithDelivery(a.store, a.delivery, attempt.DispatchID, a.currentTime), true, nil
}

func (a sessionRunnerAdapter) taskObserverForResume(resume *types.TurnResume) (task.AgentTaskObserver, bool, error) {
	taskID := taskIDFromResume(resume)
	if a.tasker == nil || taskID == "" {
		return nil, false, nil
	}
	return managerTaskObserver{
		manager: a.tasker,
		taskID:  taskID,
	}, true, nil
}

func (a sessionRunnerAdapter) notifyResumedTaskTerminal(ctx context.Context, resume *types.TurnResume, workspaceRoot string) error {
	taskID := taskIDFromResume(resume)
	if a.tasker == nil || a.notifier == nil || taskID == "" {
		return nil
	}
	completed, ok, err := a.tasker.Get(taskID, workspaceRoot)
	if err != nil || !ok {
		return err
	}
	return a.notifier.NotifyTaskTerminal(ctx, completed)
}

func taskIDFromResume(resume *types.TurnResume) string {
	if resume == nil {
		return ""
	}
	return strings.TrimSpace(resume.TaskID)
}

func (a sessionRunnerAdapter) reconcileDispatchRun(ctx context.Context, sessionID, turnID string, runErr error) error {
	if a.store == nil {
		return nil
	}
	attempt, ok, err := a.store.FindDispatchAttemptByBackgroundRun(ctx, sessionID, turnID)
	if err != nil || !ok {
		return err
	}
	now := a.currentTime()
	if runErr != nil {
		attempt.Status = types.DispatchAttemptStatusFailed
		attempt.Error = strings.TrimSpace(runErr.Error())
		attempt.FinishedAt = now
		attempt.UpdatedAt = now
		if err := a.store.UpsertDispatchAttempt(ctx, attempt); err != nil {
			return err
		}
		if err := a.releaseWatcherHoldForDispatchOutcome(ctx, attempt, false); err != nil {
			return err
		}
		return a.applyDispatchOutcome(ctx, attempt, dispatchOutcomeFailed, now)
	}

	turn, ok, err := a.store.GetTurn(ctx, turnID)
	if err != nil || !ok {
		return err
	}
	if turn.State != types.TurnStateAwaitingPermission {
		attempt.Status = types.DispatchAttemptStatusCompleted
		attempt.Error = ""
		if attempt.FinishedAt.IsZero() {
			attempt.FinishedAt = now
		}
		attempt.UpdatedAt = now
		if err := a.store.UpsertDispatchAttempt(ctx, attempt); err != nil {
			return err
		}
		if err := a.releaseWatcherHoldForDispatchOutcome(ctx, attempt, true); err != nil {
			return err
		}
		return a.applyDispatchOutcome(ctx, attempt, dispatchOutcomeCompleted, now)
	}

	requests, err := a.store.ListPermissionRequestsBySession(ctx, sessionID)
	if err != nil {
		return err
	}
	var request types.PermissionRequest
	foundRequest := false
	for _, candidate := range requests {
		if candidate.TurnID != turnID || candidate.Status != types.PermissionRequestStatusRequested {
			continue
		}
		if !foundRequest || candidate.CreatedAt.After(request.CreatedAt) {
			request = candidate
			foundRequest = true
		}
	}
	if !foundRequest {
		return nil
	}
	if err := automation.ReplaceDispatchHoldWithApprovalHold(ctx, a.store, attempt.AutomationID, attempt.DispatchID, request.ID, now); err != nil {
		return err
	}

	continuation, ok, err := a.store.GetTurnContinuationByPermissionRequest(ctx, request.ID)
	if err != nil || !ok {
		return err
	}
	attempt.Status = types.DispatchAttemptStatusAwaitingApproval
	attempt.PermissionRequestID = request.ID
	attempt.ContinuationID = continuation.ID
	attempt.BackgroundSessionID = sessionID
	attempt.BackgroundTurnID = turnID
	attempt.UpdatedAt = now
	if err := a.store.UpsertDispatchAttempt(ctx, attempt); err != nil {
		return err
	}
	if phases, err := a.store.ListIncidentPhaseStates(ctx, attempt.IncidentID); err == nil {
		for _, phase := range phases {
			if phase.Phase != attempt.Phase {
				continue
			}
			phase.Status = types.IncidentPhaseStatusAwaitingApproval
			phase.UpdatedAt = now
			if err := a.store.UpsertIncidentPhaseState(ctx, phase); err != nil {
				return err
			}
			break
		}
	}
	if incident, ok, err := a.store.GetAutomationIncident(ctx, attempt.IncidentID); err == nil && ok {
		incident.Status = types.AutomationIncidentStatusActive
		incident.UpdatedAt = now
		return a.store.UpsertAutomationIncident(ctx, incident)
	} else if err != nil {
		return err
	}
	return nil
}

func (a sessionRunnerAdapter) releaseWatcherHoldForDispatchOutcome(ctx context.Context, attempt types.DispatchAttempt, succeeded bool) error {
	if a.store == nil {
		return nil
	}
	spec, ok, err := a.store.GetAutomation(ctx, attempt.AutomationID)
	if err != nil || !ok {
		return err
	}
	bundle, err := automation.LoadChildAgentRuntimeBundle(spec.WorkspaceRoot, spec.ID, attempt.Phase, attempt.ChildAgentID)
	if err != nil {
		return err
	}

	kind, ownerID, err := a.activeWatcherHoldForAttempt(ctx, attempt)
	if err != nil {
		return err
	}

	if succeeded {
		if bundle.Strategy.CompletionPolicy.ResumeWatcherOnSuccess == nil || !*bundle.Strategy.CompletionPolicy.ResumeWatcherOnSuccess {
			return nil
		}
		if err := automation.ReleaseWatcherHoldByOwner(ctx, a.store, attempt.AutomationID, kind, ownerID); err != nil {
			return err
		}
		return a.syncWatcher(ctx, attempt.AutomationID)
	}
	if bundle.Strategy.CompletionPolicy.ResumeWatcherOnFailure == nil || !*bundle.Strategy.CompletionPolicy.ResumeWatcherOnFailure {
		return nil
	}
	if err := automation.ReleaseWatcherHoldByOwner(ctx, a.store, attempt.AutomationID, kind, ownerID); err != nil {
		return err
	}
	return a.syncWatcher(ctx, attempt.AutomationID)
}

func (a sessionRunnerAdapter) activeWatcherHoldForAttempt(ctx context.Context, attempt types.DispatchAttempt) (types.AutomationWatcherHoldKind, string, error) {
	if a.store == nil {
		return types.AutomationWatcherHoldKindDispatch, attempt.DispatchID, nil
	}
	holds, err := a.store.ListAutomationWatcherHolds(ctx, attempt.AutomationID)
	if err != nil {
		return "", "", err
	}
	requestID := strings.TrimSpace(attempt.PermissionRequestID)
	if requestID != "" {
		for _, hold := range holds {
			if hold.Kind == types.AutomationWatcherHoldKindApproval && strings.TrimSpace(hold.OwnerID) == requestID {
				return types.AutomationWatcherHoldKindApproval, requestID, nil
			}
		}
	}
	return types.AutomationWatcherHoldKindDispatch, attempt.DispatchID, nil
}

func (a sessionRunnerAdapter) syncWatcher(ctx context.Context, automationID string) error {
	if a.watcher == nil {
		return nil
	}
	return a.watcher.SyncAutomation(ctx, automationID)
}

type dispatchOutcome string

const (
	dispatchOutcomeCompleted dispatchOutcome = "completed"
	dispatchOutcomeFailed    dispatchOutcome = "failed"
)

func (a sessionRunnerAdapter) applyDispatchOutcome(ctx context.Context, attempt types.DispatchAttempt, outcome dispatchOutcome, now time.Time) error {
	phaseStatus, incidentStatus, err := a.resolveDispatchOutcome(ctx, attempt, outcome)
	if err != nil {
		return err
	}
	if err := updateIncidentPhaseState(ctx, a.store, attempt.IncidentID, attempt.Phase, now, func(phase *types.IncidentPhaseState) {
		if phase.ActiveDispatchCount > 0 {
			phase.ActiveDispatchCount--
		}
		phase.Status = phaseStatus
		switch outcome {
		case dispatchOutcomeCompleted:
			phase.CompletedDispatchCount++
		case dispatchOutcomeFailed:
			phase.FailedDispatchCount++
		}
	}); err != nil {
		return err
	}
	return updateAutomationIncidentStatus(ctx, a.store, attempt.IncidentID, incidentStatus, now)
}

func (a sessionRunnerAdapter) resolveDispatchOutcome(ctx context.Context, attempt types.DispatchAttempt, outcome dispatchOutcome) (types.IncidentPhaseStatus, types.AutomationIncidentStatus, error) {
	action := types.AutomationPhaseTransitionComplete
	phaseStatus := types.IncidentPhaseStatusCompleted
	if outcome == dispatchOutcomeFailed {
		action = types.AutomationPhaseTransitionEscalate
		phaseStatus = types.IncidentPhaseStatusFailed
	}

	spec, ok, err := a.store.GetAutomation(ctx, attempt.AutomationID)
	if err != nil {
		return "", "", err
	}
	if ok {
		if planPhase, found := findResponsePlanPhase(spec.ResponsePlan, attempt.Phase); found {
			switch outcome {
			case dispatchOutcomeCompleted:
				if planPhase.OnSuccess != "" {
					action = planPhase.OnSuccess
				}
			case dispatchOutcomeFailed:
				if planPhase.OnFailure != "" {
					action = planPhase.OnFailure
				}
			}
		}
	}

	return phaseStatus, incidentStatusForPhaseTransition(action), nil
}

func findResponsePlanPhase(raw json.RawMessage, phaseName types.AutomationPhaseName) (types.AutomationPhasePlan, bool) {
	normalized := types.NormalizeAutomationResponsePlanJSON(raw)
	if len(normalized) == 0 {
		return types.AutomationPhasePlan{}, false
	}

	var plan types.ResponsePlanV2
	if err := json.Unmarshal(normalized, &plan); err != nil {
		return types.AutomationPhasePlan{}, false
	}
	for _, phase := range plan.Phases {
		if phase.Phase == phaseName {
			return phase, true
		}
	}
	return types.AutomationPhasePlan{}, false
}

func incidentStatusForPhaseTransition(action types.AutomationPhaseTransitionAction) types.AutomationIncidentStatus {
	switch action {
	case types.AutomationPhaseTransitionNextPhase:
		return types.AutomationIncidentStatusQueued
	case types.AutomationPhaseTransitionEscalate:
		return types.AutomationIncidentStatusEscalated
	case types.AutomationPhaseTransitionCancel:
		return types.AutomationIncidentStatusCanceled
	case types.AutomationPhaseTransitionComplete:
		fallthrough
	default:
		return types.AutomationIncidentStatusResolved
	}
}

func updateIncidentPhaseState(ctx context.Context, store *sqlite.Store, incidentID string, phaseName types.AutomationPhaseName, now time.Time, apply func(*types.IncidentPhaseState)) error {
	if store == nil || strings.TrimSpace(incidentID) == "" {
		return nil
	}
	phases, err := store.ListIncidentPhaseStates(ctx, incidentID)
	if err != nil {
		return err
	}
	for _, phase := range phases {
		if phase.Phase != phaseName {
			continue
		}
		apply(&phase)
		phase.UpdatedAt = now
		return store.UpsertIncidentPhaseState(ctx, phase)
	}
	return nil
}

func updateAutomationIncidentStatus(ctx context.Context, store *sqlite.Store, incidentID string, status types.AutomationIncidentStatus, now time.Time) error {
	if store == nil || strings.TrimSpace(incidentID) == "" {
		return nil
	}
	incident, ok, err := store.GetAutomationIncident(ctx, incidentID)
	if err != nil || !ok {
		return err
	}
	incident.Status = status
	incident.UpdatedAt = now
	return store.UpsertAutomationIncident(ctx, incident)
}

func (a sessionRunnerAdapter) currentTime() time.Time {
	if a.now != nil {
		return a.now().UTC()
	}
	return time.Now().UTC()
}

func mustParseDetectorSignalForPrompt(incident types.AutomationIncident) types.AutomationDetectorSignal {
	detectorSignal, err := automation.ParseAutomationDetectorSignalPayload(incident.Payload)
	if err != nil {
		return types.AutomationDetectorSignal{
			Summary: strings.TrimSpace(incident.Summary),
			Facts:   map[string]any{},
		}
	}
	return detectorSignal
}

func (a agentTaskExecutor) RunTask(ctx context.Context, taskID string, workspaceRoot string, prompt string, activatedSkillNames []string, observer task.AgentTaskObserver) error {
	if a.runner == nil {
		return errors.New("engine runner is not configured")
	}

	sessionID := types.NewID("task_session")
	turnID := types.NewID("task_turn")
	if err := a.prepareTaskRun(ctx, sessionID, turnID, workspaceRoot, prompt); err != nil {
		return err
	}
	sink := &taskEventSink{observer: observer}
	if observer != nil {
		if err := observer.SetRunContext(sessionID, turnID); err != nil {
			return err
		}
	}
	if err := a.runner.RunTurn(ctx, engine.Input{
		Session: types.Session{
			ID:            sessionID,
			WorkspaceRoot: workspaceRoot,
		},
		Turn: types.Turn{
			ID:          turnID,
			SessionID:   sessionID,
			UserMessage: prompt,
		},
		TaskID:              strings.TrimSpace(taskID),
		Sink:                sink,
		ActivatedSkillNames: append([]string(nil), activatedSkillNames...),
	}); err != nil {
		return err
	}
	if observer == nil {
		return nil
	}
	finalText := sink.FinalText()
	if strings.TrimSpace(finalText) == "" {
		return nil
	}
	return observer.SetFinalText(finalText)
}

func (a agentTaskExecutor) prepareTaskRun(ctx context.Context, sessionID, turnID, workspaceRoot, prompt string) error {
	if a.store == nil {
		return nil
	}
	now := a.currentTime()
	sessionRow := types.Session{
		ID:                sessionID,
		WorkspaceRoot:     strings.TrimSpace(workspaceRoot),
		PermissionProfile: "read_only",
		State:             types.SessionStateIdle,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	if existing, ok, err := a.store.GetSession(ctx, sessionRow.ID); err != nil {
		return err
	} else if ok {
		sessionRow = existing
	} else if err := a.store.InsertSession(ctx, sessionRow); err != nil {
		return err
	}
	turnRow := types.Turn{
		ID:          turnID,
		SessionID:   sessionRow.ID,
		State:       types.TurnStateCreated,
		UserMessage: prompt,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if _, ok, err := a.store.GetTurn(ctx, turnRow.ID); err != nil {
		return err
	} else if !ok {
		if err := a.store.InsertTurn(ctx, turnRow); err != nil {
			return err
		}
	}
	if a.manager != nil {
		a.manager.RegisterSession(sessionRow)
	}
	return nil
}

func (a agentTaskExecutor) currentTime() time.Time {
	if a.now != nil {
		return a.now().UTC()
	}
	return time.Now().UTC()
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

	if strings.TrimSpace(completed.ParentSessionID) == "" {
		return nil
	}

	updatedBlock := timelineBlockFromCompletedTask(completed, runtimeTask, ok)
	eventSink := storeAndBusSink{store: n.store, bus: n.bus}
	taskEvent, err := types.NewEvent(completed.ParentSessionID, completed.ParentTurnID, types.EventTaskUpdated, updatedBlock)
	if err != nil {
		return err
	}
	if err := eventSink.Emit(ctx, taskEvent); err != nil {
		return err
	}

	if !shouldNotifyTaskResultReady(completed) {
		return nil
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
			reportEvent, err := types.NewEvent(completed.ParentSessionID, completed.ParentTurnID, types.EventReportReady, reportItem)
			if err != nil {
				return err
			}
			if err := eventSink.Emit(ctx, reportEvent); err != nil {
				return err
			}
		}
		return nil
	}
	completion, readyBlock, ok := pendingCompletionFromTask(completed, updatedBlock, now)
	if !ok {
		return nil
	}
	if err := n.store.UpsertPendingTaskCompletion(ctx, completion); err != nil {
		return err
	}
	readyEvent, err := types.NewEvent(completed.ParentSessionID, completed.ParentTurnID, types.EventTaskResultReady, readyBlock)
	if err != nil {
		return err
	}
	if err := eventSink.Emit(ctx, readyEvent); err != nil {
		return err
	}
	return nil
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

func pendingCompletionFromTask(completed task.Task, block types.TimelineBlock, now time.Time) (types.PendingTaskCompletion, types.TimelineBlock, bool) {
	result, ready := completed.FinalResult()
	if !ready {
		return types.PendingTaskCompletion{}, types.TimelineBlock{}, false
	}
	completion := types.PendingTaskCompletion{
		ID:            completed.ID,
		SessionID:     completed.ParentSessionID,
		ParentTurnID:  completed.ParentTurnID,
		TaskID:        completed.ID,
		TaskType:      string(completed.Type),
		Command:       completed.Command,
		Description:   completed.Description,
		ResultKind:    string(result.Kind),
		ResultText:    result.Text,
		ResultPreview: clampTaskResultPreview(result.Text),
		ObservedAt:    result.ObservedAt,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	readyBlock := block
	readyBlock.Status = string(runtimeTaskStateFromTaskStatus(completed.Status))
	readyBlock.ResultPreview = completion.ResultPreview
	return completion, readyBlock, true
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

func shouldNotifyTaskResultReady(completed task.Task) bool {
	return completed.Status == task.TaskStatusCompleted &&
		completed.ResultReady() &&
		strings.TrimSpace(completed.ParentSessionID) != "" &&
		completed.CompletionNotifiedAt == nil
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

func ensureDataDir(path string) error {
	return os.MkdirAll(path, 0o755)
}

func writePIDFile(path string, daemonID string, fingerprint string) error {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	raw, err := json.Marshal(map[string]any{
		"pid":                os.Getpid(),
		"daemon_id":          strings.TrimSpace(daemonID),
		"config_fingerprint": strings.TrimSpace(fingerprint),
	})
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0o644)
}

func Run(ctx context.Context) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if missing := config.MissingSetupFields(cfg); len(missing) > 0 {
		configPath, _ := config.GlobalConfigPath()
		return fmt.Errorf("sesame daemon is not configured: missing %s in %s", strings.Join(missing, ", "), configPath)
	}

	basePrompt, err := cfg.ResolveSystemPrompt()
	if err != nil {
		return err
	}

	if err := ensureDataDir(cfg.DataDir); err != nil {
		return err
	}
	if err := writePIDFile(cfg.Paths.PIDFile, cfg.DaemonID, cfg.ConfigFingerprint); err != nil {
		return err
	}
	defer os.Remove(cfg.Paths.PIDFile)

	store, err := sqlite.Open(cfg.Paths.DatabaseFile)
	if err != nil {
		return err
	}
	defer store.Close()

	_, err = artifacts.New(filepath.Join(cfg.DataDir, "artifacts"))
	if err != nil {
		return err
	}

	configureRuntimeGuardrails(cfg)
	modelClient, err := model.NewFromConfig(cfg)
	if err != nil {
		return err
	}
	runtime, err := buildRuntime(ctx, cfg, store, modelClient)
	if err != nil {
		return err
	}
	if err := validateRuntime(runtime); err != nil {
		return err
	}
	if runtime.Engine != nil {
		runtime.Engine.SetBaseSystemPrompt(basePrompt)
	}

	if err := recoverRuntimeState(ctx, runtime.Store, runtime.SessionManager); err != nil {
		return err
	}
	dispatcher := automation.NewDispatcher(runtime.Store, automationTaskLauncher{
		store:   runtime.Store,
		manager: runtime.TaskManager,
	}, automation.DispatcherConfig{Watcher: runtime.WatcherService})
	go func() {
		if err := runtime.SchedulerService.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
			slog.Error("scheduler loop exited", "error", err)
		}
	}()
	if runtime.WatcherService != nil {
		go func() {
			runSupervisedLoop(ctx, "watcher", runtime.WatcherService.ReconcileInterval(), runtime.WatcherService.Reconcile, func(_ context.Context, err error) {
				slog.Error("watcher tick failed", "error", err)
			})
		}()
	}
	if runtime.ReportingService != nil {
		go func() {
			runSupervisedLoop(ctx, "reporting", runtime.ReportingService.PollInterval(), runtime.ReportingService.Tick, func(_ context.Context, err error) {
				slog.Error("reporting tick failed", "error", err)
			})
		}()
	}
	go func() {
		runSupervisedLoop(ctx, "automation_dispatcher", time.Second, dispatcher.Tick, func(_ context.Context, err error) {
			slog.Error("automation dispatcher tick failed", "error", err)
		})
	}()

	handler := httpapi.NewRouter(buildHTTPDependencies(cfg, runtime.Store, runtime.Bus, runtime.SessionManager, runtime.SchedulerService, runtime.AutomationService))

	slog.Info("sesame daemon listening", "addr", cfg.Addr)
	return http.ListenAndServe(cfg.Addr, handler)
}

func configureRuntimeGuardrails(cfg config.Config) {
	tools.SetShellCommandGuardrails(cfg.MaxShellOutputBytes, cfg.ShellTimeoutSeconds)
	tools.SetFileWriteMaxBytes(cfg.MaxFileWriteBytes)
}

func buildHTTPDependencies(cfg config.Config, store *sqlite.Store, bus *stream.Bus, manager *session.Manager, schedulerService *scheduler.Service, automationService *automation.Service) httpapi.Dependencies {
	if automationService == nil && store != nil {
		automationService = automation.NewService(store)
	}
	return httpapi.Dependencies{
		Bus:           bus,
		Store:         store,
		Manager:       manager,
		Scheduler:     schedulerService,
		Automation:    automationService,
		Status:        buildStatusPayload(cfg),
		ConsoleRoot:   filepath.Join("web", "console", "dist"),
		WorkspaceRoot: cfg.Paths.WorkspaceRoot,
	}
}

func buildPermissionEngine(cfg config.Config) *permissions.Engine {
	return permissions.NewEngine(cfg.PermissionProfile)
}

func buildContextManagerConfig(cfg config.Config) contextstate.Config {
	return contextstate.Config{
		MaxRecentItems:             cfg.MaxRecentItems,
		MaxEstimatedTokens:         cfg.MaxEstimatedTokens,
		CompactionThreshold:        cfg.CompactionThreshold,
		MicrocompactBytesThreshold: cfg.MicrocompactBytesThreshold,
	}
}

func buildMaxToolSteps(cfg config.Config) int {
	return cfg.MaxToolSteps
}

func buildStatusPayload(cfg config.Config) httpapi.StatusPayload {
	return httpapi.StatusPayload{
		DaemonID:             cfg.DaemonID,
		Provider:             cfg.ModelProvider,
		Model:                cfg.Model,
		PermissionProfile:    cfg.PermissionProfile,
		ProviderCacheProfile: cfg.ProviderCacheProfile,
		ConfigFingerprint:    cfg.ConfigFingerprint,
		PID:                  os.Getpid(),
	}
}

func buildRuntimeWiring(cfg config.Config, modelClient model.StreamingClient) runtimeWiring {
	return runtimeWiring{
		contextManagerConfig: buildContextManagerConfig(cfg),
		runtime:              contextstate.NewRuntime(cfg.CacheExpirySeconds, cfg.MaxCompactionPasses),
		compactor:            contextstate.NewPromptedCompactor(modelClient, cfg.Model),
	}
}

func runAutomationDispatcherLoop(ctx context.Context, dispatcher *automation.Dispatcher) error {
	if dispatcher == nil {
		return nil
	}
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	if err := dispatcher.Tick(ctx); err != nil {
		return err
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := dispatcher.Tick(ctx); err != nil {
				return err
			}
		}
	}
}

func recoverRuntimeState(ctx context.Context, store *sqlite.Store, manager *session.Manager) error {
	sessions, err := store.ListSessions(ctx)
	if err != nil {
		return err
	}
	if manager != nil {
		for _, sessionRow := range sessions {
			manager.RegisterSession(sessionRow)
		}
	}
	if err := ensureSelectedSession(ctx, store, sessions); err != nil {
		return err
	}
	resumedTurns, err := resumeResolvedContinuations(ctx, store, manager)
	if err != nil {
		return err
	}

	running, err := store.ListRunningTurns(ctx)
	if err != nil {
		return err
	}

	for _, turn := range running {
		if _, ok := resumedTurns[turn.ID]; ok {
			continue
		}
		if turn.State == types.TurnStateAwaitingPermission {
			continue
		}
		if err := store.MarkTurnInterrupted(ctx, turn.ID); err != nil {
			return err
		}

		event, err := types.NewEvent(turn.SessionID, turn.ID, types.EventTurnInterrupted, map[string]string{
			"reason": "daemon_restart",
		})
		if err != nil {
			return err
		}
		if _, err := store.AppendEvent(ctx, event); err != nil {
			return err
		}
	}

	attempts, err := store.ListDispatchAttempts(ctx, types.DispatchAttemptFilter{
		Status: types.DispatchAttemptStatusRunning,
	})
	if err != nil {
		return err
	}
	for _, attempt := range attempts {
		attempt.Status = types.DispatchAttemptStatusInterrupted
		attempt.Error = firstNonEmptyTrimmed(attempt.Error, "daemon_restart")
		attempt.UpdatedAt = time.Now().UTC()
		if err := store.UpsertDispatchAttempt(ctx, attempt); err != nil {
			return err
		}
		if err := updateIncidentPhaseState(ctx, store, attempt.IncidentID, attempt.Phase, attempt.UpdatedAt, func(phase *types.IncidentPhaseState) {
			if phase.ActiveDispatchCount > 0 {
				phase.ActiveDispatchCount--
			}
			phase.Status = types.IncidentPhaseStatusPending
		}); err != nil {
			return err
		}
		if err := updateAutomationIncidentStatus(ctx, store, attempt.IncidentID, types.AutomationIncidentStatusQueued, attempt.UpdatedAt); err != nil {
			return err
		}
	}

	return nil
}

func resumeResolvedContinuations(ctx context.Context, store *sqlite.Store, manager *session.Manager) (map[string]struct{}, error) {
	resumed := make(map[string]struct{})
	if store == nil || manager == nil {
		return resumed, nil
	}

	continuations, err := store.ListPendingTurnContinuations(ctx)
	if err != nil {
		return nil, err
	}

	for _, continuation := range continuations {
		if strings.TrimSpace(continuation.PermissionRequestID) == "" {
			continue
		}
		request, ok, err := store.GetPermissionRequest(ctx, continuation.PermissionRequestID)
		if err != nil {
			return nil, err
		}
		if !ok || request.Status == types.PermissionRequestStatusRequested || strings.TrimSpace(request.Decision) == "" {
			continue
		}

		turn, ok, err := store.GetTurn(ctx, continuation.TurnID)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		sessionRow, ok, err := store.GetSession(ctx, continuation.SessionID)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}

		effectiveProfile := sessionRow.PermissionProfile
		if types.PermissionDecisionGrantsProfile(request.Decision) && strings.TrimSpace(request.RequestedProfile) != "" {
			effectiveProfile = request.RequestedProfile
		}
		decisionScope := strings.TrimSpace(request.DecisionScope)
		if decisionScope == "" {
			decisionScope = request.Decision
		}
		resume := &types.TurnResume{
			ContinuationID:             continuation.ID,
			PermissionRequestID:        request.ID,
			ToolRunID:                  continuation.ToolRunID,
			ToolCallID:                 continuation.ToolCallID,
			ToolName:                   continuation.ToolName,
			RequestedProfile:           continuation.RequestedProfile,
			Reason:                     continuation.Reason,
			Decision:                   request.Decision,
			DecisionScope:              decisionScope,
			EffectivePermissionProfile: effectiveProfile,
			RunID:                      continuation.RunID,
			TaskID:                     continuation.TaskID,
		}
		if _, err := manager.ResumeTurn(ctx, sessionRow.ID, session.ResumeTurnInput{
			TurnID:  turn.ID,
			Message: turn.UserMessage,
			Resume:  resume,
		}); err != nil {
			return nil, err
		}

		now := time.Now().UTC()
		continuation.State = types.TurnContinuationStateResumed
		continuation.Decision = request.Decision
		continuation.DecisionScope = decisionScope
		continuation.UpdatedAt = now
		var resumedToolRun *types.ToolRun
		if strings.TrimSpace(continuation.ToolRunID) != "" {
			toolRun, found, err := store.GetToolRun(ctx, continuation.ToolRunID)
			if err != nil {
				return nil, err
			}
			if found {
				toolRun.PermissionRequestID = request.ID
				toolRun.UpdatedAt = now
				toolRun.CompletedAt = now
				toolRun.OutputJSON = marshalRecoveredPermissionToolRunOutput(request, effectiveProfile)
				if request.Decision == types.PermissionDecisionDeny {
					toolRun.State = types.ToolRunStateFailed
					toolRun.Error = "permission denied"
				} else {
					toolRun.State = types.ToolRunStateCompleted
					toolRun.Error = ""
				}
				resumedToolRun = &toolRun
			}
		}
		if err := store.CommitPermissionResume(ctx, sessionRow.ID, turn.ID, continuation, resumedToolRun); err != nil {
			manager.InterruptTurn(sessionRow.ID, turn.ID)
			return nil, err
		}
		if err := automation.RestoreDispatchAfterApprovalResume(ctx, store, sessionRow.ID, turn.ID, continuation.TaskID, request.ID, now); err != nil {
			return nil, err
		}

		resumed[turn.ID] = struct{}{}
	}

	return resumed, nil
}

func marshalRecoveredPermissionToolRunOutput(request types.PermissionRequest, effectiveProfile string) string {
	payload, _ := json.Marshal(map[string]any{
		"status":                       request.Status,
		"decision":                     request.Decision,
		"decision_scope":               request.DecisionScope,
		"requested_profile":            request.RequestedProfile,
		"effective_permission_profile": effectiveProfile,
		"reason":                       request.Reason,
	})
	return string(payload)
}

func ensureSelectedSession(ctx context.Context, store *sqlite.Store, sessions []types.Session) error {
	if len(sessions) == 0 {
		return nil
	}

	selected, ok, err := store.GetSelectedSessionID(ctx)
	if err != nil {
		return err
	}
	if ok {
		for _, sessionRow := range sessions {
			if sessionRow.ID == selected {
				return nil
			}
		}
	}

	return store.SetSelectedSessionID(ctx, sessions[0].ID)
}
