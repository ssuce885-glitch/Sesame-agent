package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"go-agent/internal/types"
)

func (s *Store) UpsertTurnUsage(ctx context.Context, usage types.TurnUsage) error {
	createdAt := usage.CreatedAt.UTC()
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	updatedAt := usage.UpdatedAt.UTC()
	if updatedAt.IsZero() {
		updatedAt = createdAt
	}

	_, err := s.db.ExecContext(ctx, `
		insert into turn_usage (
			turn_id, session_id, provider, model, input_tokens, output_tokens, cached_tokens, cache_hit_rate, created_at, updated_at
		)
		values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		on conflict(turn_id) do update set
			session_id = excluded.session_id,
			provider = excluded.provider,
			model = excluded.model,
			input_tokens = excluded.input_tokens,
			output_tokens = excluded.output_tokens,
			cached_tokens = excluded.cached_tokens,
			cache_hit_rate = excluded.cache_hit_rate,
			updated_at = excluded.updated_at
	`,
		usage.TurnID,
		usage.SessionID,
		usage.Provider,
		usage.Model,
		usage.InputTokens,
		usage.OutputTokens,
		usage.CachedTokens,
		usage.CacheHitRate,
		createdAt.Format(timeLayout),
		updatedAt.Format(timeLayout),
	)
	return err
}

func (s *Store) GetTurnUsage(ctx context.Context, turnID string) (types.TurnUsage, bool, error) {
	var usage types.TurnUsage
	var createdAt string
	var updatedAt string

	err := s.db.QueryRowContext(ctx, `
		select turn_id, session_id, provider, model, input_tokens, output_tokens, cached_tokens, cache_hit_rate, created_at, updated_at
		from turn_usage
		where turn_id = ?
	`, turnID).Scan(
		&usage.TurnID,
		&usage.SessionID,
		&usage.Provider,
		&usage.Model,
		&usage.InputTokens,
		&usage.OutputTokens,
		&usage.CachedTokens,
		&usage.CacheHitRate,
		&createdAt,
		&updatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return types.TurnUsage{}, false, nil
	}
	if err != nil {
		return types.TurnUsage{}, false, err
	}

	usage.CreatedAt, err = time.Parse(timeLayout, createdAt)
	if err != nil {
		return types.TurnUsage{}, false, err
	}
	usage.UpdatedAt, err = time.Parse(timeLayout, updatedAt)
	if err != nil {
		return types.TurnUsage{}, false, err
	}

	return usage, true, nil
}
