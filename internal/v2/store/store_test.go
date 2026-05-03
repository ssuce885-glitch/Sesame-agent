package store

import (
	"context"
	"strings"
	"testing"
	"time"

	"go-agent/internal/v2/contracts"
)

func TestOpenInMemory(t *testing.T) {
	s, err := OpenInMemory()
	if err != nil {
		t.Fatalf("OpenInMemory: %v", err)
	}
	defer s.Close()

	tables := []string{
		"v2_sessions",
		"v2_turns",
		"v2_messages",
		"v2_message_snapshots",
		"v2_events",
		"v2_tasks",
		"v2_reports",
		"v2_automations",
		"v2_automation_runs",
		"v2_memories",
		"v2_settings",
		"v2_project_state",
	}
	for _, table := range tables {
		var name string
		err := s.db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name=?", table).Scan(&name)
		if err != nil {
			t.Errorf("table %s not found: %v", table, err)
		}
	}
}

func TestSchemaVersionTable(t *testing.T) {
	s, err := OpenInMemory()
	if err != nil {
		t.Fatalf("OpenInMemory: %v", err)
	}
	defer s.Close()

	var count int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM v2_schema_version").Scan(&count); err != nil {
		t.Fatalf("query schema_version: %v", err)
	}
	if count == 0 {
		t.Error("expected at least one schema version row")
	}
}

func TestOpenConfiguresSQLiteBusyHandling(t *testing.T) {
	s, err := OpenInMemory()
	if err != nil {
		t.Fatalf("OpenInMemory: %v", err)
	}
	defer s.Close()

	if got := s.db.Stats().MaxOpenConnections; got != 1 {
		t.Fatalf("expected max open connections 1, got %d", got)
	}
	var timeout int
	if err := s.db.QueryRow("PRAGMA busy_timeout").Scan(&timeout); err != nil {
		t.Fatalf("query busy_timeout: %v", err)
	}
	if timeout < 10000 {
		t.Fatalf("expected busy_timeout >= 10000, got %d", timeout)
	}
}

func TestSQLiteDSNAddsBusyTimeoutPragma(t *testing.T) {
	got := sqliteDSN("/tmp/sesame.db")
	if !strings.Contains(got, "_pragma=busy_timeout%3D10000") {
		t.Fatalf("expected busy_timeout pragma in %q", got)
	}
	withQuery := sqliteDSN("/tmp/sesame.db?cache=shared")
	if !strings.Contains(withQuery, "cache=shared&_pragma=busy_timeout%3D10000") {
		t.Fatalf("expected appended busy_timeout pragma in %q", withQuery)
	}
	existing := sqliteDSN("/tmp/sesame.db?_pragma=busy_timeout%3D5000")
	if existing != "/tmp/sesame.db?_pragma=busy_timeout%3D5000" {
		t.Fatalf("expected existing busy timeout to remain unchanged, got %q", existing)
	}
}

func TestTaskRepositoryRoundTripsTraceFields(t *testing.T) {
	s, err := OpenInMemory()
	if err != nil {
		t.Fatalf("OpenInMemory: %v", err)
	}
	defer s.Close()

	now := time.Now().UTC()
	task := contracts.Task{
		ID:              "task_trace_fields",
		WorkspaceRoot:   "/workspace",
		SessionID:       "role_session",
		RoleID:          "researcher",
		TurnID:          "role_turn",
		ParentSessionID: "main_session",
		ParentTurnID:    "main_turn",
		ReportSessionID: "main_session",
		Kind:            "agent",
		State:           "running",
		Prompt:          "inspect",
		OutputPath:      "/tmp/task.log",
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if err := s.Tasks().Create(context.Background(), task); err != nil {
		t.Fatalf("Create task: %v", err)
	}
	got, err := s.Tasks().Get(context.Background(), task.ID)
	if err != nil {
		t.Fatalf("Get task: %v", err)
	}
	if got.ParentSessionID != task.ParentSessionID || got.ParentTurnID != task.ParentTurnID || got.ReportSessionID != task.ReportSessionID {
		t.Fatalf("trace fields were not persisted: %+v", got)
	}

	got.ReportSessionID = "different_main_session"
	got.State = "completed"
	if err := s.Tasks().Update(context.Background(), got); err != nil {
		t.Fatalf("Update task: %v", err)
	}
	updated, err := s.Tasks().Get(context.Background(), task.ID)
	if err != nil {
		t.Fatalf("Get updated task: %v", err)
	}
	if updated.ReportSessionID != "different_main_session" || updated.ParentSessionID != task.ParentSessionID || updated.ParentTurnID != task.ParentTurnID {
		t.Fatalf("trace fields were not updated correctly: %+v", updated)
	}
}
