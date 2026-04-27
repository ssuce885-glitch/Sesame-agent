package daemon

import (
	"context"
	"errors"
	"strings"
	"time"

	"go-agent/internal/engine"
	rolectx "go-agent/internal/roles"
	"go-agent/internal/session"
	"go-agent/internal/sessionrole"
	"go-agent/internal/store/sqlite"
	"go-agent/internal/task"
	"go-agent/internal/types"
	"go-agent/internal/workspace"
)

type agentTaskExecutor struct {
	runner  *engine.Engine
	store   agentTaskStore
	manager *session.Manager
	now     func() time.Time
}

type agentTaskStore interface {
	GetSession(context.Context, string) (types.Session, bool, error)
	InsertSession(context.Context, types.Session) error
	GetTurn(context.Context, string) (types.Turn, bool, error)
	InsertTurn(context.Context, types.Turn) error
	GetCurrentContextHeadID(context.Context) (string, bool, error)
	GetContextHead(context.Context, string) (types.ContextHead, bool, error)
	InsertContextHead(context.Context, types.ContextHead) error
	AssignTurnsWithoutHead(context.Context, string, string) error
	SetCurrentContextHeadID(context.Context, string) error
	EnsureRoleSession(context.Context, string, types.SessionRole) (types.Session, types.ContextHead, bool, error)
	EnsureSpecialistSession(context.Context, string, string, string, []string) (types.Session, types.ContextHead, bool, error)
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

	turnID := types.NewID("task_turn")
	taskCtx, sessionRow, sessionRole, activeSkillNames, contextHeadID, err := a.resolveTaskRunContext(ctx, workspaceRoot, activatedSkillNames, targetRole)
	if err != nil {
		return err
	}
	turnRow := types.Turn{
		ID:            turnID,
		SessionID:     sessionRow.ID,
		ContextHeadID: contextHeadID,
		Kind:          types.TurnKindUserMessage,
		UserMessage:   prompt,
	}
	if err := a.prepareTaskRun(taskCtx, sessionRow, turnRow); err != nil {
		return err
	}
	sink := &taskEventSink{observer: observer}
	if observer != nil {
		if err := observer.SetRunContext(sessionRow.ID, turnID); err != nil {
			return err
		}
	}
	if a.shouldSubmitThroughSessionManager(sessionRow) {
		return a.runManagedTaskTurn(taskCtx, sessionRow, turnRow, strings.TrimSpace(taskID), activeSkillNames, observer)
	}
	if err := a.runner.RunTurn(taskCtx, engine.Input{
		Session:             sessionRow,
		SessionRole:         sessionRole,
		Turn:                turnRow,
		TaskID:              strings.TrimSpace(taskID),
		Sink:                sink,
		ActivatedSkillNames: activeSkillNames,
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

func (a agentTaskExecutor) shouldSubmitThroughSessionManager(sessionRow types.Session) bool {
	return a.manager != nil && !strings.HasPrefix(strings.TrimSpace(sessionRow.ID), "task_session_")
}

func (a agentTaskExecutor) runManagedTaskTurn(ctx context.Context, sessionRow types.Session, turnRow types.Turn, taskID string, activatedSkillNames []string, observer task.AgentTaskObserver) error {
	done := make(chan error, 1)
	if _, err := a.manager.SubmitTurn(ctx, sessionRow.ID, session.SubmitTurnInput{
		Turn: turnRow,
		Run: session.RunMetadata{
			TaskID:                  taskID,
			TaskObserver:            observer,
			ActivatedSkillNames:     append([]string(nil), activatedSkillNames...),
			CancelWithSubmitContext: true,
			Done:                    done,
		},
	}); err != nil {
		return err
	}

	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		a.manager.CancelTurn(sessionRow.ID, turnRow.ID)
		select {
		case err := <-done:
			if err != nil {
				return err
			}
			return ctx.Err()
		default:
			return ctx.Err()
		}
	}
}

func (a agentTaskExecutor) resolveTaskRunContext(ctx context.Context, workspaceRoot string, activatedSkillNames []string, targetRole string) (context.Context, types.Session, types.SessionRole, []string, string, error) {
	workspaceRoot = strings.TrimSpace(workspaceRoot)
	targetRole = strings.TrimSpace(targetRole)
	if targetRole != "" {
		if a.store == nil {
			return nil, types.Session{}, "", nil, "", errors.New("target role execution requires persistent runtime store")
		}
		roleID, err := rolectx.CanonicalRoleID(targetRole)
		if err != nil {
			return nil, types.Session{}, "", nil, "", err
		}
		if roleID == string(types.SessionRoleMainParent) {
			sessionRow, head, _, err := a.store.EnsureRoleSession(ctx, workspaceRoot, types.SessionRoleMainParent)
			if err != nil {
				return nil, types.Session{}, "", nil, "", err
			}
			runCtx := withRunnerSessionContext(ctx, sessionRow, types.SessionRoleMainParent, "")
			activeSkillNames := sessionrole.MergeActivatedSkillNames(activatedSkillNames, types.SessionRoleMainParent, nil)
			return runCtx, sessionRow, types.SessionRoleMainParent, activeSkillNames, strings.TrimSpace(head.ID), nil
		}
		catalog, err := rolectx.LoadCatalog(workspaceRoot)
		if err != nil {
			return nil, types.Session{}, "", nil, "", err
		}
		spec, ok := catalog.ByID[roleID]
		if !ok {
			return nil, types.Session{}, "", nil, "", errors.New("specialist role is not installed: " + roleID)
		}
		sessionRow, head, _, err := a.store.EnsureSpecialistSession(ctx, workspaceRoot, roleID, spec.Prompt, spec.SkillNames)
		if err != nil {
			return nil, types.Session{}, "", nil, "", err
		}
		runCtx := withRunnerSessionContext(ctx, sessionRow, types.SessionRoleMainParent, roleID)
		activeSkillNames := sessionrole.MergeActivatedSkillNames(activatedSkillNames, types.SessionRoleMainParent, &spec)
		return runCtx, sessionRow, types.SessionRoleMainParent, activeSkillNames, strings.TrimSpace(head.ID), nil
	}

	now := a.currentTime()
	sessionRow := types.Session{
		ID:                types.NewID("task_session"),
		WorkspaceRoot:     workspaceRoot,
		PermissionProfile: "trusted_local",
		State:             types.SessionStateIdle,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	runCtx := withRunnerSessionContext(ctx, sessionRow, "", "")
	return runCtx, sessionRow, "", append([]string(nil), activatedSkillNames...), "", nil
}

func (a agentTaskExecutor) prepareTaskRun(ctx context.Context, sessionRow types.Session, turnRow types.Turn) error {
	if a.store == nil {
		return nil
	}
	now := a.currentTime()
	if existing, ok, err := a.store.GetSession(ctx, sessionRow.ID); err != nil {
		return err
	} else if ok {
		sessionRow = existing
	} else if err := a.store.InsertSession(ctx, sessionRow); err != nil {
		return err
	}
	turnRow.SessionID = sessionRow.ID
	turnRow.State = types.TurnStateCreated
	turnRow.CreatedAt = now
	turnRow.UpdatedAt = now
	if _, ok, err := a.store.GetTurn(ctx, turnRow.ID); err != nil {
		return err
	} else if !ok {
		if err := a.store.InsertTurn(ctx, turnRow); err != nil {
			return err
		}
	}
	if strings.TrimSpace(turnRow.ContextHeadID) == "" {
		if err := a.ensureTaskContextHead(ctx, sessionRow); err != nil {
			return err
		}
	}
	a.registerTaskSession(sessionRow)
	return nil
}

func (a agentTaskExecutor) registerTaskSession(sessionRow types.Session) {
	if a.manager == nil {
		return
	}
	if a.manager.UpdateSession(sessionRow) {
		return
	}
	a.manager.RegisterSession(sessionRow)
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
