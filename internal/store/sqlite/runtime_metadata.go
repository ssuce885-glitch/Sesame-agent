package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

const selectedSessionMetadataKey = "last_selected_session_id"

type queryRowContexter interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func (s *Store) GetSelectedSessionID(ctx context.Context) (string, bool, error) {
	return getSelectedSessionIDWithQueryer(ctx, s.db)
}

func getSelectedSessionIDWithQueryer(ctx context.Context, queryer queryRowContexter) (string, bool, error) {
	var value string
	err := queryer.QueryRowContext(ctx, `
		select value
		from runtime_metadata
		where key = ?
	`, selectedSessionMetadataKey).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return value, true, nil
}

func (s *Store) SetSelectedSessionID(ctx context.Context, sessionID string) error {
	return setSelectedSessionIDWithExec(ctx, s.db, sessionID)
}

func setSelectedSessionIDWithExec(ctx context.Context, execer execContexter, sessionID string) error {
	_, err := execer.ExecContext(ctx, `
		insert into runtime_metadata (key, value, updated_at)
		values (?, ?, ?)
		on conflict(key) do update set
			value = excluded.value,
			updated_at = excluded.updated_at
	`, selectedSessionMetadataKey, sessionID, time.Now().UTC().Format(timeLayout))
	return err
}

func clearSelectedSessionIDWithExec(ctx context.Context, execer execContexter) error {
	_, err := execer.ExecContext(ctx, `
		delete from runtime_metadata
		where key = ?
	`, selectedSessionMetadataKey)
	return err
}
