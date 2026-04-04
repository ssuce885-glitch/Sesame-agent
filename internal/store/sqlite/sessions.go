package sqlite

import (
	"context"
	"time"

	"go-agent/internal/types"
)

const timeLayout = time.RFC3339Nano

func (s *Store) InsertSession(ctx context.Context, session types.Session) error {
	_, err := s.db.ExecContext(ctx, `
		insert into sessions (id, workspace_root, state, active_turn_id, created_at, updated_at)
		values (?, ?, ?, ?, ?, ?)`,
		session.ID,
		session.WorkspaceRoot,
		session.State,
		session.ActiveTurnID,
		session.CreatedAt.Format(timeLayout),
		session.UpdatedAt.Format(timeLayout),
	)

	return err
}

func (s *Store) InsertTurn(ctx context.Context, turn types.Turn) error {
	_, err := s.db.ExecContext(ctx, `
		insert into turns (id, session_id, client_turn_id, state, user_message, created_at, updated_at)
		values (?, ?, ?, ?, ?, ?, ?)`,
		turn.ID,
		turn.SessionID,
		turn.ClientTurnID,
		turn.State,
		turn.UserMessage,
		turn.CreatedAt.Format(timeLayout),
		turn.UpdatedAt.Format(timeLayout),
	)

	return err
}

func (s *Store) ListSessions(ctx context.Context) ([]types.Session, error) {
	rows, err := s.db.QueryContext(ctx, `
		select id, workspace_root, state, active_turn_id, created_at, updated_at
		from sessions
		order by updated_at desc, created_at desc
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []types.Session
	for rows.Next() {
		var session types.Session
		var state string
		var createdAt string
		var updatedAt string
		if err := rows.Scan(&session.ID, &session.WorkspaceRoot, &state, &session.ActiveTurnID, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		session.State = types.SessionState(state)
		session.CreatedAt, err = time.Parse(timeLayout, createdAt)
		if err != nil {
			return nil, err
		}
		session.UpdatedAt, err = time.Parse(timeLayout, updatedAt)
		if err != nil {
			return nil, err
		}
		out = append(out, session)
	}

	return out, rows.Err()
}

func (s *Store) DeleteTurn(ctx context.Context, turnID string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	if _, err := tx.ExecContext(ctx, `
		delete from conversation_items
		where turn_id = ?`,
		turnID,
	); err != nil {
		return err
	}

	if _, err := tx.ExecContext(ctx, `
		delete from turns
		where id = ?`,
		turnID,
	); err != nil {
		return err
	}

	return tx.Commit()
}

func (s *Store) ListRunningTurns(ctx context.Context) ([]types.Turn, error) {
	rows, err := s.db.QueryContext(ctx, `
		select id, session_id, client_turn_id, state, user_message, created_at, updated_at
		from turns
		where state in (?, ?, ?, ?, ?, ?)
		order by created_at asc
	`,
		types.TurnStateBuildingContext,
		types.TurnStateModelStreaming,
		types.TurnStateToolDispatching,
		types.TurnStateAwaitingPermission,
		types.TurnStateToolRunning,
		types.TurnStateLoopContinue,
	)
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

func (s *Store) MarkTurnInterrupted(ctx context.Context, turnID string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	now := time.Now().UTC().Format(timeLayout)
	if _, err := tx.ExecContext(ctx, `
		update turns
		set state = ?, updated_at = ?
		where id = ?`,
		types.TurnStateInterrupted,
		now,
		turnID,
	); err != nil {
		return err
	}

	if _, err := tx.ExecContext(ctx, `
		update sessions
		set state = ?, active_turn_id = '', updated_at = ?
		where active_turn_id = ?`,
		types.SessionStateIdle,
		now,
		turnID,
	); err != nil {
		return err
	}

	return tx.Commit()
}
