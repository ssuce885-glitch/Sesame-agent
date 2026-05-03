package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"go-agent/internal/v2/contracts"
)

type projectStateRepo struct {
	db *sql.DB
	tx *sql.Tx
}

var _ contracts.ProjectStateRepository = (*projectStateRepo)(nil)

func (r *projectStateRepo) execer() execer { return repoExec(r.db, r.tx) }

func (r *projectStateRepo) Get(ctx context.Context, workspaceRoot string) (contracts.ProjectState, bool, error) {
	workspaceRoot = strings.TrimSpace(workspaceRoot)
	if workspaceRoot == "" {
		return contracts.ProjectState{}, false, nil
	}
	var state contracts.ProjectState
	var createdAt, updatedAt string
	err := r.execer().QueryRow(`
SELECT workspace_root, summary, source_session_id, source_turn_id, created_at, updated_at
FROM v2_project_state
WHERE workspace_root = ?`, workspaceRoot).Scan(
		&state.WorkspaceRoot,
		&state.Summary,
		&state.SourceSessionID,
		&state.SourceTurnID,
		&createdAt,
		&updatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return contracts.ProjectState{}, false, nil
	}
	if err != nil {
		return contracts.ProjectState{}, false, err
	}
	var parseErr error
	state.CreatedAt, parseErr = parseTime(createdAt)
	if parseErr != nil {
		return contracts.ProjectState{}, false, parseErr
	}
	state.UpdatedAt, parseErr = parseTime(updatedAt)
	if parseErr != nil {
		return contracts.ProjectState{}, false, parseErr
	}
	return state, true, nil
}

func (r *projectStateRepo) Upsert(ctx context.Context, state contracts.ProjectState) error {
	state.WorkspaceRoot = strings.TrimSpace(state.WorkspaceRoot)
	if state.WorkspaceRoot == "" {
		return nil
	}
	if state.CreatedAt.IsZero() {
		state.CreatedAt = sqlNow()
	}
	if state.UpdatedAt.IsZero() {
		state.UpdatedAt = sqlNow()
	}
	_, err := r.execer().Exec(`
INSERT INTO v2_project_state (workspace_root, summary, source_session_id, source_turn_id, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT(workspace_root) DO UPDATE SET
    summary = excluded.summary,
    source_session_id = excluded.source_session_id,
    source_turn_id = excluded.source_turn_id,
    updated_at = excluded.updated_at`,
		state.WorkspaceRoot,
		state.Summary,
		state.SourceSessionID,
		state.SourceTurnID,
		timeString(state.CreatedAt),
		timeString(state.UpdatedAt),
	)
	return err
}

func (r *projectStateRepo) Delete(ctx context.Context, workspaceRoot string) error {
	_, err := r.execer().Exec(`DELETE FROM v2_project_state WHERE workspace_root = ?`, strings.TrimSpace(workspaceRoot))
	return err
}
