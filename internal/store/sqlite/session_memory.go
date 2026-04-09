package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"go-agent/internal/types"
)

func (s *Store) GetSessionMemory(ctx context.Context, sessionID string) (types.SessionMemory, bool, error) {
	var (
		memory    types.SessionMemory
		createdAt string
		updatedAt string
	)

	err := s.db.QueryRowContext(ctx, `
		select session_id, workspace_root, source_turn_id, up_to_position, item_count, summary_payload, created_at, updated_at
		from session_memories
		where session_id = ?
	`, sessionID).Scan(
		&memory.SessionID,
		&memory.WorkspaceRoot,
		&memory.SourceTurnID,
		&memory.UpToPosition,
		&memory.ItemCount,
		&memory.SummaryPayload,
		&createdAt,
		&updatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return types.SessionMemory{}, false, nil
		}
		return types.SessionMemory{}, false, err
	}

	if parsed, err := time.Parse(timeLayout, createdAt); err == nil {
		memory.CreatedAt = parsed
	}
	if parsed, err := time.Parse(timeLayout, updatedAt); err == nil {
		memory.UpdatedAt = parsed
	}
	return memory, true, nil
}

func (s *Store) UpsertSessionMemory(ctx context.Context, memory types.SessionMemory) error {
	now := time.Now().UTC()
	if memory.CreatedAt.IsZero() {
		memory.CreatedAt = now
	}
	if memory.UpdatedAt.IsZero() {
		memory.UpdatedAt = now
	}

	_, err := s.db.ExecContext(ctx, `
		insert into session_memories (
			session_id, workspace_root, source_turn_id, up_to_position, item_count, summary_payload, created_at, updated_at
		) values (?, ?, ?, ?, ?, ?, ?, ?)
		on conflict(session_id) do update set
			workspace_root = excluded.workspace_root,
			source_turn_id = excluded.source_turn_id,
			up_to_position = excluded.up_to_position,
			item_count = excluded.item_count,
			summary_payload = excluded.summary_payload,
			created_at = excluded.created_at,
			updated_at = excluded.updated_at
	`,
		memory.SessionID,
		memory.WorkspaceRoot,
		memory.SourceTurnID,
		memory.UpToPosition,
		memory.ItemCount,
		memory.SummaryPayload,
		memory.CreatedAt.Format(timeLayout),
		memory.UpdatedAt.Format(timeLayout),
	)
	return err
}
