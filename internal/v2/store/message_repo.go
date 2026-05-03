package store

import (
	"context"
	"database/sql"
	"go-agent/internal/v2/contracts"
)

type messageRepo struct {
	db *sql.DB
	tx *sql.Tx
}

var _ contracts.MessageRepository = (*messageRepo)(nil)

func (r *messageRepo) execer() execer { return repoExec(r.db, r.tx) }

func (r *messageRepo) Append(ctx context.Context, messages []contracts.Message) error {
	for _, m := range messages {
		if m.ID > 0 {
			_, err := r.execer().Exec(`
INSERT INTO v2_messages (id, session_id, turn_id, role, content, tool_call_id, position, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
				m.ID, m.SessionID, m.TurnID, m.Role, m.Content, m.ToolCallID, m.Position, timeString(m.CreatedAt))
			if err != nil {
				return err
			}
			continue
		}
		_, err := r.execer().Exec(`
INSERT INTO v2_messages (session_id, turn_id, role, content, tool_call_id, position, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?)`,
			m.SessionID, m.TurnID, m.Role, m.Content, m.ToolCallID, m.Position, timeString(m.CreatedAt))
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *messageRepo) List(ctx context.Context, sessionID string, opts contracts.MessageListOptions) ([]contracts.Message, error) {
	query := `
SELECT id, session_id, turn_id, role, content, tool_call_id, position, created_at
FROM v2_messages WHERE session_id = ? ORDER BY position ASC`
	args := []any{sessionID}
	if opts.Limit > 0 {
		query += ` LIMIT ?`
		args = append(args, opts.Limit)
	}
	rows, err := r.execer().Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []contracts.Message
	for rows.Next() {
		m, err := scanMessage(rows)
		if err != nil {
			return nil, err
		}
		messages = append(messages, m)
	}
	return messages, rows.Err()
}

func (r *messageRepo) MaxPosition(ctx context.Context, sessionID string) (int, error) {
	var maxPosition int
	err := r.execer().QueryRow(`
SELECT COALESCE(MAX(position), 0) FROM v2_messages WHERE session_id = ?`, sessionID).Scan(&maxPosition)
	if err != nil {
		return 0, err
	}
	return maxPosition, nil
}

func (r *messageRepo) SaveSnapshot(ctx context.Context, sessionID string, label string, startPos, endPos int, summary string) (string, error) {
	id, err := newID("snapshot")
	if err != nil {
		return "", err
	}
	_, err = r.execer().Exec(`
INSERT INTO v2_message_snapshots (id, session_id, label, start_position, end_position, summary, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?)`,
		id, sessionID, label, startPos, endPos, summary, timeString(sqlNow()))
	if err != nil {
		return "", err
	}
	return id, nil
}

func (r *messageRepo) LoadSnapshot(ctx context.Context, snapshotID string) ([]contracts.Message, error) {
	var sessionID string
	var startPos, endPos int
	err := r.execer().QueryRow(`
SELECT session_id, start_position, end_position FROM v2_message_snapshots WHERE id = ?`, snapshotID).Scan(&sessionID, &startPos, &endPos)
	if err != nil {
		return nil, err
	}
	rows, err := r.execer().Query(`
SELECT id, session_id, turn_id, role, content, tool_call_id, position, created_at
FROM v2_messages
WHERE session_id = ? AND position BETWEEN ? AND ?
ORDER BY position ASC`, sessionID, startPos, endPos)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanMessageList(rows)
}

func scanMessage(row interface {
	Scan(dest ...any) error
}) (contracts.Message, error) {
	var m contracts.Message
	var createdAt string
	err := row.Scan(&m.ID, &m.SessionID, &m.TurnID, &m.Role, &m.Content, &m.ToolCallID, &m.Position, &createdAt)
	if err != nil {
		return contracts.Message{}, err
	}
	m.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return contracts.Message{}, err
	}
	return m, nil
}

func scanMessageList(rows *sql.Rows) ([]contracts.Message, error) {
	var messages []contracts.Message
	for rows.Next() {
		m, err := scanMessage(rows)
		if err != nil {
			return nil, err
		}
		messages = append(messages, m)
	}
	return messages, rows.Err()
}
