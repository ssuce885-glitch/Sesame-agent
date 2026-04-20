package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"go-agent/internal/automation"
	"go-agent/internal/engine"
	rolectx "go-agent/internal/roles"
	"go-agent/internal/session"
	"go-agent/internal/sessionbinding"
	"go-agent/internal/sessionrole"
	"go-agent/internal/store/sqlite"
	"go-agent/internal/task"
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

type taskEventSink struct {
	observer    task.AgentTaskObserver
	currentText strings.Builder
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

func (a sessionRunnerAdapter) RunTurn(ctx context.Context, in session.RunInput) error {
	sink := engine.EventSink(a.sink)
	var observerSink *taskEventSink
	var dispatchObserver task.AgentTaskObserver
	var taskObserver task.AgentTaskObserver
	var observers []task.AgentTaskObserver
	var finalDispatchText string
	var taskCompleted bool

	dispatchAttempt, hasDispatchAttempt, err := a.dispatchAttemptForTask(ctx, taskIDFromResume(in.Resume))
	if err != nil {
		return err
	}
	if hasDispatchAttempt {
		in.Session.PermissionProfile = firstNonEmptyTrimmed(in.Session.PermissionProfile, "read_only")
	}
	role, err := a.store.ResolveSessionRole(ctx, in.Session.ID, in.Session.WorkspaceRoot)
	if err != nil {
		return err
	}
	specialistRoleID := rolectx.SpecialistRoleIDFromContext(ctx)
	if specialistRoleID == "" {
		resolvedSpecialistRoleID, err := a.store.ResolveSpecialistRoleID(ctx, in.Session.ID, in.Session.WorkspaceRoot)
		if err != nil {
			return err
		}
		specialistRoleID = resolvedSpecialistRoleID
	}
	runCtx := withRunnerSessionContext(ctx, in.Session, role, specialistRoleID)
	if in.Turn.ContextHeadID == "" && a.store != nil {
		headID, ok, err := a.store.GetCurrentContextHeadID(runCtx)
		if err != nil {
			return err
		}
		if ok {
			in.Turn.ContextHeadID = strings.TrimSpace(headID)
		}
	}
	if resumeTaskObserver, ok, err := a.taskObserverForResume(in.Resume); err != nil {
		return err
	} else if ok {
		taskObserver = resumeTaskObserver
		observers = append(observers, taskObserver)
	}
	if taskObserver == nil {
		if currentDispatchObserver, ok, err := a.dispatchObserverForRun(ctx, dispatchAttempt, hasDispatchAttempt); err != nil {
			return err
		} else if ok {
			dispatchObserver = currentDispatchObserver
			observers = append(observers, dispatchObserver)
		}
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

	err = a.engine.RunTurn(runCtx, engine.Input{
		Session:             in.Session,
		SessionRole:         sessionrole.Normalize(string(role)),
		Turn:                in.Turn,
		TaskID:              firstNonEmptyTrimmed(taskIDFromResume(in.Resume)),
		Sink:                sink,
		Resume:              in.Resume,
		ActivatedSkillNames: append([]string(nil), dispatchAttempt.ActivatedSkillNames...),
	})
	if observerSink != nil && len(observers) > 0 && err == nil {
		finalDispatchText = strings.TrimSpace(observerSink.FinalText())
		if in.Resume != nil && finalDispatchText != "" {
			taskCompleted = true
		} else if turn, ok, turnErr := a.store.GetTurn(runCtx, in.Turn.ID); turnErr != nil {
			return turnErr
		} else if ok && turn.State != types.TurnStateAwaitingPermission {
			taskCompleted = true
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
	if dispatchObserver != nil && finalDispatchText != "" {
		if err := dispatchObserver.SetFinalText(finalDispatchText); err != nil {
			return err
		}
	}
	if taskObserver != nil {
		if notifyErr := a.notifyResumedTaskTerminal(runCtx, in.Resume, in.Session.WorkspaceRoot); notifyErr != nil {
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

func withRunnerSessionContext(ctx context.Context, sessionRow types.Session, role types.SessionRole, specialistRoleID string) context.Context {
	ctx = rolectx.WithSpecialistRoleID(sessionrole.WithSessionRole(ctx, role), specialistRoleID)
	if strings.HasPrefix(strings.TrimSpace(sessionRow.ID), "task_session_") {
		return sessionbinding.WithContextBinding(ctx, taskContextBinding(sessionRow.ID))
	}
	return ctx
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

func (a sessionRunnerAdapter) dispatchAttemptForTask(ctx context.Context, taskID string) (types.DispatchAttempt, bool, error) {
	if a.store == nil || strings.TrimSpace(taskID) == "" {
		return types.DispatchAttempt{}, false, nil
	}
	return a.store.FindDispatchAttemptByTaskID(ctx, taskID)
}

func (a sessionRunnerAdapter) dispatchObserverForRun(_ context.Context, attempt types.DispatchAttempt, ok bool) (task.AgentTaskObserver, bool, error) {
	if a.store == nil || !ok {
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
