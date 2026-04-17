package session

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"go-agent/internal/sessionrole"
	"go-agent/internal/types"
)

type DelegateToRoleInput struct {
	WorkspaceRoot   string
	SourceSessionID string
	SourceTurnID    string
	TargetRole      types.SessionRole
	Message         string
	Reason          string
}

type DelegateToRoleOutput struct {
	TargetRole      types.SessionRole
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

	targetRole := sessionrole.Normalize(string(in.TargetRole))
	roleCtx := sessionrole.WithSessionRole(ctx, targetRole)
	targetSession, targetHead, createdSession, err := s.store.EnsureRoleSession(roleCtx, workspaceRoot, targetRole)
	if err != nil {
		return DelegateToRoleOutput{}, err
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
	if err := s.store.InsertTurn(roleCtx, turn); err != nil {
		return DelegateToRoleOutput{}, err
	}
	if !s.manager.UpdateSession(targetSession) {
		s.manager.RegisterSession(targetSession)
	}
	if _, err := s.manager.SubmitTurn(roleCtx, targetSession.ID, SubmitTurnInput{Turn: turn}); err != nil {
		deleteErr := s.store.DeleteTurn(context.WithoutCancel(roleCtx), turn.ID)
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
