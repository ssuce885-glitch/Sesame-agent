package sqlite

import (
	"context"
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

func (s *Store) InsertConversationSummary(ctx context.Context, sessionID string, upToPosition int, summary model.Summary) error {
	payload, err := json.Marshal(summary)
	if err != nil {
		return err
	}

	_, err = s.db.ExecContext(ctx, `
		insert into conversation_summaries (session_id, up_to_position, payload, created_at)
		values (?, ?, ?, ?)
	`, sessionID, upToPosition, string(payload), time.Now().UTC().Format(timeLayout))
	return err
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

func (s *Store) ListConversationTimelineItems(ctx context.Context, sessionID string) ([]types.ConversationTimelineItem, error) {
	rows, err := s.db.QueryContext(ctx, `
		select turn_id, payload
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
		var turnID string
		var rawPayload string
		if err := rows.Scan(&turnID, &rawPayload); err != nil {
			return nil, err
		}

		var item model.ConversationItem
		if err := json.Unmarshal([]byte(rawPayload), &item); err != nil {
			return nil, err
		}
		out = append(out, types.ConversationTimelineItem{
			TurnID: turnID,
			Item:   item,
		})
	}

	return out, rows.Err()
}

func (s *Store) ListConversationSummaries(ctx context.Context, sessionID string) ([]model.Summary, error) {
	rows, err := s.db.QueryContext(ctx, `
		select payload
		from conversation_summaries
		where session_id = ?
		order by up_to_position asc, id asc
	`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []model.Summary
	for rows.Next() {
		var rawPayload string
		if err := rows.Scan(&rawPayload); err != nil {
			return nil, err
		}

		var summary model.Summary
		if err := json.Unmarshal([]byte(rawPayload), &summary); err != nil {
			return nil, err
		}
		out = append(out, summary)
	}

	return out, rows.Err()
}
