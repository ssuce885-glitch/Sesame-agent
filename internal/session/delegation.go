package session

import (
	"context"
	"fmt"
	"strings"

	rolectx "go-agent/internal/roles"
	"go-agent/internal/task"
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
	TaskID     string
	TargetRole string
	Accepted   bool
}

type RoleDelegationService interface {
	DelegateToRole(context.Context, DelegateToRoleInput) (DelegateToRoleOutput, error)
}

type roleDelegationStore interface {
	ResolveSessionRole(context.Context, string, string) (types.SessionRole, error)
	ResolveSpecialistRoleID(context.Context, string, string) (string, error)
}

type roleDelegationTaskManager interface {
	Create(context.Context, task.CreateTaskInput) (task.Task, error)
}

type DelegationService struct {
	store       roleDelegationStore
	taskManager roleDelegationTaskManager
}

func NewDelegationService(store roleDelegationStore, taskManager roleDelegationTaskManager) *DelegationService {
	return &DelegationService{
		store:       store,
		taskManager: taskManager,
	}
}

func (s *DelegationService) DelegateToRole(ctx context.Context, in DelegateToRoleInput) (DelegateToRoleOutput, error) {
	if s == nil || s.store == nil || s.taskManager == nil {
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

	targetRole, err := rolectx.CanonicalRoleID(in.TargetRole)
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

	activatedSkillNames := []string(nil)
	if targetRole == string(types.SessionRoleMainParent) {
		targetRole = string(types.SessionRoleMainParent)
	} else {
		catalog, err := rolectx.LoadCatalog(workspaceRoot)
		if err != nil {
			return DelegateToRoleOutput{}, err
		}
		spec, ok := catalog.ByID[targetRole]
		if !ok {
			return DelegateToRoleOutput{}, fmt.Errorf("target_role %q is currently unavailable", targetRole)
		}
		activatedSkillNames = append([]string(nil), spec.SkillNames...)
	}

	description := strings.TrimSpace(in.Reason)
	if description == "" {
		description = fmt.Sprintf("delegate_to_role -> %s", targetRole)
	}
	createdTask, err := s.taskManager.Create(ctx, task.CreateTaskInput{
		Type:                task.TaskTypeAgent,
		Command:             message,
		Description:         description,
		ParentSessionID:     sourceSessionID,
		ParentTurnID:        strings.TrimSpace(in.SourceTurnID),
		Kind:                "specialist_role",
		Owner:               targetRole,
		ActivatedSkillNames: activatedSkillNames,
		TargetRole:          targetRole,
		WorkspaceRoot:       workspaceRoot,
		Start:               true,
	})
	if err != nil {
		return DelegateToRoleOutput{}, err
	}
	return DelegateToRoleOutput{
		TaskID:     createdTask.ID,
		TargetRole: targetRole,
		Accepted:   true,
	}, nil
}
