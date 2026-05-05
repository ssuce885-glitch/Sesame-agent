package store

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"go-agent/internal/v2/contracts"
)

func TestWorkflowRepositoryCRUDAndRuns(t *testing.T) {
	s, err := OpenInMemory()
	if err != nil {
		t.Fatalf("OpenInMemory: %v", err)
	}
	defer s.Close()

	ctx := context.Background()
	now := time.Date(2026, 5, 4, 9, 0, 0, 0, time.UTC)
	workflow := contracts.Workflow{
		ID:             "workflow-1",
		WorkspaceRoot:  "/workspace",
		Name:           "Review flow",
		Trigger:        "manual",
		OwnerRole:      "reviewer",
		InputSchema:    `{"type":"object"}`,
		Steps:          `[{"kind":"task","name":"review"}]`,
		RequiredTools:  `["git"]`,
		ApprovalPolicy: `{"required":true}`,
		ReportPolicy:   `{"on":"always"}`,
		FailurePolicy:  `{"retry":0}`,
		ResumePolicy:   `{"mode":"manual"}`,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := s.Workflows().Create(ctx, workflow); err != nil {
		t.Fatalf("create workflow: %v", err)
	}

	got, err := s.Workflows().Get(ctx, workflow.ID)
	if err != nil {
		t.Fatalf("get workflow: %v", err)
	}
	if got.Name != workflow.Name || got.Steps != workflow.Steps {
		t.Fatalf("got workflow = %+v", got)
	}

	workflow.Name = "Review flow updated"
	workflow.Trigger = "file_change"
	workflow.UpdatedAt = now.Add(time.Hour)
	if err := s.Workflows().Update(ctx, workflow); err != nil {
		t.Fatalf("update workflow: %v", err)
	}

	workflows, err := s.Workflows().ListByWorkspace(ctx, "/workspace")
	if err != nil {
		t.Fatalf("list workflows: %v", err)
	}
	if len(workflows) != 1 || workflows[0].Name != "Review flow updated" {
		t.Fatalf("list workflows = %+v", workflows)
	}

	run := contracts.WorkflowRun{
		ID:            "wfrun-1",
		WorkflowID:    workflow.ID,
		WorkspaceRoot: "/workspace",
		State:         "running",
		TriggerRef:    "manual:1",
		TaskIDs:       `["task-1"]`,
		ReportIDs:     `["report-1"]`,
		ApprovalIDs:   `["approval-1"]`,
		Trace:         `[{"event":"started"}]`,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := s.Workflows().CreateRun(ctx, run); err != nil {
		t.Fatalf("create workflow run: %v", err)
	}

	gotRun, err := s.Workflows().GetRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("get workflow run: %v", err)
	}
	if gotRun.WorkflowID != workflow.ID || gotRun.Trace != run.Trace {
		t.Fatalf("got workflow run = %+v", gotRun)
	}

	run.State = "completed"
	run.UpdatedAt = now.Add(2 * time.Hour)
	if err := s.Workflows().UpdateRun(ctx, run); err != nil {
		t.Fatalf("update workflow run: %v", err)
	}

	runs, err := s.Workflows().ListRunsByWorkspace(ctx, "/workspace", contracts.WorkflowRunListOptions{
		WorkflowID: workflow.ID,
		State:      "completed",
		Limit:      10,
	})
	if err != nil {
		t.Fatalf("list workflow runs: %v", err)
	}
	if len(runs) != 1 || runs[0].ID != run.ID || runs[0].State != "completed" {
		t.Fatalf("list workflow runs = %+v", runs)
	}

	approval := contracts.Approval{
		ID:              "approval-1",
		WorkflowRunID:   run.ID,
		WorkspaceRoot:   "/workspace",
		RequestedAction: "deploy release",
		RiskLevel:       "high",
		Summary:         "Deploy to production",
		ProposedPayload: `{"environment":"prod"}`,
		State:           "pending",
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if err := s.Workflows().CreateApproval(ctx, approval); err != nil {
		t.Fatalf("create approval: %v", err)
	}

	gotApproval, err := s.Workflows().GetApproval(ctx, approval.ID)
	if err != nil {
		t.Fatalf("get approval: %v", err)
	}
	if gotApproval.WorkflowRunID != run.ID || gotApproval.RequestedAction != approval.RequestedAction {
		t.Fatalf("got approval = %+v", gotApproval)
	}

	approval.State = "approved"
	approval.DecidedBy = "operator"
	approval.DecidedAt = now.Add(3 * time.Hour)
	approval.UpdatedAt = now.Add(3 * time.Hour)
	if err := s.Workflows().UpdateApproval(ctx, approval); err != nil {
		t.Fatalf("update approval: %v", err)
	}

	approvals, err := s.Workflows().ListApprovalsByWorkspace(ctx, "/workspace", contracts.ApprovalListOptions{
		WorkflowRunID: run.ID,
		State:         "approved",
		Limit:         10,
	})
	if err != nil {
		t.Fatalf("list approvals: %v", err)
	}
	if len(approvals) != 1 || approvals[0].ID != approval.ID || approvals[0].DecidedBy != "operator" {
		t.Fatalf("list approvals = %+v", approvals)
	}

	workflow.ID = "missing"
	if err := s.Workflows().Update(ctx, workflow); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("update missing workflow err = %v, want sql.ErrNoRows", err)
	}
	run.ID = "missing-run"
	if err := s.Workflows().UpdateRun(ctx, run); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("update missing workflow run err = %v, want sql.ErrNoRows", err)
	}
	approval.ID = "missing-approval"
	if err := s.Workflows().UpdateApproval(ctx, approval); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("update missing approval err = %v, want sql.ErrNoRows", err)
	}
	if err := s.Workflows().CreateRun(ctx, contracts.WorkflowRun{
		ID:            "orphan-run",
		WorkflowID:    "missing-workflow",
		WorkspaceRoot: "/workspace",
		State:         "queued",
		CreatedAt:     now,
		UpdatedAt:     now,
	}); err == nil {
		t.Fatalf("create orphan workflow run succeeded")
	}
	if err := s.Workflows().CreateApproval(ctx, contracts.Approval{
		ID:              "orphan-approval",
		WorkflowRunID:   "missing-run",
		WorkspaceRoot:   "/workspace",
		RequestedAction: "deploy",
		State:           "pending",
		CreatedAt:       now,
		UpdatedAt:       now,
	}); err == nil {
		t.Fatalf("create orphan approval succeeded")
	}
}

