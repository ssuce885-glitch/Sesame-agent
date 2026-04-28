package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"go-agent/internal/model"
	"go-agent/internal/types"
)

func (s *Store) InsertConversationItem(ctx context.Context, sessionID, turnID string, position int, item model.ConversationItem) error {
	payload, err := json.Marshal(item)
	if err != nil {
		return err
	}

	_, err = s.db.ExecContext(ctx, `
		insert into conversation_items (session_id, turn_id, position, kind, payload, created_at)
		values (?, ?, ?, ?, ?, ?)
	`, sessionID, turnID, position, item.Kind, string(payload), time.Now().UTC().Format(timeLayout))
	return err
}

func (s *Store) InsertConversationItemWithContextHead(ctx context.Context, sessionID, contextHeadID, turnID string, position int, item model.ConversationItem) error {
	if err := validateStoredContextHeadID(contextHeadID); err != nil {
		return err
	}

	payload, err := json.Marshal(item)
	if err != nil {
		return err
	}

	_, err = s.db.ExecContext(ctx, `
		insert into conversation_items (session_id, context_head_id, turn_id, position, kind, payload, created_at)
		values (?, ?, ?, ?, ?, ?, ?)
	`, sessionID, contextHeadID, turnID, position, item.Kind, string(payload), time.Now().UTC().Format(timeLayout))
	return err
}

func (s *Store) GetConversationItemIDByContextHeadAndPosition(ctx context.Context, sessionID, contextHeadID string, position int) (int64, bool, error) {
	if err := validateStoredContextHeadID(contextHeadID); err != nil {
		return 0, false, err
	}

	var itemID int64
	err := s.db.QueryRowContext(ctx, `
		select id
		from conversation_items
		where session_id = ? and context_head_id = ? and position = ?
		order by id asc
		limit 1
	`, sessionID, contextHeadID, position).Scan(&itemID)
	if err == sql.ErrNoRows {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return itemID, true, nil
}

func (s *Store) ListConversationItems(ctx context.Context, sessionID string) ([]model.ConversationItem, error) {
	rows, err := s.db.QueryContext(ctx, `
		select payload
		from conversation_items
		where session_id = ?
		order by position asc, id asc
	`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []model.ConversationItem
	for rows.Next() {
		var rawPayload string
		if err := rows.Scan(&rawPayload); err != nil {
			return nil, err
		}

		var item model.ConversationItem
		if err := json.Unmarshal([]byte(rawPayload), &item); err != nil {
			return nil, err
		}
		out = append(out, item)
	}

	return out, rows.Err()
}

func (s *Store) MaxConversationPosition(ctx context.Context, sessionID string) (int, bool, error) {
	var maxPos sql.NullInt64
	err := s.db.QueryRowContext(ctx, `
		select max(position)
		from conversation_items
		where session_id = ?
	`, sessionID).Scan(&maxPos)
	if err != nil {
		return 0, false, err
	}
	if !maxPos.Valid {
		return 0, false, nil
	}
	return int(maxPos.Int64), true, nil
}

func (s *Store) ListConversationTimelineItems(ctx context.Context, sessionID string) ([]types.ConversationTimelineItem, error) {
	rows, err := s.db.QueryContext(ctx, `
		select id, position, turn_id, payload
		from conversation_items
		where session_id = ?
		order by position asc, id asc
	`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []types.ConversationTimelineItem
	for rows.Next() {
		var itemID int64
		var position int
		var turnID string
		var rawPayload string
		if err := rows.Scan(&itemID, &position, &turnID, &rawPayload); err != nil {
			return nil, err
		}

		var item model.ConversationItem
		if err := json.Unmarshal([]byte(rawPayload), &item); err != nil {
			return nil, err
		}
		out = append(out, types.ConversationTimelineItem{
			ItemID:   itemID,
			Position: position,
			TurnID:   turnID,
			Item:     item,
		})
	}

	return out, rows.Err()
}

func (s *Store) ListConversationTimelineItemsByStoredContextHeads(ctx context.Context, sessionID, headID string) ([]types.ConversationTimelineItem, error) {
	if err := validateStoredContextHeadID(headID); err != nil {
		return nil, err
	}

	rows, err := s.db.QueryContext(ctx, `
		select id, position, turn_id, payload
		from conversation_items
		where session_id = ? and context_head_id = ?
		order by position asc, id asc
	`, sessionID, headID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []types.ConversationTimelineItem
	for rows.Next() {
		var itemID int64
		var position int
		var turnID string
		var rawPayload string
		if err := rows.Scan(&itemID, &position, &turnID, &rawPayload); err != nil {
			return nil, err
		}

		var item model.ConversationItem
		if err := json.Unmarshal([]byte(rawPayload), &item); err != nil {
			return nil, err
		}
		out = append(out, types.ConversationTimelineItem{
			ItemID:   itemID,
			Position: position,
			TurnID:   turnID,
			Item:     item,
		})
	}

	return out, rows.Err()
}

func (s *Store) ListConversationTimelineItemsByContextHead(ctx context.Context, sessionID, headID string) ([]types.ConversationTimelineItem, error) {
	lineage, err := s.ListContextHeadLineage(ctx, sessionID, headID)
	if err != nil {
		return nil, err
	}

	allowedTurns := make(map[string]struct{})
	for _, head := range lineage {
		turns, err := s.ListTurnsByContextHead(ctx, sessionID, head.ID)
		if err != nil {
			return nil, err
		}
		for _, turn := range turns {
			allowedTurns[turn.ID] = struct{}{}
		}
	}

	items, err := s.ListConversationTimelineItems(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	out := make([]types.ConversationTimelineItem, 0, len(items))
	for _, item := range items {
		if _, ok := allowedTurns[item.TurnID]; ok {
			out = append(out, item)
		}
	}
	return out, nil
}

func (s *Store) ListConversationItemsByContextHead(ctx context.Context, sessionID, headID string) ([]model.ConversationItem, error) {
	timelineItems, err := s.ListConversationTimelineItemsByContextHead(ctx, sessionID, headID)
	if err != nil {
		return nil, err
	}
	out := make([]model.ConversationItem, 0, len(timelineItems))
	for _, item := range timelineItems {
		out = append(out, item.Item)
	}
	return out, nil
}

func (s *Store) ListConversationItemsByStoredContextHeads(ctx context.Context, sessionID, headID string) ([]model.ConversationItem, error) {
	if err := validateStoredContextHeadID(headID); err != nil {
		return nil, err
	}

	timelineItems, err := s.ListConversationTimelineItemsByStoredContextHeads(ctx, sessionID, headID)
	if err != nil {
		return nil, err
	}
	out := make([]model.ConversationItem, 0, len(timelineItems))
	for _, item := range timelineItems {
		out = append(out, item.Item)
	}
	return out, nil
}
