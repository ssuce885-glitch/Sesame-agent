package session

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	rolectx "go-agent/internal/roles"
	"go-agent/internal/sessionrole"
	"go-agent/internal/types"
)

type DelegateToRoleInput struct {
	WorkspaceRoot   string
	SourceSessionID string
	SourceTurnID    string
	TargetRole      string
	Message         string
	Reason          string
}

type DelegateToRoleOutput struct {
	TargetRole      string
	TargetSessionID string
	TargetTurnID    string
	Accepted        bool
	CreatedSession  bool
}

type RoleDelegationService interface {
	DelegateToRole(context.Context, DelegateToRoleInput) (DelegateToRoleOutput, error)
}

type roleDelegationStore interface {
	EnsureRoleSession(context.Context, string, types.SessionRole) (types.Session, types.ContextHead, bool, error)
	EnsureSpecialistSession(context.Context, string, string, string, []string) (types.Session, types.ContextHead, bool, error)
	ResolveSessionRole(context.Context, string, string) (types.SessionRole, error)
	ResolveSpecialistRoleID(context.Context, string, string) (string, error)
	InsertTurn(context.Context, types.Turn) error
	DeleteTurn(context.Context, string) error
}

type roleDelegationManager interface {
	RegisterSession(types.Session)
	UpdateSession(types.Session) bool
	SubmitTurn(context.Context, string, SubmitTurnInput) (string, error)
}

type DelegationService struct {
	store   roleDelegationStore
	manager roleDelegationManager
	now     func() time.Time
}

func NewDelegationService(store roleDelegationStore, manager roleDelegationManager) *DelegationService {
	return &DelegationService{
		store:   store,
		manager: manager,
		now: func() time.Time {
			return time.Now().UTC()
		},
	}
}

func (s *DelegationService) DelegateToRole(ctx context.Context, in DelegateToRoleInput) (DelegateToRoleOutput, error) {
	if s == nil || s.store == nil || s.manager == nil {
		return DelegateToRoleOutput{}, fmt.Errorf("session delegation service is not configured")
	}

	workspaceRoot := strings.TrimSpace(in.WorkspaceRoot)
	sourceSessionID := strings.TrimSpace(in.SourceSessionID)
	message := strings.TrimSpace(in.Message)
	if workspaceRoot == "" {
		return DelegateToRoleOutput{}, fmt.Errorf("workspace_root is required")
	}
	if sourceSessionID == "" {
		return DelegateToRoleOutput{}, fmt.Errorf("source_session_id is required")
	}
	if message == "" {
		return DelegateToRoleOutput{}, fmt.Errorf("message is required")
	}

	targetRole, err := validateRoleID(in.TargetRole)
	if err != nil {
		return DelegateToRoleOutput{}, err
	}
	sourceSessionRole, err := s.store.ResolveSessionRole(ctx, sourceSessionID, workspaceRoot)
	if err != nil {
		return DelegateToRoleOutput{}, err
	}
	sourceSpecialistRoleID := rolectx.SpecialistRoleIDFromContext(ctx)
	if sourceSpecialistRoleID == "" {
		resolvedSpecialistRoleID, err := s.store.ResolveSpecialistRoleID(ctx, sourceSessionID, workspaceRoot)
		if err != nil {
			return DelegateToRoleOutput{}, err
		}
		sourceSpecialistRoleID = resolvedSpecialistRoleID
	}
	if strings.TrimSpace(sourceSpecialistRoleID) == "" && sourceSessionRole != types.SessionRoleMainParent && !strings.HasPrefix(sourceSessionID, "task_session_") {
		return DelegateToRoleOutput{}, fmt.Errorf("source session %q in workspace %q is neither main_parent nor mapped specialist", sourceSessionID, workspaceRoot)
	}
	if sourceSpecialistRoleID != "" && targetRole != string(types.SessionRoleMainParent) {
		return DelegateToRoleOutput{}, fmt.Errorf("specialist roles cannot delegate directly to another specialist role; report back to main_parent and ask it to delegate")
	}

	var (
		targetCtx      context.Context
		targetSession  types.Session
		targetHead     types.ContextHead
		createdSession bool
	)
	if targetRole == string(types.SessionRoleMainParent) {
		targetCtx = rolectx.WithSpecialistRoleID(
			sessionrole.WithSessionRole(ctx, types.SessionRoleMainParent),
			"",
		)
		targetSession, targetHead, createdSession, err = s.store.EnsureRoleSession(targetCtx, workspaceRoot, types.SessionRoleMainParent)
		if err != nil {
			return DelegateToRoleOutput{}, err
		}
	} else {
		catalog, err := rolectx.LoadCatalog(workspaceRoot)
		if err != nil {
			return DelegateToRoleOutput{}, err
		}
		spec, ok := catalog.ByID[targetRole]
		if !ok {
			return DelegateToRoleOutput{}, fmt.Errorf("target_role %q is not installed", targetRole)
		}
		targetCtx = rolectx.WithSpecialistRoleID(
			sessionrole.WithSessionRole(ctx, types.SessionRoleMainParent),
			spec.RoleID,
		)
		targetSession, targetHead, createdSession, err = s.store.EnsureSpecialistSession(
			targetCtx,
			workspaceRoot,
			spec.RoleID,
			spec.Prompt,
			spec.SkillNames,
		)
		if err != nil {
			return DelegateToRoleOutput{}, err
		}
	}
	if strings.TrimSpace(targetSession.ID) == sourceSessionID {
		return DelegateToRoleOutput{}, fmt.Errorf("target role resolves to the current session; continue in this session instead")
	}
	if targetSession.State == types.SessionStateAwaitingPermission {
		return DelegateToRoleOutput{}, fmt.Errorf("target role session is awaiting permission; resolve that request before delegating more work")
	}

	now := s.now()
	turn := types.Turn{
		ID:            types.NewID("turn"),
		SessionID:     targetSession.ID,
		ContextHeadID: strings.TrimSpace(targetHead.ID),
		Kind:          types.TurnKindUserMessage,
		State:         types.TurnStateCreated,
		UserMessage:   message,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := s.store.InsertTurn(targetCtx, turn); err != nil {
		return DelegateToRoleOutput{}, err
	}
	if !s.manager.UpdateSession(targetSession) {
		s.manager.RegisterSession(targetSession)
	}
	if _, err := s.manager.SubmitTurn(targetCtx, targetSession.ID, SubmitTurnInput{Turn: turn}); err != nil {
		deleteErr := s.store.DeleteTurn(context.WithoutCancel(targetCtx), turn.ID)
		if deleteErr != nil {
			return DelegateToRoleOutput{}, errors.Join(err, deleteErr)
		}
		return DelegateToRoleOutput{}, err
	}

	return DelegateToRoleOutput{
		TargetRole:      targetRole,
		TargetSessionID: targetSession.ID,
		TargetTurnID:    turn.ID,
		Accepted:        true,
		CreatedSession:  createdSession,
	}, nil
}

func validateRoleID(raw string) (string, error) {
	roleID := strings.TrimSpace(raw)
	if roleID == "" {
		return "", fmt.Errorf("target_role is required")
	}
	if strings.HasPrefix(roleID, ".") || strings.Contains(roleID, "/") || strings.Contains(roleID, "\\") || strings.Contains(roleID, "..") {
		return "", fmt.Errorf("invalid target_role %q", roleID)
	}
	return roleID, nil
}
