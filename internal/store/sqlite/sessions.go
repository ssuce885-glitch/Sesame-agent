package sqlite

import (
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"strings"
	"time"

	rolectx "go-agent/internal/roles"
	"go-agent/internal/sessionrole"
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

func (s *Store) EnsureRoleSession(ctx context.Context, workspaceRoot string, role types.SessionRole) (types.Session, types.ContextHead, bool, error) {
	role = sessionrole.Normalize(string(role))
	roleCtx := rolectx.WithSpecialistRoleID(sessionrole.WithSessionRole(ctx, role), "")
	if role == types.SessionRoleMainParent {
		session, head, created, err := s.EnsureCanonicalSession(roleCtx, workspaceRoot)
		if err != nil {
			return types.Session{}, types.ContextHead{}, false, err
		}
		session, err = s.ensureRoleSystemPrompt(roleCtx, session, role)
		if err != nil {
			return types.Session{}, types.ContextHead{}, false, err
		}
		if err := s.SetRoleSessionID(ctx, workspaceRoot, role, session.ID); err != nil {
			return types.Session{}, types.ContextHead{}, false, err
		}
		return session, head, created, nil
	}

	if sessionID, ok, err := s.GetRoleSessionID(ctx, workspaceRoot, role); err != nil {
		return types.Session{}, types.ContextHead{}, false, err
	} else if ok {
		session, found, err := s.GetSession(ctx, sessionID)
		if err != nil {
			return types.Session{}, types.ContextHead{}, false, err
		}
		if found && session.WorkspaceRoot == workspaceRoot {
			session, err = s.ensureRoleSystemPrompt(roleCtx, session, role)
			if err != nil {
				return types.Session{}, types.ContextHead{}, false, err
			}
			head, _, err := s.ensureCurrentContextHead(roleCtx, session)
			return session, head, false, err
		}
	}

	now := time.Now().UTC()
	session := types.Session{
		ID:            types.NewID("sess"),
		WorkspaceRoot: workspaceRoot,
		SystemPrompt:  sessionrole.DefaultSystemPrompt(role),
		State:         types.SessionStateIdle,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := s.InsertSession(ctx, session); err != nil {
		return types.Session{}, types.ContextHead{}, false, err
	}
	if err := s.SetRoleSessionID(ctx, workspaceRoot, role, session.ID); err != nil {
		return types.Session{}, types.ContextHead{}, false, err
	}
	head, _, err := s.ensureCurrentContextHead(roleCtx, session)
	if err != nil {
		return types.Session{}, types.ContextHead{}, false, err
	}
	return session, head, true, nil
}

func (s *Store) EnsureSpecialistSession(ctx context.Context, workspaceRoot, roleID, systemPrompt string, skillNames []string) (types.Session, types.ContextHead, bool, error) {
	workspaceRoot = strings.TrimSpace(workspaceRoot)
	roleID = normalizeSpecialistRoleID(roleID)
	if workspaceRoot == "" {
		return types.Session{}, types.ContextHead{}, false, errors.New("workspace root is required")
	}
	if roleID == "" {
		return types.Session{}, types.ContextHead{}, false, errors.New("specialist role id is required")
	}
	_ = skillNames

	specialistCtx := rolectx.WithSpecialistRoleID(sessionrole.WithSessionRole(ctx, types.SessionRoleMainParent), roleID)

	if sessionID, ok, err := s.GetSpecialistSessionID(ctx, workspaceRoot, roleID); err != nil {
		return types.Session{}, types.ContextHead{}, false, err
	} else if ok {
		session, found, err := s.GetSession(ctx, sessionID)
		if err != nil {
			return types.Session{}, types.ContextHead{}, false, err
		}
		if found && session.WorkspaceRoot == workspaceRoot {
			session, err = s.ensureSpecialistSystemPrompt(specialistCtx, session, roleID, systemPrompt)
			if err != nil {
				return types.Session{}, types.ContextHead{}, false, err
			}
			head, _, err := s.ensureCurrentContextHead(specialistCtx, session)
			return session, head, false, err
		}
	}

	prompt := strings.TrimSpace(systemPrompt)
	if prompt == "" {
		return types.Session{}, types.ContextHead{}, false, errors.New("specialist system prompt is required")
	}

	now := time.Now().UTC()
	session := types.Session{
		ID:            types.NewID("sess"),
		WorkspaceRoot: workspaceRoot,
		SystemPrompt:  prompt,
		State:         types.SessionStateIdle,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := s.InsertSession(ctx, session); err != nil {
		return types.Session{}, types.ContextHead{}, false, err
	}
	if err := s.SetSpecialistSessionID(ctx, workspaceRoot, roleID, session.ID); err != nil {
		return types.Session{}, types.ContextHead{}, false, err
	}
	head, _, err := s.ensureCurrentContextHead(specialistCtx, session)
	if err != nil {
		return types.Session{}, types.ContextHead{}, false, err
	}
	return session, head, true, nil
}

func (s *Store) ResolveSpecialistRoleID(ctx context.Context, sessionID, workspaceRoot string) (string, error) {
	sessionID = strings.TrimSpace(sessionID)
	workspaceRoot = strings.TrimSpace(workspaceRoot)
	if sessionID == "" || workspaceRoot == "" {
		return "", nil
	}

	session, found, err := s.GetSession(ctx, sessionID)
	if err != nil {
		return "", err
	}
	if !found || session.WorkspaceRoot != workspaceRoot {
		return "", nil
	}

	rows, err := s.db.QueryContext(ctx, `
		select key, value
		from runtime_metadata
		where substr(key, 1, length(?)) = ?
	`, specialistSessionMetadataKey(workspaceRoot, ""), specialistSessionMetadataKey(workspaceRoot, ""))
	if err != nil {
		return "", err
	}
	defer rows.Close()

	for rows.Next() {
		var metadataKey string
		var mappedSessionID string
		if err := rows.Scan(&metadataKey, &mappedSessionID); err != nil {
			return "", err
		}
		if strings.TrimSpace(mappedSessionID) != sessionID {
			continue
		}
		roleID, ok := specialistRoleIDFromMetadataKey(metadataKey)
		if !ok {
			continue
		}
		return roleID, nil
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	return "", nil
}

func (s *Store) ResolveSessionRole(ctx context.Context, sessionID, workspaceRoot string) (types.SessionRole, error) {
	sessionID = strings.TrimSpace(sessionID)
	workspaceRoot = strings.TrimSpace(workspaceRoot)
	if sessionID == "" || workspaceRoot == "" {
		return "", nil
	}

	for _, role := range []types.SessionRole{types.SessionRoleMonitoringParent, types.SessionRoleMainParent} {
		mappedID, ok, err := s.GetRoleSessionID(ctx, workspaceRoot, role)
		if err != nil {
			return "", err
		}
		if !ok || strings.TrimSpace(mappedID) != sessionID {
			continue
		}
		session, found, err := s.GetSession(ctx, sessionID)
		if err != nil {
			return "", err
		}
		if found && session.WorkspaceRoot == workspaceRoot {
			return role, nil
		}
	}
	return "", nil
}

func (s *Store) EnsureCanonicalSession(ctx context.Context, workspaceRoot string) (types.Session, types.ContextHead, bool, error) {
	if sessionID, ok, err := s.GetCanonicalSessionID(ctx); err != nil {
		return types.Session{}, types.ContextHead{}, false, err
	} else if ok {
		session, found, err := s.GetSession(ctx, sessionID)
		if err != nil {
			return types.Session{}, types.ContextHead{}, false, err
		}
		if found && session.WorkspaceRoot == workspaceRoot {
			head, _, err := s.ensureCurrentContextHead(ctx, session)
			return session, head, false, err
		}
		// The stored canonical session may be stale for a different workspace.
		// Fall through and resolve/create the canonical session for workspaceRoot.
	}

	session, created, err := s.resolveOrCreateCanonicalSession(ctx, workspaceRoot)
	if err != nil {
		return types.Session{}, types.ContextHead{}, false, err
	}
	if err := s.SetCanonicalSessionID(ctx, session.ID); err != nil {
		return types.Session{}, types.ContextHead{}, false, err
	}
	head, _, err := s.ensureCurrentContextHead(ctx, session)
	if err != nil {
		return types.Session{}, types.ContextHead{}, false, err
	}
	return session, head, created, nil
}

func (s *Store) resolveOrCreateCanonicalSession(ctx context.Context, workspaceRoot string) (types.Session, bool, error) {
	var session types.Session
	var state string
	var createdAt string
	var updatedAt string
	err := s.db.QueryRowContext(ctx, `
		select id, workspace_root, system_prompt, permission_profile, state, active_turn_id, created_at, updated_at
		from sessions
		where workspace_root = ?
		order by updated_at desc, created_at desc
		limit 1
	`, workspaceRoot).Scan(
		&session.ID,
		&session.WorkspaceRoot,
		&session.SystemPrompt,
		&session.PermissionProfile,
		&state,
		&session.ActiveTurnID,
		&createdAt,
		&updatedAt,
	)
	if err == nil {
		session.State = types.SessionState(state)
		session.CreatedAt, err = time.Parse(timeLayout, createdAt)
		if err != nil {
			return types.Session{}, false, err
		}
		session.UpdatedAt, err = time.Parse(timeLayout, updatedAt)
		if err != nil {
			return types.Session{}, false, err
		}
		return session, false, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return types.Session{}, false, err
	}

	now := time.Now().UTC()
	session = types.Session{
		ID:            types.NewID("sess"),
		WorkspaceRoot: workspaceRoot,
		SystemPrompt:  sessionrole.DefaultSystemPrompt(sessionrole.FromContext(ctx)),
		State:         types.SessionStateIdle,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := s.InsertSession(ctx, session); err != nil {
		return types.Session{}, false, err
	}
	return session, true, nil
}

func (s *Store) ensureRoleSystemPrompt(ctx context.Context, session types.Session, role types.SessionRole) (types.Session, error) {
	if !sessionrole.ShouldRefreshDefaultSystemPrompt(role, session.SystemPrompt) {
		return session, nil
	}

	prompt := strings.TrimSpace(sessionrole.DefaultSystemPrompt(role))
	if prompt == "" {
		return session, nil
	}

	now := time.Now().UTC()
	result, err := s.db.ExecContext(ctx, `
		update sessions
		set system_prompt = ?, updated_at = ?
		where id = ?`,
		prompt,
		now.Format(timeLayout),
		session.ID,
	)
	if err != nil {
		return types.Session{}, err
	}
	if err := requireSingleRow(result); err != nil {
		return types.Session{}, err
	}
	session.SystemPrompt = prompt
	session.UpdatedAt = now
	return session, nil
}

func (s *Store) ensureSpecialistSystemPrompt(ctx context.Context, session types.Session, roleID, systemPrompt string) (types.Session, error) {
	_ = roleID
	prompt := strings.TrimSpace(systemPrompt)
	if prompt == "" || strings.TrimSpace(session.SystemPrompt) == prompt {
		return session, nil
	}

	now := time.Now().UTC()
	result, err := s.db.ExecContext(ctx, `
		update sessions
		set system_prompt = ?, updated_at = ?
		where id = ?`,
		prompt,
		now.Format(timeLayout),
		session.ID,
	)
	if err != nil {
		return types.Session{}, err
	}
	if err := requireSingleRow(result); err != nil {
		return types.Session{}, err
	}
	session.SystemPrompt = prompt
	session.UpdatedAt = now
	return session, nil
}

func specialistRoleIDFromMetadataKey(metadataKey string) (string, bool) {
	parts := strings.Split(metadataKey, ":")
	if len(parts) != 3 || parts[0] != "specialist_session" {
		return "", false
	}
	rawRoleID, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return "", false
	}
	roleID := normalizeSpecialistRoleID(string(rawRoleID))
	if roleID == "" {
		return "", false
	}
	return roleID, true
}

func (s *Store) ensureCurrentContextHead(ctx context.Context, session types.Session) (types.ContextHead, bool, error) {
	if headID, ok, err := s.GetCurrentContextHeadID(ctx); err != nil {
		return types.ContextHead{}, false, err
	} else if ok {
		head, found, err := s.GetContextHead(ctx, headID)
		if err != nil {
			return types.ContextHead{}, false, err
		}
		if found && head.SessionID == session.ID {
			if err := s.AssignTurnsWithoutHead(ctx, session.ID, head.ID); err != nil {
				return types.ContextHead{}, false, err
			}
			return head, false, nil
		}
	}

	now := time.Now().UTC()
	head := types.ContextHead{
		ID:         types.NewID("head"),
		SessionID:  session.ID,
		SourceKind: types.ContextHeadSourceBootstrap,
		Title:      deriveSessionTitle(ctx, s, session.ID),
		Preview:    deriveSessionPreview(ctx, s, session.ID),
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if err := s.InsertContextHead(ctx, head); err != nil {
		return types.ContextHead{}, false, err
	}
	if err := s.AssignTurnsWithoutHead(ctx, session.ID, head.ID); err != nil {
		return types.ContextHead{}, false, err
	}
	if err := s.SetCurrentContextHeadID(ctx, head.ID); err != nil {
		return types.ContextHead{}, false, err
	}
	return head, true, nil
}
