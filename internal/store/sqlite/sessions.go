package sqlite

import (
	"context"
	"database/sql"
	"time"

	"go-agent/internal/types"
)

const timeLayout = time.RFC3339Nano

func (s *Store) InsertSession(ctx context.Context, session types.Session) error {
	_, err := s.db.ExecContext(ctx, `
		insert into sessions (id, workspace_root, system_prompt, state, active_turn_id, created_at, updated_at)
		values (?, ?, ?, ?, ?, ?, ?)`,
		session.ID,
		session.WorkspaceRoot,
		session.SystemPrompt,
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
		select id, workspace_root, system_prompt, state, active_turn_id, created_at, updated_at
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
		if err := rows.Scan(&session.ID, &session.WorkspaceRoot, &session.SystemPrompt, &state, &session.ActiveTurnID, &createdAt, &updatedAt); err != nil {
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

func (s *Store) GetSession(ctx context.Context, sessionID string) (types.Session, bool, error) {
	var session types.Session
	var state string
	var createdAt string
	var updatedAt string
	err := s.db.QueryRowContext(ctx, `
		select id, workspace_root, system_prompt, state, active_turn_id, created_at, updated_at
		from sessions
		where id = ?`,
		sessionID,
	).Scan(&session.ID, &session.WorkspaceRoot, &session.SystemPrompt, &state, &session.ActiveTurnID, &createdAt, &updatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return types.Session{}, false, nil
		}
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

func (s *Store) UpdateSessionSystemPrompt(ctx context.Context, sessionID, systemPrompt string) (types.Session, bool, error) {
	now := time.Now().UTC().Format(timeLayout)
	result, err := s.db.ExecContext(ctx, `
		update sessions
		set system_prompt = ?, updated_at = ?
		where id = ?`,
		systemPrompt,
		now,
		sessionID,
	)
	if err != nil {
		return types.Session{}, false, err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return types.Session{}, false, err
	}
	if rowsAffected == 0 {
		return types.Session{}, false, nil
	}
	return s.GetSession(ctx, sessionID)
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
