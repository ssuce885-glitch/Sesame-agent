package store

import (
	"context"
	"database/sql"
	"fmt"
	"go-agent/internal/v2/contracts"
	"go-agent/internal/v2/schema"
	"net/url"
	"strings"

	_ "modernc.org/sqlite"
)

// Store implements contracts.Store.
type Store struct {
	db          *sql.DB
	dbOverride  DBHandle
	sessions    *sessionRepo
	turns       *turnRepo
	messages    *messageRepo
	events      *eventRepo
	tasks       *taskRepo
	reports     *reportRepo
	automations *automationRepo
	memories    *memoryRepo
	settings    *settingRepo
	project     *projectStateRepo
}

var _ contracts.Store = (*Store)(nil)

// DBHandle is the SQL execution surface exposed by Store.DB.
type DBHandle interface {
	Exec(query string, args ...any) (sql.Result, error)
	Query(query string, args ...any) (*sql.Rows, error)
	QueryRow(query string, args ...any) *sql.Row
}

// Open creates a new Store and runs pending migrations.
func Open(dsn string) (*Store, error) {
	db, err := sql.Open("sqlite", sqliteDSN(dsn))
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}
	// WAL mode for concurrent reads.
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		return nil, fmt.Errorf("enable sqlite WAL: %w", err)
	}
	if _, err := db.Exec("PRAGMA foreign_keys=ON"); err != nil {
		return nil, fmt.Errorf("enable sqlite foreign keys: %w", err)
	}
	if _, err := db.Exec("PRAGMA busy_timeout=10000"); err != nil {
		return nil, fmt.Errorf("set sqlite busy timeout: %w", err)
	}

	if err := schema.Run(db); err != nil {
		return nil, fmt.Errorf("run schema migrations: %w", err)
	}

	s := &Store{db: db}
	s.sessions = &sessionRepo{db: db}
	s.turns = &turnRepo{db: db}
	s.messages = &messageRepo{db: db}
	s.events = &eventRepo{db: db}
	s.tasks = &taskRepo{db: db}
	s.reports = &reportRepo{db: db}
	s.automations = &automationRepo{db: db}
	s.memories = &memoryRepo{db: db}
	s.settings = &settingRepo{db: db}
	s.project = &projectStateRepo{db: db}
	return s, nil
}

// OpenInMemory creates a Store backed by an in-memory SQLite database (for tests).
func OpenInMemory() (*Store, error) {
	return Open(":memory:?_journal_mode=WAL&_foreign_keys=ON")
}

func (s *Store) DB() DBHandle {
	if s.dbOverride != nil {
		return s.dbOverride
	}
	return s.db
}

func (s *Store) Close() error { return s.db.Close() }

// Repository accessors
func (s *Store) Sessions() contracts.SessionRepository       { return s.sessions }
func (s *Store) Turns() contracts.TurnRepository             { return s.turns }
func (s *Store) Messages() contracts.MessageRepository       { return s.messages }
func (s *Store) Events() contracts.EventRepository           { return s.events }
func (s *Store) Tasks() contracts.TaskRepository             { return s.tasks }
func (s *Store) Reports() contracts.ReportRepository         { return s.reports }
func (s *Store) Automations() contracts.AutomationRepository { return s.automations }
func (s *Store) Memories() contracts.MemoryRepository        { return s.memories }
func (s *Store) Settings() contracts.SettingRepository       { return s.settings }
func (s *Store) ProjectStates() contracts.ProjectStateRepository {
	return s.project
}

// WithTx runs fn in a transaction. If fn returns an error, the transaction is rolled back.
func (s *Store) WithTx(ctx context.Context, fn func(tx contracts.Store) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	txStore := &Store{db: s.db, dbOverride: tx}
	txStore.sessions = &sessionRepo{db: s.db, tx: tx}
	txStore.turns = &turnRepo{db: s.db, tx: tx}
	txStore.messages = &messageRepo{db: s.db, tx: tx}
	txStore.events = &eventRepo{db: s.db, tx: tx}
	txStore.tasks = &taskRepo{db: s.db, tx: tx}
	txStore.reports = &reportRepo{db: s.db, tx: tx}
	txStore.automations = &automationRepo{db: s.db, tx: tx}
	txStore.memories = &memoryRepo{db: s.db, tx: tx}
	txStore.settings = &settingRepo{db: s.db, tx: tx}
	txStore.project = &projectStateRepo{db: s.db, tx: tx}

	if err := fn(txStore); err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit()
}

func sqliteDSN(dsn string) string {
	if strings.Contains(dsn, "_pragma=busy_timeout") {
		return dsn
	}
	separator := "?"
	if strings.Contains(dsn, "?") {
		separator = "&"
	}
	return dsn + separator + "_pragma=" + url.QueryEscape("busy_timeout=10000")
}
