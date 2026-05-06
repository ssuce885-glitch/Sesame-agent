package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"go-agent/internal/v2/contracts"
)

type roleRuntimeStateRepo struct {
	db *sql.DB
	tx *sql.Tx
}

var _ contracts.RoleRuntimeStateRepository = (*roleRuntimeStateRepo)(nil)

func (r *roleRuntimeStateRepo) execer() execer { return repoExec(r.db, r.tx) }

func (r *roleRuntimeStateRepo) Get(ctx context.Context, workspaceRoot, roleID string) (contracts.RoleRuntimeState, bool, error) {
	workspaceRoot = strings.TrimSpace(workspaceRoot)
	roleID = strings.TrimSpace(roleID)
	if workspaceRoot == "" || roleID == "" {
		return contracts.RoleRuntimeState{}, false, nil
	}
	var state contracts.RoleRuntimeState
	var createdAt, updatedAt string
	err := r.execer().QueryRow(`
SELECT workspace_root, role_id, summary, source_session_id, source_turn_id, created_at, updated_at
FROM v2_role_runtime_states
WHERE workspace_root = ? AND role_id = ?`, workspaceRoot, roleID).Scan(
		&state.WorkspaceRoot,
		&state.RoleID,
		&state.Summary,
		&state.SourceSessionID,
		&state.SourceTurnID,
		&createdAt,
		&updatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return contracts.RoleRuntimeState{}, false, nil
	}
	if err != nil {
		return contracts.RoleRuntimeState{}, false, err
	}
	var parseErr error
	state.CreatedAt, parseErr = parseTime(createdAt)
	if parseErr != nil {
		return contracts.RoleRuntimeState{}, false, parseErr
	}
	state.UpdatedAt, parseErr = parseTime(updatedAt)
	if parseErr != nil {
		return contracts.RoleRuntimeState{}, false, parseErr
	}
	return state, true, nil
}

func (r *roleRuntimeStateRepo) Upsert(ctx context.Context, state contracts.RoleRuntimeState) error {
	state.WorkspaceRoot = strings.TrimSpace(state.WorkspaceRoot)
	state.RoleID = strings.TrimSpace(state.RoleID)
	if state.WorkspaceRoot == "" || state.RoleID == "" {
		return nil
	}
	if state.CreatedAt.IsZero() {
		state.CreatedAt = sqlNow()
	}
	if state.UpdatedAt.IsZero() {
		state.UpdatedAt = sqlNow()
	}
	_, err := r.execer().Exec(`
INSERT INTO v2_role_runtime_states (workspace_root, role_id, summary, source_session_id, source_turn_id, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(workspace_root, role_id) DO UPDATE SET
    summary = excluded.summary,
    source_session_id = excluded.source_session_id,
    source_turn_id = excluded.source_turn_id,
    updated_at = excluded.updated_at`,
		state.WorkspaceRoot,
		state.RoleID,
		state.Summary,
		state.SourceSessionID,
		state.SourceTurnID,
		timeString(state.CreatedAt),
		timeString(state.UpdatedAt),
	)
	return err
}

func (r *roleRuntimeStateRepo) Delete(ctx context.Context, workspaceRoot, roleID string) error {
	_, err := r.execer().Exec(
		`DELETE FROM v2_role_runtime_states WHERE workspace_root = ? AND role_id = ?`,
		strings.TrimSpace(workspaceRoot),
		strings.TrimSpace(roleID),
	)
	return err
}
