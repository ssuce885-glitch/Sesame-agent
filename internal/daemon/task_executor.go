package daemon

import (
	"context"
	"errors"
	"strings"
	"time"

	"go-agent/internal/engine"
	rolectx "go-agent/internal/roles"
	"go-agent/internal/session"
	"go-agent/internal/sessionbinding"
	"go-agent/internal/store/sqlite"
	"go-agent/internal/task"
	"go-agent/internal/types"
	"go-agent/internal/workspace"
)

type agentTaskExecutor struct {
	runner  *engine.Engine
	store   *sqlite.Store
	manager *session.Manager
	now     func() time.Time
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

func (a agentTaskExecutor) RunTask(ctx context.Context, taskID string, workspaceRoot string, prompt string, activatedSkillNames []string, targetRole string, observer task.AgentTaskObserver) error {
	if a.runner == nil {
		return errors.New("engine runner is not configured")
	}

	sessionID := types.NewID("task_session")
	turnID := types.NewID("task_turn")
	taskCtx := sessionbinding.WithContextBinding(ctx, taskContextBinding(sessionID))
	taskCtx = workspace.WithWorkspaceRoot(taskCtx, workspaceRoot)
	targetRole = strings.TrimSpace(targetRole)
	specialistRoleID := ""
	if targetRole != "" && targetRole != string(types.SessionRoleMainParent) {
		specialistRoleID = targetRole
	}
	taskCtx = rolectx.WithSpecialistRoleID(taskCtx, specialistRoleID)
	if err := a.prepareTaskRun(taskCtx, sessionID, turnID, workspaceRoot, prompt); err != nil {
		return err
	}
	sink := &taskEventSink{observer: observer}
	if observer != nil {
		if err := observer.SetRunContext(sessionID, turnID); err != nil {
			return err
		}
	}
	if err := a.runner.RunTurn(taskCtx, engine.Input{
		Session: types.Session{
			ID:            sessionID,
			WorkspaceRoot: workspaceRoot,
		},
		Turn: types.Turn{
			ID:          turnID,
			SessionID:   sessionID,
			Kind:        types.TurnKindUserMessage,
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
		Kind:        types.TurnKindUserMessage,
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
	if err := a.ensureTaskContextHead(ctx, sessionRow); err != nil {
		return err
	}
	if a.manager != nil {
		a.manager.RegisterSession(sessionRow)
	}
	return nil
}

func (a agentTaskExecutor) ensureTaskContextHead(ctx context.Context, sessionRow types.Session) error {
	if a.store == nil {
		return nil
	}
	ctx = workspace.WithWorkspaceRoot(ctx, sessionRow.WorkspaceRoot)
	if headID, ok, err := a.store.GetCurrentContextHeadID(ctx); err != nil {
		return err
	} else if ok {
		head, found, err := a.store.GetContextHead(ctx, headID)
		if err != nil {
			return err
		}
		if found && head.SessionID == sessionRow.ID {
			return a.store.AssignTurnsWithoutHead(ctx, sessionRow.ID, head.ID)
		}
	}

	now := a.currentTime()
	head := types.ContextHead{
		ID:         types.NewID("head"),
		SessionID:  sessionRow.ID,
		SourceKind: types.ContextHeadSourceBootstrap,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if err := a.store.InsertContextHead(ctx, head); err != nil {
		return err
	}
	if err := a.store.AssignTurnsWithoutHead(ctx, sessionRow.ID, head.ID); err != nil {
		return err
	}
	return a.store.SetCurrentContextHeadID(ctx, head.ID)
}

func taskContextBinding(sessionID string) string {
	return "task:" + strings.TrimSpace(sessionID)
}

func (a agentTaskExecutor) currentTime() time.Time {
	if a.now != nil {
		return a.now().UTC()
	}
	return time.Now().UTC()
}
