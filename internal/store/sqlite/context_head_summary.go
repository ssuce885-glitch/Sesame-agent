package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"go-agent/internal/types"
)

var ErrContextHeadIDRequired = errors.New("context_head_id is required")

func validateStoredContextHeadID(contextHeadID string) error {
	if strings.TrimSpace(contextHeadID) == "" {
		return ErrContextHeadIDRequired
	}
	return nil
}

func (s *Store) GetContextHeadSummary(ctx context.Context, sessionID, contextHeadID string) (types.ContextHeadSummary, bool, error) {
	if err := validateStoredContextHeadID(contextHeadID); err != nil {
		return types.ContextHeadSummary{}, false, err
	}

	var memory types.ContextHeadSummary
	var createdAt string
	var updatedAt string

	err := s.db.QueryRowContext(ctx, `
		select session_id, context_head_id, workspace_root, source_turn_id, up_to_item_id, item_count, summary_payload, created_at, updated_at
		from context_head_summaries
		where session_id = ? and context_head_id = ?
	`, sessionID, contextHeadID).Scan(
		&memory.SessionID,
		&memory.ContextHeadID,
		&memory.WorkspaceRoot,
		&memory.SourceTurnID,
		&memory.UpToItemID,
		&memory.ItemCount,
		&memory.SummaryPayload,
		&createdAt,
		&updatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return types.ContextHeadSummary{}, false, nil
	}
	if err != nil {
		return types.ContextHeadSummary{}, false, err
	}

	memory.CreatedAt, err = time.Parse(timeLayout, createdAt)
	if err != nil {
		return types.ContextHeadSummary{}, false, err
	}
	memory.UpdatedAt, err = time.Parse(timeLayout, updatedAt)
	if err != nil {
		return types.ContextHeadSummary{}, false, err
	}

	return memory, true, nil
}

func (s *Store) UpsertContextHeadSummary(ctx context.Context, record types.ContextHeadSummary) error {
	if err := validateStoredContextHeadID(record.ContextHeadID); err != nil {
		return err
	}

	now := time.Now().UTC()
	if record.CreatedAt.IsZero() {
		record.CreatedAt = now
	}
	if record.UpdatedAt.IsZero() {
		record.UpdatedAt = now
	}

	_, err := s.db.ExecContext(ctx, `
		insert into context_head_summaries (
			session_id, context_head_id, workspace_root, source_turn_id, up_to_item_id, item_count, summary_payload, created_at, updated_at
		) values (?, ?, ?, ?, ?, ?, ?, ?, ?)
		on conflict(session_id, context_head_id) do update set
			workspace_root = excluded.workspace_root,
			source_turn_id = excluded.source_turn_id,
			up_to_item_id = excluded.up_to_item_id,
			item_count = excluded.item_count,
			summary_payload = excluded.summary_payload,
			updated_at = excluded.updated_at
	`,
		record.SessionID,
		record.ContextHeadID,
		record.WorkspaceRoot,
		record.SourceTurnID,
		record.UpToItemID,
		record.ItemCount,
		record.SummaryPayload,
		record.CreatedAt.UTC().Format(timeLayout),
		record.UpdatedAt.UTC().Format(timeLayout),
	)
	return err
}
