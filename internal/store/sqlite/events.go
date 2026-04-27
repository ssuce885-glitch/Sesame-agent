package sqlite

import (
	"context"
	"encoding/json"
	"time"

	"go-agent/internal/types"
)

func (s *Store) AppendEvent(ctx context.Context, event types.Event) (int64, error) {
	return appendEventWithExec(ctx, s.db, event)
}

func (s *Store) ListTurnsBySession(ctx context.Context, sessionID string) ([]types.Turn, error) {
	rows, err := s.db.QueryContext(ctx, `
		select id, session_id, context_head_id, turn_kind, client_turn_id, state, user_message, created_at, updated_at
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

func (s *Store) LatestSessionEventSeq(ctx context.Context, sessionID string) (int64, error) {
	var seq int64
	err := s.db.QueryRowContext(ctx, `
		select coalesce(max(seq), 0)
		from events
		where session_id = ?
	`, sessionID).Scan(&seq)
	return seq, err
}

func appendEventWithExec(ctx context.Context, execer execContexter, event types.Event) (int64, error) {
	res, err := execer.ExecContext(ctx, `
		insert into events (id, session_id, turn_id, type, time, payload)
		values (?, ?, ?, ?, ?, ?)`,
		event.ID,
		event.SessionID,
		event.TurnID,
		event.Type,
		event.Time.Format(timeLayout),
		string(event.Payload),
	)
	if err != nil {
		return 0, err
	}

	return res.LastInsertId()
}

func (s *Store) ListSessionEvents(ctx context.Context, sessionID string, afterSeq int64) ([]types.Event, error) {
	rows, err := s.db.QueryContext(ctx, `
		select seq, id, session_id, turn_id, type, time, payload
		from events
		where session_id = ? and seq > ?
		order by seq asc`,
		sessionID,
		afterSeq,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []types.Event
	for rows.Next() {
		var event types.Event
		var eventTime string
		var payload string
		if err := rows.Scan(
			&event.Seq,
			&event.ID,
			&event.SessionID,
			&event.TurnID,
			&event.Type,
			&eventTime,
			&payload,
		); err != nil {
			return nil, err
		}

		parsed, err := time.Parse(timeLayout, eventTime)
		if err != nil {
			return nil, err
		}

		event.Time = parsed
		event.Payload = json.RawMessage(payload)
		events = append(events, event)
	}

	return events, rows.Err()
}
