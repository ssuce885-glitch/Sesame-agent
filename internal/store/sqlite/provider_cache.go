package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"go-agent/internal/types"
)

func (s *Store) UpsertProviderCacheHead(ctx context.Context, head types.ProviderCacheHead) error {
	_, err := s.db.ExecContext(ctx, `
		insert into provider_cache_heads (
			session_id, provider, capability_profile, active_session_ref, active_prefix_ref, active_generation, updated_at
		) values (?, ?, ?, ?, ?, ?, ?)
		on conflict(session_id, provider, capability_profile) do update set
			active_session_ref = excluded.active_session_ref,
			active_prefix_ref = excluded.active_prefix_ref,
			active_generation = excluded.active_generation,
			updated_at = excluded.updated_at
	`,
		head.SessionID,
		head.Provider,
		head.CapabilityProfile,
		head.ActiveSessionRef,
		head.ActivePrefixRef,
		head.ActiveGeneration,
		head.UpdatedAt.UTC().Format(timeLayout),
	)
	return err
}

func (s *Store) GetProviderCacheHead(ctx context.Context, sessionID, provider, capabilityProfile string) (types.ProviderCacheHead, bool, error) {
	var head types.ProviderCacheHead
	var updatedAt string
	err := s.db.QueryRowContext(ctx, `
		select session_id, provider, capability_profile, active_session_ref, active_prefix_ref, active_generation, updated_at
		from provider_cache_heads
		where session_id = ? and provider = ? and capability_profile = ?
	`, sessionID, provider, capabilityProfile).Scan(
		&head.SessionID,
		&head.Provider,
		&head.CapabilityProfile,
		&head.ActiveSessionRef,
		&head.ActivePrefixRef,
		&head.ActiveGeneration,
		&updatedAt,
	)
	if err == sql.ErrNoRows {
		return types.ProviderCacheHead{}, false, nil
	}
	if err != nil {
		return types.ProviderCacheHead{}, false, err
	}

	parsed, err := time.Parse(timeLayout, updatedAt)
	if err != nil {
		return types.ProviderCacheHead{}, false, err
	}
	head.UpdatedAt = parsed
	return head, true, nil
}

func (s *Store) InsertProviderCacheEntry(ctx context.Context, entry types.ProviderCacheEntry) error {
	metadataJSON := entry.MetadataJSON
	if metadataJSON == "" {
		metadataJSON = "{}"
	}

	_, err := s.db.ExecContext(ctx, `
		insert into provider_cache_entries (
			id, session_id, provider, capability_profile, cache_kind, external_ref, parent_external_ref,
			generation, status, expires_at, last_used_at, metadata_json, created_at, updated_at
		) values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		entry.ID,
		entry.SessionID,
		entry.Provider,
		entry.CapabilityProfile,
		entry.CacheKind,
		entry.ExternalRef,
		entry.ParentExternalRef,
		entry.Generation,
		entry.Status,
		formatOptionalTime(entry.ExpiresAt),
		formatOptionalTime(entry.LastUsedAt),
		metadataJSON,
		entry.CreatedAt.UTC().Format(timeLayout),
		entry.UpdatedAt.UTC().Format(timeLayout),
	)
	return err
}

func (s *Store) InsertConversationCompaction(ctx context.Context, compaction types.ConversationCompaction) error {
	metadataJSON := compaction.MetadataJSON

	_, err := s.db.ExecContext(ctx, `
		insert into conversation_compactions (
			id, session_id, kind, generation, start_position, end_position, summary_payload, metadata_json, reason, provider_profile, created_at
		) values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		compaction.ID,
		compaction.SessionID,
		compaction.Kind,
		compaction.Generation,
		compaction.StartPosition,
		compaction.EndPosition,
		compaction.SummaryPayload,
		metadataJSON,
		compaction.Reason,
		compaction.ProviderProfile,
		compaction.CreatedAt.UTC().Format(timeLayout),
	)
	return err
}

