package store

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"go-agent/internal/v2/contracts"
)

func TestAutomationRepositoryCRUD(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	defer s.Close()

	now := time.Now().UTC()
	automation := contracts.Automation{
		ID:            "automation-1",
		WorkspaceRoot: "/workspace",
		Title:         "Watch docs",
		Goal:          "Keep docs fresh",
		State:         "active",
		Owner:         "role:doc_writer",
		WorkflowID:    "workflow-docs",
		WatcherPath:   "roles/doc_writer/automations/watch.sh",
		WatcherCron:   "@every 1h",
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := s.Automations().Create(ctx, automation); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := s.Automations().Get(ctx, automation.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ID != automation.ID || got.Owner != automation.Owner || got.WorkflowID != automation.WorkflowID {
		t.Fatalf("unexpected automation: %+v", got)
	}

	got.State = "paused"
	got.WorkflowID = "workflow-docs-updated"
	got.UpdatedAt = now.Add(time.Minute)
	if err := s.Automations().Update(ctx, got); err != nil {
		t.Fatalf("Update: %v", err)
	}
	got, err = s.Automations().Get(ctx, automation.ID)
	if err != nil {
		t.Fatalf("Get updated: %v", err)
	}
	if got.State != "paused" || got.WorkflowID != "workflow-docs-updated" {
		t.Fatalf("expected paused, got %+v", got)
	}

	list, err := s.Automations().ListByWorkspace(ctx, "/workspace")
	if err != nil {
		t.Fatalf("ListByWorkspace: %v", err)
	}
	if len(list) != 1 || list[0].ID != automation.ID {
		t.Fatalf("unexpected workspace automations: %+v", list)
	}

	all, err := s.Automations().ListByWorkspace(ctx, "")
	if err != nil {
		t.Fatalf("List all: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("expected all automations, got %+v", all)
	}

	run := contracts.AutomationRun{
		AutomationID:  automation.ID,
		DedupeKey:     "docs-stale",
		TaskID:        "task-1",
		WorkflowRunID: "wfrun-1",
		Status:        "needs_agent",
		Summary:       "Docs are stale.",
		CreatedAt:     now,
	}
	if err := s.Automations().CreateRun(ctx, run); err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	gotRun, err := s.Automations().GetRunByDedupeKey(ctx, automation.ID, run.DedupeKey)
	if err != nil {
		t.Fatalf("GetRunByDedupeKey: %v", err)
	}
	if gotRun.TaskID != run.TaskID || gotRun.WorkflowRunID != run.WorkflowRunID || gotRun.Status != run.Status {
		t.Fatalf("unexpected run: %+v", gotRun)
	}
	runs, err := s.Automations().ListRunsByAutomation(ctx, automation.ID, 10)
	if err != nil {
		t.Fatalf("ListRunsByAutomation: %v", err)
	}
	if len(runs) != 1 || runs[0].DedupeKey != run.DedupeKey {
		t.Fatalf("unexpected runs: %+v", runs)
	}
	if _, err := s.Automations().GetRunByDedupeKey(ctx, automation.ID, "missing"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected sql.ErrNoRows, got %v", err)
	}
}
