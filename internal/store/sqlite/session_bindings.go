package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"go-agent/internal/sessionrole"
	"go-agent/internal/types"
)

const (
	workspaceSessionBindingKindMainParent = "main_parent"
	workspaceSessionBindingKindSpecialist = "specialist"
)

type workspaceSessionBinding struct {
	WorkspaceRoot    string
	BindingKind      string
	Role             string
	SpecialistRoleID string
	SessionID        string
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

func newRoleSessionBinding(workspaceRoot string, role types.SessionRole, sessionID string) workspaceSessionBinding {
	return workspaceSessionBinding{
		WorkspaceRoot: strings.TrimSpace(workspaceRoot),
		BindingKind:   workspaceSessionBindingKindMainParent,
		Role:          string(sessionrole.Normalize(string(role))),
		SessionID:     strings.TrimSpace(sessionID),
	}
}

func newSpecialistSessionBinding(workspaceRoot, roleID, sessionID string) workspaceSessionBinding {
	return workspaceSessionBinding{
		WorkspaceRoot:    strings.TrimSpace(workspaceRoot),
		BindingKind:      workspaceSessionBindingKindSpecialist,
		SpecialistRoleID: normalizeSpecialistRoleID(roleID),
		SessionID:        strings.TrimSpace(sessionID),
	}
}

func (b workspaceSessionBinding) normalized() workspaceSessionBinding {
	b.WorkspaceRoot = strings.TrimSpace(b.WorkspaceRoot)
	b.BindingKind = strings.TrimSpace(b.BindingKind)
	b.SessionID = strings.TrimSpace(b.SessionID)
	switch b.BindingKind {
	case workspaceSessionBindingKindMainParent:
		b.Role = string(sessionrole.Normalize(b.Role))
		b.SpecialistRoleID = ""
	case workspaceSessionBindingKindSpecialist:
		b.Role = ""
		b.SpecialistRoleID = normalizeSpecialistRoleID(b.SpecialistRoleID)
	default:
		b.Role = strings.TrimSpace(b.Role)
		b.SpecialistRoleID = normalizeSpecialistRoleID(b.SpecialistRoleID)
	}
	return b
}

func getWorkspaceSessionBinding(
	ctx context.Context,
	queryer queryRowContexter,
	workspaceRoot, bindingKind, role, specialistRoleID string,
) (workspaceSessionBinding, bool, error) {
	workspaceRoot = strings.TrimSpace(workspaceRoot)
	bindingKind = strings.TrimSpace(bindingKind)
	role = strings.TrimSpace(role)
	specialistRoleID = strings.TrimSpace(specialistRoleID)

	var binding workspaceSessionBinding
	var createdAt string
	var updatedAt string
	err := queryer.QueryRowContext(ctx, `
		select workspace_root, binding_kind, role, specialist_role_id, session_id, created_at, updated_at
		from workspace_session_bindings
		where workspace_root = ? and binding_kind = ? and role = ? and specialist_role_id = ?
	`,
		workspaceRoot,
		bindingKind,
		role,
		specialistRoleID,
	).Scan(
		&binding.WorkspaceRoot,
		&binding.BindingKind,
		&binding.Role,
		&binding.SpecialistRoleID,
		&binding.SessionID,
		&createdAt,
		&updatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return workspaceSessionBinding{}, false, nil
	}
	if err != nil {
		return workspaceSessionBinding{}, false, err
	}
	binding.CreatedAt, err = time.Parse(timeLayout, createdAt)
	if err != nil {
		return workspaceSessionBinding{}, false, err
	}
	binding.UpdatedAt, err = time.Parse(timeLayout, updatedAt)
	if err != nil {
		return workspaceSessionBinding{}, false, err
	}
	return binding.normalized(), true, nil
}

func getWorkspaceSessionBindingBySession(
	ctx context.Context,
	queryer queryRowContexter,
	workspaceRoot, sessionID string,
) (workspaceSessionBinding, bool, error) {
	workspaceRoot = strings.TrimSpace(workspaceRoot)
	sessionID = strings.TrimSpace(sessionID)

	var binding workspaceSessionBinding
	var createdAt string
	var updatedAt string
	err := queryer.QueryRowContext(ctx, `
		select workspace_root, binding_kind, role, specialist_role_id, session_id, created_at, updated_at
		from workspace_session_bindings
		where workspace_root = ? and session_id = ?
	`,
		workspaceRoot,
		sessionID,
	).Scan(
		&binding.WorkspaceRoot,
		&binding.BindingKind,
		&binding.Role,
		&binding.SpecialistRoleID,
		&binding.SessionID,
		&createdAt,
		&updatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return workspaceSessionBinding{}, false, nil
	}
	if err != nil {
		return workspaceSessionBinding{}, false, err
	}
	binding.CreatedAt, err = time.Parse(timeLayout, createdAt)
	if err != nil {
		return workspaceSessionBinding{}, false, err
	}
	binding.UpdatedAt, err = time.Parse(timeLayout, updatedAt)
	if err != nil {
		return workspaceSessionBinding{}, false, err
	}
	return binding.normalized(), true, nil
}

func upsertWorkspaceSessionBinding(ctx context.Context, execer execContexter, binding workspaceSessionBinding) error {
	binding = binding.normalized()
	if binding.WorkspaceRoot == "" {
		return errors.New("workspace root is required")
	}
	if binding.SessionID == "" {
		return errors.New("session id is required")
	}
	switch binding.BindingKind {
	case workspaceSessionBindingKindMainParent:
		if binding.Role == "" {
			return errors.New("role binding requires role")
		}
	case workspaceSessionBindingKindSpecialist:
		if binding.SpecialistRoleID == "" {
			return errors.New("specialist binding requires specialist role id")
		}
	default:
		return errors.New("binding kind is required")
	}

	now := time.Now().UTC()
	_, err := execer.ExecContext(ctx, `
		delete from workspace_session_bindings
		where workspace_root = ? and session_id = ?
		  and not (binding_kind = ? and role = ? and specialist_role_id = ?)
	`,
		binding.WorkspaceRoot,
		binding.SessionID,
		binding.BindingKind,
		binding.Role,
		binding.SpecialistRoleID,
	)
	if err != nil {
		return err
	}

	_, err = execer.ExecContext(ctx, `
		insert into workspace_session_bindings (
			workspace_root, binding_kind, role, specialist_role_id, session_id, created_at, updated_at
		)
		values (?, ?, ?, ?, ?, ?, ?)
		on conflict(workspace_root, binding_kind, role, specialist_role_id) do update set
			session_id = excluded.session_id,
			updated_at = excluded.updated_at
	`,
		binding.WorkspaceRoot,
		binding.BindingKind,
		binding.Role,
		binding.SpecialistRoleID,
		binding.SessionID,
		now.Format(timeLayout),
		now.Format(timeLayout),
	)
	return err
}
