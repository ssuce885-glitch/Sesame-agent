package schema

import (
	"database/sql"
	"fmt"
	"sort"
)

// Migration is a single schema migration.
type Migration struct {
	Version int
	Name    string
	Up      string // raw SQL (may contain multiple statements)
}

// List holds all migrations in version order.
var List = []Migration{
	Migration001,
	Migration002,
	Migration003,
	Migration004,
	Migration005,
	Migration006,
	Migration007,
	Migration008,
	Migration009,
	Migration010,
	Migration011,
	Migration012,
}

func init() {
	// Ensure migrations are ordered by version.
	sort.Slice(List, func(i, j int) bool { return List[i].Version < List[j].Version })
}

// Run applies all unapplied migrations.
func Run(db *sql.DB) error {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS v2_schema_version (version INTEGER PRIMARY KEY, name TEXT NOT NULL, applied_at TEXT NOT NULL DEFAULT (datetime('now')))`)
	if err != nil {
		return fmt.Errorf("create schema_version: %w", err)
	}

	rows, err := db.Query(`SELECT version FROM v2_schema_version ORDER BY version`)
	if err != nil {
		return fmt.Errorf("query applied versions: %w", err)
	}
	defer rows.Close()

	applied := map[int]bool{}
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			return fmt.Errorf("scan version: %w", err)
		}
		applied[v] = true
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for _, m := range List {
		if applied[m.Version] {
			continue
		}
		if _, err := db.Exec(m.Up); err != nil {
			return fmt.Errorf("migration %d %s: %w", m.Version, m.Name, err)
		}
		if _, err := db.Exec(`INSERT INTO v2_schema_version (version, name) VALUES (?, ?)`, m.Version, m.Name); err != nil {
			return fmt.Errorf("record migration %d: %w", m.Version, err)
		}
	}
	return nil
}