func (s *Store) InsertConversationCompactionWithContextHead(ctx context.Context, compaction types.ConversationCompaction) error {
	if err := validateStoredContextHeadID(compaction.ContextHeadID); err != nil {
		return err
	}

	metadataJSON := compaction.MetadataJSON

	_, err := s.db.ExecContext(ctx, `
		insert into conversation_compactions (
			id, session_id, context_head_id, kind, generation, start_item_id, end_item_id, start_position, end_position,
			summary_payload, metadata_json, reason, provider_profile, created_at
		) values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		compaction.ID,
		compaction.SessionID,
		compaction.ContextHeadID,
		compaction.Kind,
		compaction.Generation,
		compaction.StartItemID,
		compaction.EndItemID,
		compaction.StartPosition,
		compaction.EndPosition,
		compaction.SummaryPayload,
		metadataJSON,
		compaction.Reason,
		compaction.ProviderProfile,
		compaction.CreatedAt.UTC().Format(timeLayout),
	)
	return err
}

func (s *Store) ListConversationCompactions(ctx context.Context, sessionID string) ([]types.ConversationCompaction, error) {
	rows, err := s.db.QueryContext(ctx, `
		select id, session_id, kind, generation, start_position, end_position, summary_payload, metadata_json, reason, provider_profile, created_at
		from conversation_compactions
		where session_id = ?
		order by created_at asc, id asc
	`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []types.ConversationCompaction
	for rows.Next() {
		var raw types.ConversationCompaction
		var createdAt string
		if err := rows.Scan(
			&raw.ID,
			&raw.SessionID,
			&raw.Kind,
			&raw.Generation,
			&raw.StartPosition,
			&raw.EndPosition,
			&raw.SummaryPayload,
			&raw.MetadataJSON,
			&raw.Reason,
			&raw.ProviderProfile,
			&createdAt,
		); err != nil {
			return nil, err
		}
		parsed, err := time.Parse(timeLayout, createdAt)
		if err != nil {
			return nil, err
		}
		raw.CreatedAt = parsed
		out = append(out, raw)
	}

	return out, rows.Err()
}

func (s *Store) ListConversationCompactionsByStoredContextHead(ctx context.Context, sessionID, contextHeadID string) ([]types.ConversationCompaction, error) {
	if err := validateStoredContextHeadID(contextHeadID); err != nil {
		return nil, err
	}

	rows, err := s.db.QueryContext(ctx, `
		select id, session_id, context_head_id, kind, generation, start_item_id, end_item_id, start_position, end_position, summary_payload, metadata_json, reason, provider_profile, created_at
		from conversation_compactions
		where session_id = ? and context_head_id = ?
		order by created_at asc, id asc
	`, sessionID, contextHeadID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []types.ConversationCompaction
	for rows.Next() {
		var raw types.ConversationCompaction
		var createdAt string
		if err := rows.Scan(
			&raw.ID,
			&raw.SessionID,
			&raw.ContextHeadID,
			&raw.Kind,
			&raw.Generation,
			&raw.StartItemID,
			&raw.EndItemID,
			&raw.StartPosition,
			&raw.EndPosition,
			&raw.SummaryPayload,
			&raw.MetadataJSON,
			&raw.Reason,
			&raw.ProviderProfile,
			&createdAt,
		); err != nil {
			return nil, err
		}
		parsed, err := time.Parse(timeLayout, createdAt)
		if err != nil {
			return nil, err
		}
		raw.CreatedAt = parsed
		out = append(out, raw)
	}

	return out, rows.Err()
}

func parseConversationCompactionPayload[T any](payload string) (T, error) {
	var out T
	err := json.Unmarshal([]byte(payload), &out)
	return out, err
}

func formatOptionalTime(ts *time.Time) string {
	if ts == nil || ts.IsZero() {
		return ""
	}
	return ts.UTC().Format(timeLayout)
}
