package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"go-agent/internal/engine"
	rolectx "go-agent/internal/roles"
	"go-agent/internal/session"
	"go-agent/internal/sessionbinding"
	"go-agent/internal/sessionrole"
	"go-agent/internal/store/sqlite"
	"go-agent/internal/task"
	"go-agent/internal/types"
	"go-agent/internal/workspace"
)

type sessionRunnerAdapter struct {
	engine   *engine.Engine
	sink     storeAndBusSink
	store    *sqlite.Store
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
	var taskObserver task.AgentTaskObserver
	var observers []task.AgentTaskObserver
	var finalObserverText string
	var taskCompleted bool
	role, err := a.store.ResolveSessionRole(ctx, in.Session.ID, in.Session.WorkspaceRoot)
	if err != nil {
		return err
	}
	resolvedSpecialistRoleID, err := a.store.ResolveSpecialistRoleID(ctx, in.Session.ID, in.Session.WorkspaceRoot)
	if err != nil {
		return err
	}
	specialistRoleID := strings.TrimSpace(resolvedSpecialistRoleID)
	if specialistRoleID == "" {
		specialistRoleID = rolectx.SpecialistRoleIDFromContext(ctx)
	}
	if specialistRoleID != "" && role == "" {
		role = types.SessionRoleMainParent
	}
	isTaskSession := strings.HasPrefix(strings.TrimSpace(in.Session.ID), "task_session_")
	if specialistRoleID == "" && role != types.SessionRoleMainParent && !isTaskSession {
		return fmt.Errorf("session %q in workspace %q is neither main_parent nor mapped specialist", strings.TrimSpace(in.Session.ID), strings.TrimSpace(in.Session.WorkspaceRoot))
	}
	if isTaskSession && role == "" {
		role = types.SessionRoleMainParent
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
		Session:     in.Session,
		SessionRole: sessionrole.Normalize(string(role)),
		Turn:        in.Turn,
		TaskID:      firstNonEmptyTrimmed(taskIDFromResume(in.Resume)),
		Sink:        sink,
		Resume:      in.Resume,
	})
	if observerSink != nil && len(observers) > 0 && err == nil {
		finalObserverText = strings.TrimSpace(observerSink.FinalText())
		if in.Resume != nil && finalObserverText != "" {
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
	if taskObserver != nil && finalObserverText != "" {
		if err := taskObserver.SetFinalText(finalObserverText); err != nil {
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
	ctx = workspace.WithWorkspaceRoot(ctx, sessionRow.WorkspaceRoot)
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

func (a sessionRunnerAdapter) currentTime() time.Time {
	if a.now != nil {
		return a.now().UTC()
	}
	return time.Now().UTC()
}
