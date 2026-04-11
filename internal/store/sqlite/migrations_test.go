package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"go-agent/internal/types"
)

func TestMigrateBackfillsLegacyReportingSessionsAndMailboxRecords(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "legacy-agentd.db")
	legacyDB, err := sql.Open("sqlite", sqliteDSN(dbPath))
	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}

	ctx := context.Background()
	now := time.Date(2026, 4, 10, 11, 0, 0, 0, time.UTC)
	job := types.ScheduledJob{
		ID:             "cron_legacy_worker",
		Name:           "Legacy worker",
		WorkspaceRoot:  "/tmp/legacy",
		OwnerSessionID: "sess_legacy",
		Kind:           types.ScheduleKindEvery,
		Prompt:         "legacy prompt",
		EveryMinutes:   30,
		Enabled:        true,
		NextRunAt:      now.Add(30 * time.Minute),
		LastStatus:     types.ScheduledJobStatusPending,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	jobPayload, err := json.Marshal(job)
	if err != nil {
		t.Fatalf("json.Marshal(job) error = %v", err)
	}
	specPayload, err := json.Marshal(types.ChildAgentSpec{
		AgentID:      job.ID,
		Purpose:      "Legacy worker",
		Mode:         types.ChildAgentModeBackgroundWorker,
		ReportGroups: []string{"legacy-group"},
		CreatedAt:    now,
		UpdatedAt:    now,
	})
	if err != nil {
		t.Fatalf("json.Marshal(spec) error = %v", err)
	}
	groupPayload, err := json.Marshal(types.ReportGroup{
		GroupID:   "legacy-group",
		Title:     "Legacy Group",
		Sources:   []string{job.ID},
		CreatedAt: now,
		UpdatedAt: now,
	})
	if err != nil {
		t.Fatalf("json.Marshal(group) error = %v", err)
	}
	resultPayload, err := json.Marshal(types.ChildAgentResult{
		ResultID:        "result_legacy_1",
		AgentID:         job.ID,
		ReportGroupRefs: []string{"legacy-group"},
		ObservedAt:      now,
		Envelope: types.ReportEnvelope{
			Title:   "Legacy result",
			Status:  "completed",
			Summary: "legacy summary",
		},
		CreatedAt: now,
		UpdatedAt: now,
	})
	if err != nil {
		t.Fatalf("json.Marshal(result) error = %v", err)
	}
	digestPayload, err := json.Marshal(types.DigestRecord{
		DigestID:    "digest_legacy_1",
		GroupID:     "legacy-group",
		WindowStart: now.Add(-time.Hour),
		WindowEnd:   now,
		Envelope: types.ReportEnvelope{
			Title:   "Legacy digest",
			Status:  "completed",
			Summary: "digest summary",
		},
		CreatedAt: now,
		UpdatedAt: now,
	})
	if err != nil {
		t.Fatalf("json.Marshal(digest) error = %v", err)
	}
	mailboxPayload, err := json.Marshal(types.ReportMailboxItem{
		ID:         "task_result:legacy_task_1",
		SessionID:  "sess_legacy",
		SourceKind: types.ReportMailboxSourceTaskResult,
		SourceID:   "legacy_task_1",
		ObservedAt: now,
		Envelope: types.ReportEnvelope{
			Source:  "task_result",
			Status:  "completed",
			Title:   "Legacy mailbox",
			Summary: "mailbox summary",
		},
		CreatedAt: now,
		UpdatedAt: now,
	})
	if err != nil {
		t.Fatalf("json.Marshal(mailbox) error = %v", err)
	}

	stmts := []string{
		`create table scheduled_jobs (
			id text primary key,
			workspace_root text not null,
			owner_session_id text not null,
			kind text not null,
			enabled integer not null default 1,
			next_run_at text not null default '',
			last_status text not null default '',
			payload text not null,
			created_at text not null,
			updated_at text not null
		);`,
		`create table child_agent_specs (
			id text primary key,
			payload text not null,
			created_at text not null,
			updated_at text not null
		);`,
		`create table report_groups (
			id text primary key,
			payload text not null,
			created_at text not null,
			updated_at text not null
		);`,
		`create table child_agent_results (
			id text primary key,
			agent_id text not null,
			status text not null default '',
			severity text not null default '',
			observed_at text not null default '',
			payload text not null,
			created_at text not null,
			updated_at text not null
		);`,
		`create table digest_records (
			id text primary key,
			group_id text not null,
			status text not null default '',
			severity text not null default '',
			window_start text not null default '',
			window_end text not null default '',
			payload text not null,
			created_at text not null,
			updated_at text not null
		);`,
		`create table report_mailbox_items (
			id text primary key,
			session_id text not null,
			source_kind text not null,
			source_id text not null default '',
			severity text not null default '',
			observed_at text not null default '',
			injected_turn_id text not null default '',
			injected_at text not null default '',
			payload text not null,
			created_at text not null,
			updated_at text not null
		);`,
	}
	for _, stmt := range stmts {
		if _, err := legacyDB.ExecContext(ctx, stmt); err != nil {
			t.Fatalf("ExecContext(schema) error = %v", err)
		}
	}
	if _, err := legacyDB.ExecContext(ctx, `
		insert into scheduled_jobs (
			id, workspace_root, owner_session_id, kind, enabled, next_run_at, last_status, payload, created_at, updated_at
		) values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, job.ID, job.WorkspaceRoot, job.OwnerSessionID, job.Kind, 1, job.NextRunAt.Format(timeLayout), job.LastStatus, string(jobPayload), now.Format(timeLayout), now.Format(timeLayout)); err != nil {
		t.Fatalf("insert scheduled_jobs error = %v", err)
	}
	if _, err := legacyDB.ExecContext(ctx, `
		insert into child_agent_specs (id, payload, created_at, updated_at) values (?, ?, ?, ?)
	`, job.ID, string(specPayload), now.Format(timeLayout), now.Format(timeLayout)); err != nil {
		t.Fatalf("insert child_agent_specs error = %v", err)
	}
	if _, err := legacyDB.ExecContext(ctx, `
		insert into report_groups (id, payload, created_at, updated_at) values (?, ?, ?, ?)
	`, "legacy-group", string(groupPayload), now.Format(timeLayout), now.Format(timeLayout)); err != nil {
		t.Fatalf("insert report_groups error = %v", err)
	}
	if _, err := legacyDB.ExecContext(ctx, `
		insert into child_agent_results (id, agent_id, status, severity, observed_at, payload, created_at, updated_at)
		values (?, ?, ?, ?, ?, ?, ?, ?)
	`, "result_legacy_1", job.ID, "completed", "", now.Format(timeLayout), string(resultPayload), now.Format(timeLayout), now.Format(timeLayout)); err != nil {
		t.Fatalf("insert child_agent_results error = %v", err)
	}
	if _, err := legacyDB.ExecContext(ctx, `
		insert into digest_records (id, group_id, status, severity, window_start, window_end, payload, created_at, updated_at)
		values (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, "digest_legacy_1", "legacy-group", "completed", "", now.Add(-time.Hour).Format(timeLayout), now.Format(timeLayout), string(digestPayload), now.Format(timeLayout), now.Format(timeLayout)); err != nil {
		t.Fatalf("insert digest_records error = %v", err)
	}
	if _, err := legacyDB.ExecContext(ctx, `
		insert into report_mailbox_items (
			id, session_id, source_kind, source_id, severity, observed_at, injected_turn_id, injected_at, payload, created_at, updated_at
		) values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, "task_result:legacy_task_1", "sess_legacy", string(types.ReportMailboxSourceTaskResult), "legacy_task_1", "", now.Format(timeLayout), "", "", string(mailboxPayload), now.Format(timeLayout), now.Format(timeLayout)); err != nil {
		t.Fatalf("insert report_mailbox_items error = %v", err)
	}
	if err := legacyDB.Close(); err != nil {
		t.Fatalf("legacyDB.Close() error = %v", err)
	}

	store, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	spec, ok, err := store.GetChildAgentSpec(ctx, job.ID)
	if err != nil {
		t.Fatalf("GetChildAgentSpec() error = %v", err)
	}
	if !ok || spec.SessionID != "sess_legacy" {
		t.Fatalf("spec = %#v, want session backfilled to sess_legacy", spec)
	}
	group, ok, err := store.GetReportGroup(ctx, "legacy-group")
	if err != nil {
		t.Fatalf("GetReportGroup() error = %v", err)
	}
	if !ok || group.SessionID != "sess_legacy" {
		t.Fatalf("group = %#v, want session backfilled to sess_legacy", group)
	}
	result, ok, err := store.GetChildAgentResult(ctx, "result_legacy_1")
	if err != nil {
		t.Fatalf("GetChildAgentResult() error = %v", err)
	}
	if !ok || result.SessionID != "sess_legacy" {
		t.Fatalf("result = %#v, want session backfilled to sess_legacy", result)
	}
	digest, ok, err := store.GetDigestRecord(ctx, "digest_legacy_1")
	if err != nil {
		t.Fatalf("GetDigestRecord() error = %v", err)
	}
	if !ok || digest.SessionID != "sess_legacy" {
		t.Fatalf("digest = %#v, want session backfilled to sess_legacy", digest)
	}

	reports, err := store.ListReports(ctx, "sess_legacy")
	if err != nil {
		t.Fatalf("ListReports() error = %v", err)
	}
	if len(reports) != 1 || reports[0].Envelope.Title != "Legacy mailbox" {
		t.Fatalf("reports = %#v, want migrated legacy mailbox report", reports)
	}
	deliveries, err := store.ListReportDeliveries(ctx, "sess_legacy", types.ReportChannelMailbox)
	if err != nil {
		t.Fatalf("ListReportDeliveries() error = %v", err)
	}
	if len(deliveries) != 1 || deliveries[0].State != types.ReportDeliveryStatePending {
		t.Fatalf("deliveries = %#v, want one pending migrated delivery", deliveries)
	}
	items, err := store.ListReportMailboxItems(ctx, "sess_legacy")
	if err != nil {
		t.Fatalf("ListReportMailboxItems() error = %v", err)
	}
	if len(items) != 1 || items[0].Envelope.Title != "Legacy mailbox" {
		t.Fatalf("items = %#v, want migrated mailbox item", items)
	}
}
