package tasks

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go-agent/internal/types"
	"go-agent/internal/v2/contracts"
	"go-agent/internal/v2/roles"
	v2session "go-agent/internal/v2/session"
)

const defaultAgentTaskTimeout = 30 * time.Minute

type AgentRunner struct {
	store       contracts.Store
	sessionMgr  contracts.SessionManager
	roleService RoleLister
}

type RoleLister interface {
	Get(id string) (roles.RoleSpec, bool, error)
}

func NewAgentRunner(store contracts.Store, sessionMgr contracts.SessionManager, roleService RoleLister) *AgentRunner {
	return &AgentRunner{
		store:       store,
		sessionMgr:  sessionMgr,
		roleService: roleService,
	}
}

func (r *AgentRunner) Run(ctx context.Context, task contracts.Task, sink OutputSink) error {
	if r.store == nil {
		return fmt.Errorf("store is required")
	}
	if r.sessionMgr == nil {
		return fmt.Errorf("session manager is required")
	}
	if r.roleService == nil {
		return fmt.Errorf("role service is required")
	}

	roleID := strings.TrimSpace(task.RoleID)
	if roleID == "" {
		return fmt.Errorf("role_id is required for agent task")
	}
	roleSpec, ok, err := r.roleService.Get(roleID)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("role %q not found", roleID)
	}

	workspaceRoot := strings.TrimSpace(task.WorkspaceRoot)
	if workspaceRoot == "" {
		return fmt.Errorf("workspace_root is required")
	}
	specialist, err := r.ensureSpecialistSession(ctx, workspaceRoot, roleSpec)
	if err != nil {
		return err
	}

	now := time.Now().UTC()
	turnID := strings.TrimSpace(task.TurnID)
	if turnID == "" {
		turnID = types.NewID("turn")
	}
	turn := contracts.Turn{
		ID:          turnID,
		SessionID:   specialist.ID,
		Kind:        "user_message",
		State:       "created",
		UserMessage: task.Prompt,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := retryDatabaseBusy(ctx, func(ctx context.Context) error {
		return r.store.Turns().Create(ctx, turn)
	}); err != nil {
		return fmt.Errorf("create agent turn: %w", err)
	}

	task.SessionID = specialist.ID
	task.TurnID = turn.ID
	task.RoleID = roleID
	task.UpdatedAt = now
	if err := retryDatabaseBusy(ctx, func(ctx context.Context) error {
		return r.store.Tasks().Update(ctx, task)
	}); err != nil {
		_ = r.store.Turns().UpdateState(context.WithoutCancel(ctx), turn.ID, "failed")
		return fmt.Errorf("update agent task turn: %w", err)
	}

	turnRoleSpec := &contracts.RoleSpec{
		ID:                roleSpec.ID,
		Model:             roleSpec.Model,
		PermissionProfile: roleSpec.PermissionProfile,
		MaxToolCalls:      roleSpec.MaxToolCalls,
		MaxRuntime:        roleSpec.MaxRuntime,
		MaxContextTokens:  roleSpec.MaxContextTokens,
		SkillNames:        append([]string(nil), roleSpec.SkillNames...),
		DeniedTools:       append([]string(nil), roleSpec.DeniedTools...),
		AllowedTools:      append([]string(nil), roleSpec.AllowedTools...),
		DeniedPaths:       append([]string(nil), roleSpec.DeniedPaths...),
		AllowedPaths:      append([]string(nil), roleSpec.AllowedPaths...),
		ToolPolicy:        contracts.CloneToolPolicyMap(roleSpec.ToolPolicy),
		CanDelegate:       roleSpec.CanDelegate,
		AutomationOwners:  append([]string(nil), roleSpec.AutomationOwners...),
	}

	runCtx := ctx
	var cancel context.CancelFunc
	if _, hasDeadline := runCtx.Deadline(); !hasDeadline {
		timeout := defaultAgentTaskTimeout
		if roleSpec.MaxRuntime > 0 {
			timeout = time.Duration(roleSpec.MaxRuntime) * time.Second
		}
		runCtx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	if _, err := r.sessionMgr.SubmitTurn(runCtx, specialist.ID, contracts.SubmitTurnInput{Turn: turn, RoleSpec: turnRoleSpec}); err != nil {
		_ = r.store.Turns().UpdateState(context.WithoutCancel(ctx), turn.ID, "failed")
		return fmt.Errorf("submit agent turn: %w", err)
	}

	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-runCtx.Done():
			r.sessionMgr.CancelTurn(specialist.ID, turn.ID)
			_ = r.store.Turns().UpdateState(context.WithoutCancel(runCtx), turn.ID, "interrupted")
			return runCtx.Err()
		case <-ticker.C:
			latest, err := r.store.Turns().Get(runCtx, turn.ID)
			if err != nil {
				return fmt.Errorf("load agent turn: %w", err)
			}
			switch latest.State {
			case "completed":
				finalText, err := r.lastAssistantText(ctx, specialist.ID, turn.ID)
				if err != nil {
					return err
				}
				if strings.TrimSpace(finalText) != "" && sink != nil {
					if err := sink.Append(task.ID, []byte(finalText)); err != nil {
						return err
					}
				}
				return r.persistFinalText(ctx, task, specialist.ID, turn.ID, roleID, finalText)
			case "failed":
				return fmt.Errorf("agent turn %s failed", turn.ID)
			case "interrupted":
				return fmt.Errorf("agent turn %s was interrupted", turn.ID)
			}
		}
	}
}

