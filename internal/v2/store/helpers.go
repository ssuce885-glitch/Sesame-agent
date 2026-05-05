package store

import (
	"crypto/rand"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// execer is used by repos that support both db and tx execution.
type execer interface {
	Exec(query string, args ...any) (sql.Result, error)
	Query(query string, args ...any) (*sql.Rows, error)
	QueryRow(query string, args ...any) *sql.Row
}

func repoExec(db *sql.DB, tx *sql.Tx) execer {
	if tx != nil {
		return tx
	}
	return db
}

func timeString(t time.Time) string {
	if t.IsZero() {
		t = time.Now().UTC()
	}
	return t.UTC().Format(time.RFC3339Nano)
}

func sqlNow() time.Time {
	return time.Now().UTC()
}

func parseTime(value string) (time.Time, error) {
	t, err := time.Parse(time.RFC3339Nano, value)
	if err == nil {
		return t, nil
	}
	t, err = time.Parse("2006-01-02 15:04:05", value)
	if err == nil {
		return t, nil
	}
	return time.Time{}, err
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func intBool(value int) bool {
	return value != 0
}

func newID(prefix string) (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("generate id: %w", err)
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%s-%08x-%04x-%04x-%04x-%012x", prefix, b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}

func firstNonEmptyStore(value, fallback string) string {
	if strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	return fallback
}

func requireAffected(result sql.Result) error {
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}
