package sqlite

import (
	"context"
	"encoding/json"
	"time"

	"go-agent/internal/types"
)

func (s *Store) AppendEvent(ctx context.Context, event types.Event) (int64, error) {
	res, err := s.db.ExecContext(ctx, `
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
