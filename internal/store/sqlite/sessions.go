package sqlite

import (
	"context"
	"time"

	"go-agent/internal/types"
)

const timeLayout = time.RFC3339Nano

func (s *Store) InsertSession(ctx context.Context, session types.Session) error {
	_, err := s.db.ExecContext(ctx, `
		insert into sessions (id, workspace_root, state, active_turn_id, created_at, updated_at)
		values (?, ?, ?, ?, ?, ?)`,
		session.ID,
		session.WorkspaceRoot,
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

func (s *Store) DeleteTurn(ctx context.Context, turnID string) error {
	_, err := s.db.ExecContext(ctx, `
		delete from turns
		where id = ?`,
		turnID,
	)

	return err
}
