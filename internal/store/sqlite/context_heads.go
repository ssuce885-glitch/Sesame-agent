package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"go-agent/internal/types"
)

func (s *Store) InsertContextHead(ctx context.Context, head types.ContextHead) error {
	_, err := s.db.ExecContext(ctx, `
		insert into context_heads (
			id, session_id, parent_head_id, source_kind, title, preview, created_at, updated_at
		)
		values (?, ?, ?, ?, ?, ?, ?, ?)
	`,
		head.ID,
		head.SessionID,
		head.ParentHeadID,
		head.SourceKind,
		head.Title,
		head.Preview,
		head.CreatedAt.Format(timeLayout),
		head.UpdatedAt.Format(timeLayout),
	)
	return err
}

func (s *Store) GetContextHead(ctx context.Context, headID string) (types.ContextHead, bool, error) {
	var head types.ContextHead
	var sourceKind string
	var createdAt string
	var updatedAt string
	err := s.db.QueryRowContext(ctx, `
		select id, session_id, parent_head_id, source_kind, title, preview, created_at, updated_at
		from context_heads
		where id = ?
	`, headID).Scan(
		&head.ID,
		&head.SessionID,
		&head.ParentHeadID,
		&sourceKind,
		&head.Title,
		&head.Preview,
		&createdAt,
		&updatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return types.ContextHead{}, false, nil
	}
	if err != nil {
		return types.ContextHead{}, false, err
	}

	head.SourceKind = types.ContextHeadSourceKind(sourceKind)
	head.CreatedAt, err = time.Parse(timeLayout, createdAt)
	if err != nil {
		return types.ContextHead{}, false, err
	}
	head.UpdatedAt, err = time.Parse(timeLayout, updatedAt)
	if err != nil {
		return types.ContextHead{}, false, err
	}
	return head, true, nil
}

func (s *Store) AssignTurnsWithoutHead(ctx context.Context, sessionID, headID string) error {
	_, err := s.db.ExecContext(ctx, `
		update turns
		set context_head_id = ?
		where session_id = ? and context_head_id = ''
	`, headID, sessionID)
	return err
}

func (s *Store) ListTurnsByContextHead(ctx context.Context, sessionID, headID string) ([]types.Turn, error) {
	rows, err := s.db.QueryContext(ctx, `
		select id, session_id, context_head_id, client_turn_id, state, execution_mode, foreground_lease_id, foreground_lease_expires_at, user_message, created_at, updated_at
		from turns
		where session_id = ? and context_head_id = ?
		order by created_at asc, id asc
	`, sessionID, headID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]types.Turn, 0)
	for rows.Next() {
		turn, err := scanTurnRowWithContextHead(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, turn)
	}
	return out, rows.Err()
}

func (s *Store) ListContextHeadLineage(ctx context.Context, sessionID, headID string) ([]types.ContextHead, error) {
	ordered := []types.ContextHead{}
	currentID := strings.TrimSpace(headID)
	for currentID != "" {
		head, found, err := s.GetContextHead(ctx, currentID)
		if err != nil {
			return nil, err
		}
		if !found || head.SessionID != sessionID {
			return nil, errors.New("context head not found in session lineage")
		}
		ordered = append([]types.ContextHead{head}, ordered...)
		currentID = strings.TrimSpace(head.ParentHeadID)
	}
	return ordered, nil
}

func deriveSessionTitle(ctx context.Context, s *Store, sessionID string) string {
	turns, err := s.ListTurnsBySession(ctx, sessionID)
	if err != nil {
		return ""
	}
	for _, turn := range turns {
		if text := clampHeadPreview(turn.UserMessage); text != "" {
			return text
		}
	}
	return ""
}

func deriveSessionPreview(ctx context.Context, s *Store, sessionID string) string {
	turns, err := s.ListTurnsBySession(ctx, sessionID)
	if err != nil {
		return ""
	}
	preview := ""
	for _, turn := range turns {
		if text := clampHeadPreview(turn.UserMessage); text != "" {
			preview = text
		}
	}
	return preview
}

func clampHeadPreview(text string) string {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return ""
	}
	const maxLen = 120
	runes := []rune(trimmed)
	if len(runes) <= maxLen {
		return trimmed
	}
	return string(runes[:maxLen]) + "..."
}

func scanTurnRowWithContextHead(scanner interface {
	Scan(dest ...any) error
}) (types.Turn, error) {
	var turn types.Turn
	var state string
	var executionMode string
	var foregroundLeaseExpiresAt string
	var createdAt string
	var updatedAt string
	if err := scanner.Scan(
		&turn.ID,
		&turn.SessionID,
		&turn.ContextHeadID,
		&turn.ClientTurnID,
		&state,
		&executionMode,
		&turn.ForegroundLeaseID,
		&foregroundLeaseExpiresAt,
		&turn.UserMessage,
		&createdAt,
		&updatedAt,
	); err != nil {
		return types.Turn{}, err
	}

	turn.State = types.TurnState(state)
	turn.ExecutionMode = types.TurnExecutionMode(executionMode)
	var err error
	turn.CreatedAt, err = time.Parse(timeLayout, createdAt)
	if err != nil {
		return types.Turn{}, err
	}
	turn.UpdatedAt, err = time.Parse(timeLayout, updatedAt)
	if err != nil {
		return types.Turn{}, err
	}
	if strings.TrimSpace(foregroundLeaseExpiresAt) != "" {
		turn.ForegroundLeaseExpiresAt, err = time.Parse(timeLayout, foregroundLeaseExpiresAt)
		if err != nil {
			return types.Turn{}, err
		}
	}
	return turn, nil
}
