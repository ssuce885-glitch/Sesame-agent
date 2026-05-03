package store

import (
	"context"
	"database/sql"
	"go-agent/internal/v2/contracts"
)

type sessionRepo struct {
	db *sql.DB
	tx *sql.Tx
}

var _ contracts.SessionRepository = (*sessionRepo)(nil)

func (r *sessionRepo) execer() execer { return repoExec(r.db, r.tx) }

func (r *sessionRepo) Create(ctx context.Context, s contracts.Session) error {
	_, err := r.execer().Exec(`
INSERT INTO v2_sessions (id, workspace_root, system_prompt, permission_profile, state, active_turn_id, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		s.ID, s.WorkspaceRoot, s.SystemPrompt, s.PermissionProfile, s.State, s.ActiveTurnID, timeString(s.CreatedAt), timeString(s.UpdatedAt))
	return err
}

func (r *sessionRepo) Get(ctx context.Context, id string) (contracts.Session, error) {
	return scanSession(r.execer().QueryRow(`
SELECT id, workspace_root, system_prompt, permission_profile, state, active_turn_id, created_at, updated_at
FROM v2_sessions WHERE id = ?`, id))
}

func (r *sessionRepo) UpdateState(ctx context.Context, id, state string) error {
	_, err := r.execer().Exec(`UPDATE v2_sessions SET state = ?, updated_at = ? WHERE id = ?`, state, timeString(sqlNow()), id)
	return err
}

func (r *sessionRepo) SetActiveTurn(ctx context.Context, id, turnID string) error {
	_, err := r.execer().Exec(`UPDATE v2_sessions SET active_turn_id = ?, state = 'running', updated_at = ? WHERE id = ?`, turnID, timeString(sqlNow()), id)
	return err
}

func (r *sessionRepo) ListByWorkspace(ctx context.Context, workspaceRoot string) ([]contracts.Session, error) {
	rows, err := r.execer().Query(`
SELECT id, workspace_root, system_prompt, permission_profile, state, active_turn_id, created_at, updated_at
FROM v2_sessions WHERE workspace_root = ? ORDER BY created_at ASC`, workspaceRoot)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []contracts.Session
	for rows.Next() {
		s, err := scanSessionRows(rows)
		if err != nil {
			return nil, err
		}
		sessions = append(sessions, s)
	}
	return sessions, rows.Err()
}

func scanSession(row interface {
	Scan(dest ...any) error
}) (contracts.Session, error) {
	var s contracts.Session
	var createdAt, updatedAt string
	err := row.Scan(&s.ID, &s.WorkspaceRoot, &s.SystemPrompt, &s.PermissionProfile, &s.State, &s.ActiveTurnID, &createdAt, &updatedAt)
	if err != nil {
		return contracts.Session{}, err
	}
	s.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return contracts.Session{}, err
	}
	s.UpdatedAt, err = parseTime(updatedAt)
	if err != nil {
		return contracts.Session{}, err
	}
	return s, nil
}

func scanSessionRows(rows *sql.Rows) (contracts.Session, error) {
	return scanSession(rows)
}
