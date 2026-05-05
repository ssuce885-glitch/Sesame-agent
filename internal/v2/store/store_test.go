package store

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
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
		"v2_context_blocks",
		"v2_workflows",
		"v2_workflow_runs",
		"v2_approvals",
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

func TestOpenMigratesWorkflowRunDedupeRefIndexCompatibly(t *testing.T) {
	dsn := filepath.Join(t.TempDir(), "workflow-trigger-ref-migration.sqlite")

	db, err := sql.Open("sqlite", sqliteDSN(dsn))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}

	setup := []string{
		`CREATE TABLE v2_schema_version (version INTEGER PRIMARY KEY, name TEXT NOT NULL, applied_at TEXT NOT NULL DEFAULT (datetime('now')));`,
		`CREATE TABLE v2_workflows (
			id TEXT PRIMARY KEY,
			workspace_root TEXT NOT NULL,
			name TEXT NOT NULL DEFAULT '',
			trigger TEXT NOT NULL DEFAULT 'manual',
			owner_role TEXT NOT NULL DEFAULT '',
			input_schema TEXT NOT NULL DEFAULT '',
			steps TEXT NOT NULL DEFAULT '',
			required_tools TEXT NOT NULL DEFAULT '',
			approval_policy TEXT NOT NULL DEFAULT '',
			report_policy TEXT NOT NULL DEFAULT '',
			failure_policy TEXT NOT NULL DEFAULT '',
			resume_policy TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			UNIQUE(id, workspace_root)
		);`,
		`CREATE TABLE v2_workflow_runs (
			id TEXT PRIMARY KEY,
			workflow_id TEXT NOT NULL,
			workspace_root TEXT NOT NULL,
			state TEXT NOT NULL DEFAULT 'queued',
			trigger_ref TEXT NOT NULL DEFAULT '',
			task_ids TEXT NOT NULL DEFAULT '',
			report_ids TEXT NOT NULL DEFAULT '',
			approval_ids TEXT NOT NULL DEFAULT '',
			trace TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			FOREIGN KEY (workflow_id, workspace_root) REFERENCES v2_workflows(id, workspace_root) ON UPDATE RESTRICT ON DELETE CASCADE
		);`,
		`INSERT INTO v2_workflows (
			id, workspace_root, name, trigger, owner_role, input_schema, steps, required_tools,
			approval_policy, report_policy, failure_policy, resume_policy, created_at, updated_at
		) VALUES (
			'workflow-1', '/workspace', 'Workflow', 'manual', '', '', '[{"kind":"role_task","role_id":"reviewer","prompt":"Review"}]',
			'', '', '', '', '', '2026-05-04T10:00:00Z', '2026-05-04T10:00:00Z'
		);`,
		`INSERT INTO v2_workflow_runs (
			id, workflow_id, workspace_root, state, trigger_ref, task_ids, report_ids, approval_ids, trace, created_at, updated_at
		) VALUES
			('wfrun-1', 'workflow-1', '/workspace', 'completed', 'automation:docs-stale', '[]', '[]', '[]', '[{"event":"run_completed"}]', '2026-05-04T10:00:00Z', '2026-05-04T10:00:00Z'),
			('wfrun-2', 'workflow-1', '/workspace', 'failed', 'automation:docs-stale', '[]', '[]', '[]', '[{"event":"run_failed"}]', '2026-05-04T10:01:00Z', '2026-05-04T10:01:00Z'),
			('wfrun-3', 'workflow-1', '/workspace', 'completed', 'webhook:repo-main', '[]', '[]', '[]', '[{"event":"run_completed"}]', '2026-05-04T10:02:00Z', '2026-05-04T10:02:00Z'),
			('wfrun-4', 'workflow-1', '/workspace', 'failed', 'webhook:repo-main', '[]', '[]', '[]', '[{"event":"run_failed"}]', '2026-05-04T10:03:00Z', '2026-05-04T10:03:00Z');`,
	}
	for _, stmt := range setup {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("setup stmt failed: %v", err)
		}
	}
	for version := 1; version <= 11; version++ {
		if _, err := db.Exec(`INSERT INTO v2_schema_version (version, name) VALUES (?, ?)`, version, fmt.Sprintf("migration-%03d", version)); err != nil {
			t.Fatalf("insert schema version %d: %v", version, err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close setup db: %v", err)
	}

	s, err := Open(dsn)
	if err != nil {
		t.Fatalf("Open migrated store: %v", err)
	}
	defer s.Close()

	ctx := context.Background()
	primary, err := s.Workflows().GetRunByDedupeRef(ctx, "workflow-1", "automation:docs-stale")
	if err != nil {
		t.Fatalf("GetRunByDedupeRef primary: %v", err)
	}
	if primary.ID != "wfrun-1" {
		t.Fatalf("primary run id = %q, want wfrun-1", primary.ID)
	}
	if primary.TriggerRef != "automation:docs-stale" || primary.DedupeRef != "automation:docs-stale" {
		t.Fatalf("primary run = %+v, want preserved trigger_ref with dedupe_ref", primary)
	}

	legacy, err := s.Workflows().GetRun(ctx, "wfrun-2")
	if err != nil {
		t.Fatalf("GetRun legacy: %v", err)
	}
	if legacy.TriggerRef != "automation:docs-stale" {
		t.Fatalf("legacy trigger_ref = %q", legacy.TriggerRef)
	}
	if legacy.DedupeRef != "" {
		t.Fatalf("legacy dedupe_ref = %q, want empty", legacy.DedupeRef)
	}

	webhookPrimary, err := s.Workflows().GetRun(ctx, "wfrun-3")
	if err != nil {
		t.Fatalf("GetRun webhook primary: %v", err)
	}
	if webhookPrimary.TriggerRef != "webhook:repo-main" {
		t.Fatalf("webhook primary trigger_ref = %q", webhookPrimary.TriggerRef)
	}
	if webhookPrimary.DedupeRef != "" {
		t.Fatalf("webhook primary dedupe_ref = %q, want empty", webhookPrimary.DedupeRef)
	}

	webhookLegacy, err := s.Workflows().GetRun(ctx, "wfrun-4")
	if err != nil {
		t.Fatalf("GetRun webhook legacy: %v", err)
	}
	if webhookLegacy.TriggerRef != "webhook:repo-main" {
		t.Fatalf("webhook legacy trigger_ref = %q", webhookLegacy.TriggerRef)
	}
	if webhookLegacy.DedupeRef != "" {
		t.Fatalf("webhook legacy dedupe_ref = %q, want empty", webhookLegacy.DedupeRef)
	}

	run, created, err := s.Workflows().GetOrCreateRunByDedupeRef(ctx, contracts.WorkflowRun{
		ID:            "wfrun-5",
		WorkflowID:    "workflow-1",
		WorkspaceRoot: "/workspace",
		State:         "queued",
		TriggerRef:    "automation:docs-stale",
		DedupeRef:     "automation:docs-stale",
		Trace:         `[{"event":"run_created"}]`,
		CreatedAt:     time.Date(2026, 5, 4, 10, 4, 0, 0, time.UTC),
		UpdatedAt:     time.Date(2026, 5, 4, 10, 4, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("GetOrCreateRunByDedupeRef after migration: %v", err)
	}
	if created || run.ID != "wfrun-1" {
		t.Fatalf("GetOrCreateRunByDedupeRef returned %+v created=%v, want existing wfrun-1", run, created)
	}

	webhookRun, created, err := s.Workflows().GetOrCreateRunByDedupeRef(ctx, contracts.WorkflowRun{
		ID:            "wfrun-6",
		WorkflowID:    "workflow-1",
		WorkspaceRoot: "/workspace",
		State:         "queued",
		TriggerRef:    "webhook:repo-main",
		Trace:         `[{"event":"run_created"}]`,
		CreatedAt:     time.Date(2026, 5, 4, 10, 5, 0, 0, time.UTC),
		UpdatedAt:     time.Date(2026, 5, 4, 10, 5, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("GetOrCreateRunByDedupeRef webhook after migration: %v", err)
	}
	if !created || webhookRun.ID != "wfrun-6" {
		t.Fatalf("GetOrCreateRunByDedupeRef webhook returned %+v created=%v, want created wfrun-6", webhookRun, created)
	}
	if webhookRun.TriggerRef != "webhook:repo-main" || webhookRun.DedupeRef != "" {
		t.Fatalf("webhook created run = %+v, want trigger_ref preserved with empty dedupe_ref", webhookRun)
	}
}
