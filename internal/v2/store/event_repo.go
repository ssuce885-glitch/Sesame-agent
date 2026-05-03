package store

import (
	"context"
	"database/sql"
	"go-agent/internal/v2/contracts"
)

type eventRepo struct {
	db *sql.DB
	tx *sql.Tx
}

var _ contracts.EventRepository = (*eventRepo)(nil)

func (r *eventRepo) execer() execer { return repoExec(r.db, r.tx) }

func (r *eventRepo) Append(ctx context.Context, events []contracts.Event) error {
	for _, e := range events {
		if e.Seq > 0 {
			_, err := r.execer().Exec(`
INSERT INTO v2_events (seq, id, session_id, turn_id, type, time, payload)
VALUES (?, ?, ?, ?, ?, ?, ?)`,
				e.Seq, e.ID, e.SessionID, e.TurnID, e.Type, timeString(e.Time), e.Payload)
			if err != nil {
				return err
			}
			continue
		}
		_, err := r.execer().Exec(`
INSERT INTO v2_events (id, session_id, turn_id, type, time, payload)
VALUES (?, ?, ?, ?, ?, ?)`,
			e.ID, e.SessionID, e.TurnID, e.Type, timeString(e.Time), e.Payload)
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *eventRepo) List(ctx context.Context, sessionID string, afterSeq int64, limit int) ([]contracts.Event, error) {
	query := `
SELECT seq, id, session_id, turn_id, type, time, payload
FROM v2_events WHERE session_id = ? AND seq > ? ORDER BY seq ASC`
	args := []any{sessionID, afterSeq}
	if limit > 0 {
		query += ` LIMIT ?`
		args = append(args, limit)
	}
	rows, err := r.execer().Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []contracts.Event
	for rows.Next() {
		e, err := scanEvent(rows)
		if err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	return events, rows.Err()
}

func scanEvent(row interface {
	Scan(dest ...any) error
}) (contracts.Event, error) {
	var e contracts.Event
	var eventTime string
	err := row.Scan(&e.Seq, &e.ID, &e.SessionID, &e.TurnID, &e.Type, &eventTime, &e.Payload)
	if err != nil {
		return contracts.Event{}, err
	}
	e.Time, err = parseTime(eventTime)
	if err != nil {
		return contracts.Event{}, err
	}
	return e, nil
}
