package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
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
		select id, session_id, context_head_id, turn_kind, client_turn_id, state, execution_mode, foreground_lease_id, foreground_lease_expires_at, user_message, created_at, updated_at
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

func (s *Store) ListContextHistory(ctx context.Context, sessionID string) ([]types.HistoryEntry, string, error) {
	currentHeadID, ok, err := s.GetCurrentContextHeadID(ctx)
	if err != nil {
		return nil, "", err
	}
	if !ok {
		currentHeadID = ""
	}

	rows, err := s.db.QueryContext(ctx, `
		select id, title, preview, source_kind, created_at, updated_at
		from context_heads
		where session_id = ?
		order by updated_at desc, created_at desc, id desc
	`, sessionID)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()

	entries := []types.HistoryEntry{}
	for rows.Next() {
		var entry types.HistoryEntry
		var createdAt string
		var updatedAt string
		if err := rows.Scan(
			&entry.ID,
			&entry.Title,
			&entry.Preview,
			&entry.SourceKind,
			&createdAt,
			&updatedAt,
		); err != nil {
			return nil, "", err
		}
		entry.IsCurrent = entry.ID == currentHeadID
		entry.CreatedAt, err = time.Parse(timeLayout, createdAt)
		if err != nil {
			return nil, "", err
		}
		entry.UpdatedAt, err = time.Parse(timeLayout, updatedAt)
		if err != nil {
			return nil, "", err
		}
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, "", err
	}
	if currentHeadID != "" {
		for idx, entry := range entries {
			if entry.ID != currentHeadID {
				continue
			}
			if idx == 0 {
				break
			}
			current := entry
			copy(entries[1:idx+1], entries[0:idx])
			entries[0] = current
			break
		}
	}
	return entries, currentHeadID, nil
}

func (s *Store) CreateReopenContextHead(ctx context.Context, sessionID string) (types.ContextHead, error) {
	head := types.ContextHead{
		ID:         types.NewID("head"),
		SessionID:  strings.TrimSpace(sessionID),
		SourceKind: types.ContextHeadSourceReopen,
		CreatedAt:  time.Now().UTC(),
		UpdatedAt:  time.Now().UTC(),
	}
	if strings.TrimSpace(head.SessionID) == "" {
		return types.ContextHead{}, fmt.Errorf("session_id is required")
	}
	if _, found, err := s.GetSession(ctx, head.SessionID); err != nil {
		return types.ContextHead{}, err
	} else if !found {
		return types.ContextHead{}, sql.ErrNoRows
	}
	if err := s.InsertContextHead(ctx, head); err != nil {
		return types.ContextHead{}, err
	}
	if err := s.SetCurrentContextHeadID(ctx, head.ID); err != nil {
		return types.ContextHead{}, err
	}
	return head, nil
}

func (s *Store) LoadContextHead(ctx context.Context, sessionID, headID string) (types.ContextHead, error) {
	sessionID = strings.TrimSpace(sessionID)
	headID = strings.TrimSpace(headID)
	if sessionID == "" {
		return types.ContextHead{}, fmt.Errorf("session_id is required")
	}
	if headID == "" {
		return types.ContextHead{}, fmt.Errorf("head_id is required")
	}

	parent, found, err := s.GetContextHead(ctx, headID)
	if err != nil {
		return types.ContextHead{}, err
	}
	if !found || parent.SessionID != sessionID {
		return types.ContextHead{}, sql.ErrNoRows
	}

	head := types.ContextHead{
		ID:           types.NewID("head"),
		SessionID:    sessionID,
		ParentHeadID: parent.ID,
		SourceKind:   types.ContextHeadSourceHistoryLoad,
		Title:        parent.Title,
		Preview:      parent.Preview,
		CreatedAt:    time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
	}
	if err := s.InsertContextHead(ctx, head); err != nil {
		return types.ContextHead{}, err
	}
	if err := s.SetCurrentContextHeadID(ctx, head.ID); err != nil {
		return types.ContextHead{}, err
	}
	return head, nil
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
	var kind string
	var state string
	var executionMode string
	var foregroundLeaseExpiresAt string
	var createdAt string
	var updatedAt string
	if err := scanner.Scan(
		&turn.ID,
		&turn.SessionID,
		&turn.ContextHeadID,
		&kind,
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

	turn.Kind = types.TurnKind(kind)
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
