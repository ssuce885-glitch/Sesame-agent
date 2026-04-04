package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"go-agent/internal/types"
)

func (s *Store) GetSession(ctx context.Context, sessionID string) (types.Session, bool, error) {
	row := s.db.QueryRowContext(ctx, `
		select id, workspace_root, state, active_turn_id, created_at, updated_at
		from sessions
		where id = ?
	`, sessionID)

	var session types.Session
	var state string
	var createdAt string
	var updatedAt string
	err := row.Scan(&session.ID, &session.WorkspaceRoot, &state, &session.ActiveTurnID, &createdAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return types.Session{}, false, nil
	}
	if err != nil {
		return types.Session{}, false, err
	}

	session.State = types.SessionState(state)
	session.CreatedAt, err = time.Parse(timeLayout, createdAt)
	if err != nil {
		return types.Session{}, false, err
	}
	session.UpdatedAt, err = time.Parse(timeLayout, updatedAt)
	if err != nil {
		return types.Session{}, false, err
	}

	return session, true, nil
}

func (s *Store) ListTurnsBySession(ctx context.Context, sessionID string) ([]types.Turn, error) {
	rows, err := s.db.QueryContext(ctx, `
		select id, session_id, client_turn_id, state, user_message, created_at, updated_at
		from turns
		where session_id = ?
		order by created_at asc, id asc
	`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []types.Turn
	for rows.Next() {
		var turn types.Turn
		var state string
		var createdAt string
		var updatedAt string
		if err := rows.Scan(&turn.ID, &turn.SessionID, &turn.ClientTurnID, &state, &turn.UserMessage, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		turn.State = types.TurnState(state)
		turn.CreatedAt, err = time.Parse(timeLayout, createdAt)
		if err != nil {
			return nil, err
		}
		turn.UpdatedAt, err = time.Parse(timeLayout, updatedAt)
		if err != nil {
			return nil, err
		}
		out = append(out, turn)
	}

	return out, rows.Err()
}

func (s *Store) LatestSessionEventSeq(ctx context.Context, sessionID string) (int64, error) {
	var seq int64
	err := s.db.QueryRowContext(ctx, `
		select coalesce(max(seq), 0)
		from events
		where session_id = ?
	`, sessionID).Scan(&seq)
	return seq, err
}
