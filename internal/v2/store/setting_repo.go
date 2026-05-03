package store

import (
	"context"
	"database/sql"
	"go-agent/internal/v2/contracts"
)

type settingRepo struct {
	db *sql.DB
	tx *sql.Tx
}

var _ contracts.SettingRepository = (*settingRepo)(nil)

func (r *settingRepo) execer() execer { return repoExec(r.db, r.tx) }

func (r *settingRepo) Get(ctx context.Context, key string) (string, error) {
	var value string
	err := r.execer().QueryRow(`SELECT value FROM v2_settings WHERE key = ?`, key).Scan(&value)
	return value, err
}

func (r *settingRepo) Set(ctx context.Context, key, value string) error {
	_, err := r.execer().Exec(`
INSERT INTO v2_settings (key, value, updated_at) VALUES (?, ?, ?)
ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`,
		key, value, timeString(sqlNow()))
	return err
}

func (r *settingRepo) Delete(ctx context.Context, key string) error {
	_, err := r.execer().Exec(`DELETE FROM v2_settings WHERE key = ?`, key)
	return err
}