func (r *AgentRunner) ensureSpecialistSession(ctx context.Context, workspaceRoot string, roleSpec roles.RoleSpec) (contracts.Session, error) {
	sessionID := v2session.SpecialistSessionID(roleSpec.ID, workspaceRoot)
	sessions, err := r.store.Sessions().ListByWorkspace(ctx, workspaceRoot)
	if err != nil {
		return contracts.Session{}, err
	}
	for _, session := range sessions {
		if session.ID == sessionID {
			r.sessionMgr.Register(session)
			return session, nil
		}
	}

	now := time.Now().UTC()
	session := contracts.Session{
		ID:                sessionID,
		WorkspaceRoot:     workspaceRoot,
		SystemPrompt:      roleSpec.SystemPrompt,
		PermissionProfile: roleSpec.PermissionProfile,
		State:             "idle",
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	if err := r.store.Sessions().Create(ctx, session); err != nil {
		return contracts.Session{}, err
	}
	r.sessionMgr.Register(session)
	return session, nil
}

func (r *AgentRunner) lastAssistantText(ctx context.Context, sessionID, turnID string) (string, error) {
	messages, err := r.store.Messages().List(ctx, sessionID, contracts.MessageListOptions{})
	if err != nil {
		return "", fmt.Errorf("load agent messages: %w", err)
	}
	for i := len(messages) - 1; i >= 0; i-- {
		msg := messages[i]
		if msg.TurnID != turnID || msg.Role != "assistant" {
			continue
		}
		content := strings.TrimSpace(msg.Content)
		if content == "" || strings.HasPrefix(content, "__thinking_json__:") || strings.HasPrefix(content, "__tool_call_json__:") {
			continue
		}
		return msg.Content, nil
	}
	return "", nil
}

func (r *AgentRunner) persistFinalText(ctx context.Context, task contracts.Task, sessionID, turnID, roleID, finalText string) error {
	current, err := r.store.Tasks().Get(ctx, task.ID)
	if err != nil {
		current = task
	}
	current.SessionID = sessionID
	current.TurnID = turnID
	current.RoleID = roleID
	current.FinalText = finalText
	current.UpdatedAt = time.Now().UTC()
	return r.store.Tasks().Update(ctx, current)
}
