package contextasm

import (
	"fmt"
	"strings"
)

const (
	visibilityGlobal     = "global"
	visibilityWorkspace  = "workspace"
	visibilityMainOnly   = "main_only"
	visibilityRoleOnly   = "role_only"
	visibilityRoleShared = "role_shared"
	visibilityTaskOnly   = "task_only"
	visibilityPrivate    = "private"
	visibilitySession    = "session"
)

type ownerKind string

const (
	ownerUser        ownerKind = "user"
	ownerWorkspace   ownerKind = "workspace"
	ownerMainSession ownerKind = "main_session"
	ownerRole        ownerKind = "role"
	ownerTask        ownerKind = "task"
	ownerWorkflowRun ownerKind = "workflow_run"
	ownerAutomation  ownerKind = "automation"
)

type ownerRef struct {
	kind ownerKind
	id   string
	raw  string
}

func FilterVisibleBlocks(scope ExecutionScope, blocks []SourceBlock) ([]SourceBlock, error) {
	scope = scope.normalized()
	if err := scope.Validate(); err != nil {
		return nil, err
	}
	if len(blocks) == 0 {
		return []SourceBlock{}, nil
	}
	out := make([]SourceBlock, 0, len(blocks))
	for _, block := range blocks {
		block = block.normalized()
		if err := block.Validate(); err != nil {
			return nil, err
		}
		visible, err := IsVisibleToScope(scope, block)
		if err != nil {
			return nil, err
		}
		if visible {
			out = append(out, block)
		}
	}
	return out, nil
}

func IsVisibleToScope(scope ExecutionScope, block SourceBlock) (bool, error) {
	scope = scope.normalized()
	if err := scope.Validate(); err != nil {
		return false, err
	}
	block = block.normalized()
	if err := block.Validate(); err != nil {
		return false, err
	}
	owner, err := parseOwner(block.Owner)
	if err != nil {
		return false, err
	}
	visibility := normalizeVisibility(block.Visibility)
	switch visibility {
	case visibilityGlobal, visibilityWorkspace:
		return true, nil
	case visibilityMainOnly:
		return scope.Kind == ScopeMain, nil
	case visibilityRoleShared:
		if scope.Kind == ScopeMain {
			return true, nil
		}
		return matchesRoleAudience(scope, owner), nil
	case visibilityRoleOnly:
		if scope.Kind == ScopeMain {
			return false, nil
		}
		return matchesRoleOnlyAudience(scope, owner), nil
	case visibilityTaskOnly:
		if scope.Kind != ScopeTask {
			return false, nil
		}
		return owner.kind == ownerTask && scope.TaskID == owner.id, nil
	case visibilityPrivate:
		return false, nil
	case visibilitySession:
		return matchesOwnerLineage(scope, owner), nil
	default:
		return false, fmt.Errorf("%w: unsupported visibility %q", ErrInvalidInput, block.Visibility)
	}
}

func normalizeVisibility(value string) string {
	return strings.TrimSpace(value)
}

func parseOwner(raw string) (ownerRef, error) {
	raw = strings.TrimSpace(raw)
	switch {
	case raw == "":
		return ownerRef{}, fmt.Errorf("%w: owner is required", ErrInvalidInput)
	case raw == string(ownerUser):
		return ownerRef{kind: ownerUser, raw: raw}, nil
	case raw == string(ownerWorkspace):
		return ownerRef{kind: ownerWorkspace, raw: raw}, nil
	case raw == string(ownerMainSession):
		return ownerRef{kind: ownerMainSession, raw: raw}, nil
	case strings.HasPrefix(raw, "role:"):
		id := strings.TrimSpace(strings.TrimPrefix(raw, "role:"))
		if id == "" {
			return ownerRef{}, fmt.Errorf("%w: role owner id is required", ErrInvalidInput)
		}
		return ownerRef{kind: ownerRole, id: id, raw: raw}, nil
	case strings.HasPrefix(raw, "task:"):
		id := strings.TrimSpace(strings.TrimPrefix(raw, "task:"))
		if id == "" {
			return ownerRef{}, fmt.Errorf("%w: task owner id is required", ErrInvalidInput)
		}
		return ownerRef{kind: ownerTask, id: id, raw: raw}, nil
	case strings.HasPrefix(raw, "workflow_run:"):
		id := strings.TrimSpace(strings.TrimPrefix(raw, "workflow_run:"))
		if id == "" {
			return ownerRef{}, fmt.Errorf("%w: workflow_run owner id is required", ErrInvalidInput)
		}
		return ownerRef{kind: ownerWorkflowRun, id: id, raw: raw}, nil
	case strings.HasPrefix(raw, "automation:"):
		id := strings.TrimSpace(strings.TrimPrefix(raw, "automation:"))
		if id == "" {
			return ownerRef{}, fmt.Errorf("%w: automation owner id is required", ErrInvalidInput)
		}
		return ownerRef{kind: ownerAutomation, id: id, raw: raw}, nil
	default:
		return ownerRef{}, fmt.Errorf("%w: unsupported owner %q", ErrInvalidInput, raw)
	}
}

func matchesExactOwner(scope ExecutionScope, owner ownerRef) bool {
	switch owner.kind {
	case ownerUser, ownerWorkspace, ownerMainSession:
		return scope.Kind == ScopeMain
	case ownerRole:
		return scope.Kind == ScopeRole && scope.RoleID == owner.id
	case ownerTask:
		return scope.Kind == ScopeTask && scope.TaskID == owner.id
	default:
		return false
	}
}

func matchesOwnerLineage(scope ExecutionScope, owner ownerRef) bool {
	switch owner.kind {
	case ownerUser, ownerWorkspace, ownerMainSession:
		return scope.Kind == ScopeMain
	case ownerRole:
		if scope.RoleID == "" {
			return false
		}
		return scope.RoleID == owner.id && (scope.Kind == ScopeRole || scope.Kind == ScopeTask)
	case ownerTask:
		return scope.Kind == ScopeTask && scope.TaskID == owner.id
	default:
		return false
	}
}

func matchesRoleAudience(scope ExecutionScope, owner ownerRef) bool {
	if scope.Kind != ScopeRole && scope.Kind != ScopeTask {
		return false
	}
	switch owner.kind {
	case ownerUser, ownerWorkspace:
		return true
	case ownerRole:
		return scope.RoleID != "" && scope.RoleID == owner.id
	case ownerTask:
		return scope.Kind == ScopeTask && scope.TaskID == owner.id
	default:
		return false
	}
}

func matchesRoleOnlyAudience(scope ExecutionScope, owner ownerRef) bool {
	if scope.Kind != ScopeRole && scope.Kind != ScopeTask {
		return false
	}
	switch owner.kind {
	case ownerRole:
		return scope.RoleID != "" && scope.RoleID == owner.id
	case ownerTask:
		return scope.Kind == ScopeTask && scope.TaskID == owner.id
	default:
		return false
	}
}

func matchesTaskAudience(scope ExecutionScope, owner ownerRef) bool {
	if scope.Kind != ScopeTask {
		return false
	}
	switch owner.kind {
	case ownerUser, ownerWorkspace:
		return true
	case ownerRole:
		return scope.RoleID != "" && scope.RoleID == owner.id
	case ownerTask:
		return scope.TaskID == owner.id
	default:
		return false
	}
}
