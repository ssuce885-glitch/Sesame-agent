package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

type queryRowContexter interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

const canonicalSessionMetadataKey = "canonical_session_id"
const currentContextHeadMetadataKey = "current_context_head_id"

func (s *Store) GetCanonicalSessionID(ctx context.Context) (string, bool, error) {
	return getRuntimeMetadataValue(ctx, s.db, canonicalSessionMetadataKey)
}

func (s *Store) SetCanonicalSessionID(ctx context.Context, sessionID string) error {
	return setRuntimeMetadataValue(ctx, s.db, canonicalSessionMetadataKey, sessionID)
}

func (s *Store) GetCurrentContextHeadID(ctx context.Context) (string, bool, error) {
	return getRuntimeMetadataValue(ctx, s.db, currentContextHeadMetadataKey)
}

func (s *Store) SetCurrentContextHeadID(ctx context.Context, headID string) error {
	return setRuntimeMetadataValue(ctx, s.db, currentContextHeadMetadataKey, headID)
}

func getRuntimeMetadataValue(ctx context.Context, queryer queryRowContexter, key string) (string, bool, error) {
	var value string
	err := queryer.QueryRowContext(ctx, `
		select value
		from runtime_metadata
		where key = ?
	`, key).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return value, true, nil
}

func setRuntimeMetadataValue(ctx context.Context, execer execContexter, key, value string) error {
	_, err := execer.ExecContext(ctx, `
		insert into runtime_metadata (key, value, updated_at)
		values (?, ?, ?)
		on conflict(key) do update set
			value = excluded.value,
			updated_at = excluded.updated_at
	`, key, value, time.Now().UTC().Format(timeLayout))
	return err
}
