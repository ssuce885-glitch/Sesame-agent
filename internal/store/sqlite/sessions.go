package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"go-agent/internal/types"
)

const timeLayout = time.RFC3339Nano

func (s *Store) InsertSession(ctx context.Context, session types.Session) error {
	_, err := s.db.ExecContext(ctx, `
		insert into sessions (id, workspace_root, system_prompt, permission_profile, state, active_turn_id, created_at, updated_at)
		values (?, ?, ?, ?, ?, ?, ?, ?)`,
		session.ID,
		session.WorkspaceRoot,
		session.SystemPrompt,
		session.PermissionProfile,
		session.State,
		session.ActiveTurnID,
		session.CreatedAt.Format(timeLayout),
		session.UpdatedAt.Format(timeLayout),
	)

	return err
}

func (s *Store) InsertTurn(ctx context.Context, turn types.Turn) error {
	_, err := s.db.ExecContext(ctx, `
		insert into turns (id, session_id, context_head_id, turn_kind, client_turn_id, state, user_message, created_at, updated_at)
		values (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		turn.ID,
		turn.SessionID,
		turn.ContextHeadID,
		turn.Kind,
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
		select id, workspace_root, system_prompt, permission_profile, state, active_turn_id, created_at, updated_at
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
		if err := rows.Scan(&session.ID, &session.WorkspaceRoot, &session.SystemPrompt, &session.PermissionProfile, &state, &session.ActiveTurnID, &createdAt, &updatedAt); err != nil {
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
		select id, workspace_root, system_prompt, permission_profile, state, active_turn_id, created_at, updated_at
		from sessions
		where id = ?`,
		sessionID,
	).Scan(&session.ID, &session.WorkspaceRoot, &session.SystemPrompt, &session.PermissionProfile, &state, &session.ActiveTurnID, &createdAt, &updatedAt)
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

func (s *Store) UpdateSessionPermissionProfile(ctx context.Context, sessionID, permissionProfile string) (types.Session, bool, error) {
	if err := updateSessionPermissionProfileWithExec(ctx, s.db, sessionID, permissionProfile, true); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return types.Session{}, false, nil
		}
		return types.Session{}, false, err
	}
	return s.GetSession(ctx, sessionID)
}

func updateSessionPermissionProfileWithExec(ctx context.Context, execer execContexter, sessionID, permissionProfile string, requireExisting bool) error {
	result, err := execer.ExecContext(ctx, `
		update sessions
		set permission_profile = ?, updated_at = ?
		where id = ?`,
		permissionProfile,
		time.Now().UTC().Format(timeLayout),
		sessionID,
	)
	if err != nil {
		return err
	}
	if !requireExisting {
		return nil
	}
	return requireSingleRow(result)
}

func (s *Store) UpdateSessionState(ctx context.Context, sessionID string, state types.SessionState, activeTurnID string) error {
	return updateSessionStateWithExec(ctx, s.db, sessionID, state, activeTurnID, false)
}

func updateSessionStateWithExec(ctx context.Context, execer execContexter, sessionID string, state types.SessionState, activeTurnID string, requireExisting bool) error {
	result, err := execer.ExecContext(ctx, `
		update sessions
		set state = ?, active_turn_id = ?, updated_at = ?
		where id = ?`,
		state,
		activeTurnID,
		time.Now().UTC().Format(timeLayout),
		sessionID,
	)
	if err != nil {
		return err
	}
	if !requireExisting {
		return nil
	}
	return requireSingleRow(result)
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

func (s *Store) GetTurn(ctx context.Context, turnID string) (types.Turn, bool, error) {
	var turn types.Turn
	var kind string
	var state string
	var createdAt string
	var updatedAt string
	err := s.db.QueryRowContext(ctx, `
		select id, session_id, context_head_id, turn_kind, client_turn_id, state, user_message, created_at, updated_at
		from turns
		where id = ?`,
		turnID,
	).Scan(&turn.ID, &turn.SessionID, &turn.ContextHeadID, &kind, &turn.ClientTurnID, &state, &turn.UserMessage, &createdAt, &updatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return types.Turn{}, false, nil
		}
		return types.Turn{}, false, err
	}
	turn.Kind = types.TurnKind(kind)
	turn.State = types.TurnState(state)
	turn.CreatedAt, err = time.Parse(timeLayout, createdAt)
	if err != nil {
		return types.Turn{}, false, err
	}
	turn.UpdatedAt, err = time.Parse(timeLayout, updatedAt)
	if err != nil {
		return types.Turn{}, false, err
	}
	return turn, true, nil
}

func (s *Store) UpdateTurnState(ctx context.Context, turnID string, state types.TurnState) error {
	return updateTurnStateWithExec(ctx, s.db, turnID, state, false)
}

func updateTurnStateWithExec(ctx context.Context, execer execContexter, turnID string, state types.TurnState, requireExisting bool) error {
	result, err := execer.ExecContext(ctx, `
		update turns
		set state = ?, updated_at = ?
		where id = ?`,
		state,
		time.Now().UTC().Format(timeLayout),
		turnID,
	)
	if err != nil {
		return err
	}
	if !requireExisting {
		return nil
	}
	return requireSingleRow(result)
}

func (s *Store) ListRunningTurns(ctx context.Context) ([]types.Turn, error) {
	rows, err := s.db.QueryContext(ctx, `
		select id, session_id, context_head_id, turn_kind, client_turn_id, state, user_message, created_at, updated_at
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
		var kind string
		var state string
		var createdAt string
		var updatedAt string
		if err := rows.Scan(&turn.ID, &turn.SessionID, &turn.ContextHeadID, &kind, &turn.ClientTurnID, &state, &turn.UserMessage, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		turn.Kind = types.TurnKind(kind)
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

func (s *Store) TryMarkTurnInterrupted(ctx context.Context, sessionID, turnID string) (bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	var activeTurnID string
	if err := tx.QueryRowContext(ctx, `
		select active_turn_id
		from sessions
		where id = ?`,
		sessionID,
	).Scan(&activeTurnID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	if activeTurnID != turnID {
		return false, nil
	}

	var turnState string
	if err := tx.QueryRowContext(ctx, `
		select state
		from turns
		where id = ? and session_id = ?`,
		turnID,
		sessionID,
	).Scan(&turnState); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	if isTerminalTurnState(types.TurnState(turnState)) {
		return false, nil
	}

	if err := updateTurnStateWithExec(ctx, tx, turnID, types.TurnStateInterrupted, true); err != nil {
		return false, err
	}
	result, err := tx.ExecContext(ctx, `
		update sessions
		set state = ?, active_turn_id = '', updated_at = ?
		where id = ? and active_turn_id = ?`,
		types.SessionStateIdle,
		time.Now().UTC().Format(timeLayout),
		sessionID,
		turnID,
	)
	if err != nil {
		return false, err
	}
	if err := requireSingleRow(result); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}

	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

func requireSingleRow(result sql.Result) error {
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return sql.ErrNoRows
	}
	return nil
}

func isTerminalTurnState(state types.TurnState) bool {
	return state == types.TurnStateCompleted || state == types.TurnStateFailed || state == types.TurnStateInterrupted
}