func TestWorkflowRepositoryGetOrCreateRunByDedupeRefReusesAsyncRun(t *testing.T) {
	s, err := OpenInMemory()
	if err != nil {
		t.Fatalf("OpenInMemory: %v", err)
	}
	defer s.Close()

	ctx := context.Background()
	now := time.Date(2026, 5, 4, 10, 0, 0, 0, time.UTC)
	workflow := contracts.Workflow{
		ID:            "workflow-trigger-ref",
		WorkspaceRoot: "/workspace",
		Name:          "Trigger ref workflow",
		Trigger:       "manual",
		Steps:         `[{"kind":"role_task","role_id":"reviewer","prompt":"Review"}]`,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := s.Workflows().Create(ctx, workflow); err != nil {
		t.Fatalf("create workflow: %v", err)
	}

	run := contracts.WorkflowRun{
		ID:            "wfrun-1",
		WorkflowID:    workflow.ID,
		WorkspaceRoot: workflow.WorkspaceRoot,
		State:         "queued",
		TriggerRef:    "automation:docs-stale",
		DedupeRef:     "automation:docs-stale",
		Trace:         `[{"event":"run_created"}]`,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	got, created, err := s.Workflows().GetOrCreateRunByDedupeRef(ctx, run)
	if err != nil {
		t.Fatalf("GetOrCreateRunByDedupeRef create: %v", err)
	}
	if !created || got.ID != run.ID {
		t.Fatalf("first GetOrCreateRunByDedupeRef = %+v, created=%v", got, created)
	}

	second, created, err := s.Workflows().GetOrCreateRunByDedupeRef(ctx, contracts.WorkflowRun{
		ID:            "wfrun-2",
		WorkflowID:    workflow.ID,
		WorkspaceRoot: workflow.WorkspaceRoot,
		State:         "queued",
		TriggerRef:    run.TriggerRef,
		DedupeRef:     run.DedupeRef,
		Trace:         `[{"event":"run_created"}]`,
		CreatedAt:     now.Add(time.Minute),
		UpdatedAt:     now.Add(time.Minute),
	})
	if err != nil {
		t.Fatalf("GetOrCreateRunByDedupeRef reuse: %v", err)
	}
	if created {
		t.Fatalf("second GetOrCreateRunByDedupeRef created=%v, want false", created)
	}
	if second.ID != run.ID {
		t.Fatalf("second run id = %q, want %q", second.ID, run.ID)
	}

	byDedupeRef, err := s.Workflows().GetRunByDedupeRef(ctx, workflow.ID, run.DedupeRef)
	if err != nil {
		t.Fatalf("GetRunByDedupeRef: %v", err)
	}
	if byDedupeRef.ID != run.ID {
		t.Fatalf("GetRunByDedupeRef id = %q, want %q", byDedupeRef.ID, run.ID)
	}

	if err := s.Workflows().CreateRun(ctx, contracts.WorkflowRun{
		ID:            "wfrun-3",
		WorkflowID:    workflow.ID,
		WorkspaceRoot: workflow.WorkspaceRoot,
		State:         "queued",
		TriggerRef:    run.TriggerRef,
		DedupeRef:     run.DedupeRef,
		Trace:         `[{"event":"run_created"}]`,
		CreatedAt:     now.Add(2 * time.Minute),
		UpdatedAt:     now.Add(2 * time.Minute),
	}); err == nil {
		t.Fatalf("CreateRun with duplicate dedupe_ref succeeded")
	}

	runs, err := s.Workflows().ListRunsByWorkspace(ctx, workflow.WorkspaceRoot, contracts.WorkflowRunListOptions{})
	if err != nil {
		t.Fatalf("ListRunsByWorkspace: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("workflow runs = %+v, want one run", runs)
	}
}
