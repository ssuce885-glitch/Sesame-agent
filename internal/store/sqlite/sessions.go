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

func (s *Store) UpdateSessionPermissionProfile(ctx context.Context, sessionID, permissionProfile string) (types.Session, bool, error) {
	now := time.Now().UTC().Format(timeLayout)
	result, err := s.db.ExecContext(ctx, `
		update sessions
		set permission_profile = ?, updated_at = ?
		where id = ?`,
		permissionProfile,
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

func (s *Store) UpdateSessionState(ctx context.Context, sessionID string, state types.SessionState, activeTurnID string) error {
	_, err := s.db.ExecContext(ctx, `
		update sessions
		set state = ?, active_turn_id = ?, updated_at = ?
		where id = ?`,
		state,
		activeTurnID,
		time.Now().UTC().Format(timeLayout),
		sessionID,
	)
	return err
}

func (s *Store) DeleteSession(ctx context.Context, sessionID string) (string, bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", false, err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	var exists int
	if err := tx.QueryRowContext(ctx, `
		select count(*)
		from sessions
		where id = ?
	`, sessionID).Scan(&exists); err != nil {
		return "", false, err
	}
	if exists == 0 {
		return "", false, nil
	}

	selectedSessionID, hasSelected, err := getSelectedSessionIDWithQueryer(ctx, tx)
	if err != nil {
		return "", false, err
	}

	deleteStatements := []struct {
		query string
		args  []any
	}{
		{
			query: `delete from tool_runs where run_id in (select id from runs where session_id = ?)`,
			args:  []any{sessionID},
		},
		{
			query: `delete from worktrees where run_id in (select id from runs where session_id = ?)`,
			args:  []any{sessionID},
		},
		{
			query: `delete from task_records where run_id in (select id from runs where session_id = ?)`,
			args:  []any{sessionID},
		},
		{
			query: `delete from plans where run_id in (select id from runs where session_id = ?)`,
			args:  []any{sessionID},
		},
		{
			query: `delete from report_mailbox_items where session_id = ?`,
			args:  []any{sessionID},
		},
		{
			query: `delete from scheduled_jobs where owner_session_id = ?`,
			args:  []any{sessionID},
		},
		{
			query: `delete from pending_task_completions where session_id = ?`,
			args:  []any{sessionID},
		},
		{
			query: `delete from provider_cache_entries where session_id = ?`,
			args:  []any{sessionID},
		},
		{
			query: `delete from provider_cache_heads where session_id = ?`,
			args:  []any{sessionID},
		},
		{
			query: `delete from conversation_compactions where session_id = ?`,
			args:  []any{sessionID},
		},
		{
			query: `delete from conversation_summaries where session_id = ?`,
			args:  []any{sessionID},
		},
		{
			query: `delete from turn_usage where session_id = ?`,
			args:  []any{sessionID},
		},
		{
			query: `delete from conversation_items where session_id = ?`,
			args:  []any{sessionID},
		},
		{
			query: `delete from events where session_id = ?`,
			args:  []any{sessionID},
		},
		{
			query: `delete from turns where session_id = ?`,
			args:  []any{sessionID},
		},
		{
			query: `delete from runs where session_id = ?`,
			args:  []any{sessionID},
		},
		{
			query: `delete from sessions where id = ?`,
			args:  []any{sessionID},
		},
	}
	for _, stmt := range deleteStatements {
		if _, err := tx.ExecContext(ctx, stmt.query, stmt.args...); err != nil {
			return "", false, err
		}
	}

	nextSelected := ""
	if hasSelected && selectedSessionID != sessionID {
		var remaining int
		if err := tx.QueryRowContext(ctx, `
			select count(*)
			from sessions
			where id = ?
		`, selectedSessionID).Scan(&remaining); err != nil {
			return "", false, err
		}
		if remaining > 0 {
			nextSelected = selectedSessionID
		}
	}

	if nextSelected == "" {
		var candidate string
		err := tx.QueryRowContext(ctx, `
			select id
			from sessions
			order by updated_at desc, created_at desc
			limit 1
		`).Scan(&candidate)
		if err != nil && err != sql.ErrNoRows {
			return "", false, err
		}
		if err == nil {
			nextSelected = candidate
		}
	}

	if nextSelected != "" {
		if err := setSelectedSessionIDWithExec(ctx, tx, nextSelected); err != nil {
			return "", false, err
		}
	} else {
		if err := clearSelectedSessionIDWithExec(ctx, tx); err != nil {
			return "", false, err
		}
	}

	if err := tx.Commit(); err != nil {
		return "", false, err
	}
	return nextSelected, true, nil
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
	var state string
	var createdAt string
	var updatedAt string
	err := s.db.QueryRowContext(ctx, `
		select id, session_id, client_turn_id, state, user_message, created_at, updated_at
		from turns
		where id = ?`,
		turnID,
	).Scan(&turn.ID, &turn.SessionID, &turn.ClientTurnID, &state, &turn.UserMessage, &createdAt, &updatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return types.Turn{}, false, nil
		}
		return types.Turn{}, false, err
	}
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
	_, err := s.db.ExecContext(ctx, `
		update turns
		set state = ?, updated_at = ?
		where id = ?`,
		state,
		time.Now().UTC().Format(timeLayout),
		turnID,
	)
	return err
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
