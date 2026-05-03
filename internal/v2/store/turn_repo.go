package store

import (
	"context"
	"database/sql"
	"go-agent/internal/v2/contracts"
)

type turnRepo struct {
	db *sql.DB
	tx *sql.Tx
}

var _ contracts.TurnRepository = (*turnRepo)(nil)

func (r *turnRepo) execer() execer { return repoExec(r.db, r.tx) }

func (r *turnRepo) Create(ctx context.Context, t contracts.Turn) error {
	_, err := r.execer().Exec(`
INSERT INTO v2_turns (id, session_id, kind, state, user_message, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?)`,
		t.ID, t.SessionID, t.Kind, t.State, t.UserMessage, timeString(t.CreatedAt), timeString(t.UpdatedAt))
	return err
}

func (r *turnRepo) Get(ctx context.Context, id string) (contracts.Turn, error) {
	return scanTurn(r.execer().QueryRow(`
SELECT id, session_id, kind, state, user_message, created_at, updated_at
FROM v2_turns WHERE id = ?`, id))
}

func (r *turnRepo) UpdateState(ctx context.Context, id, state string) error {
	_, err := r.execer().Exec(`UPDATE v2_turns SET state = ?, updated_at = ? WHERE id = ?`, state, timeString(sqlNow()), id)
	return err
}

func (r *turnRepo) ListBySession(ctx context.Context, sessionID string) ([]contracts.Turn, error) {
	rows, err := r.execer().Query(`
SELECT id, session_id, kind, state, user_message, created_at, updated_at
FROM v2_turns WHERE session_id = ? ORDER BY created_at ASC`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTurnList(rows)
}

func (r *turnRepo) ListRunning(ctx context.Context) ([]contracts.Turn, error) {
	rows, err := r.execer().Query(`
SELECT id, session_id, kind, state, user_message, created_at, updated_at
FROM v2_turns WHERE state = 'running' ORDER BY created_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTurnList(rows)
}

func scanTurn(row interface {
	Scan(dest ...any) error
}) (contracts.Turn, error) {
	var t contracts.Turn
	var createdAt, updatedAt string
	err := row.Scan(&t.ID, &t.SessionID, &t.Kind, &t.State, &t.UserMessage, &createdAt, &updatedAt)
	if err != nil {
		return contracts.Turn{}, err
	}
	t.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return contracts.Turn{}, err
	}
	t.UpdatedAt, err = parseTime(updatedAt)
	if err != nil {
		return contracts.Turn{}, err
	}
	return t, nil
}

func scanTurnList(rows *sql.Rows) ([]contracts.Turn, error) {
	var turns []contracts.Turn
	for rows.Next() {
		t, err := scanTurn(rows)
		if err != nil {
			return nil, err
		}
		turns = append(turns, t)
	}
	return turns, rows.Err()
}
