package app

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"go-agent/internal/v2/contracts"
	v2store "go-agent/internal/v2/store"
)

func TestMarkInterruptedRecoversRunningTaskAndWorkflowRun(t *testing.T) {
	ctx := context.Background()
	s, err := v2store.OpenInMemory()
	if err != nil {
		t.Fatalf("OpenInMemory: %v", err)
	}
	defer s.Close()

	workspaceRoot := t.TempDir()
	now := time.Now().UTC()

	session := contracts.Session{
		ID:            "session-recovery",
		WorkspaceRoot: workspaceRoot,
		State:         "running",
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := s.Sessions().Create(ctx, session); err != nil {
		t.Fatalf("create session: %v", err)
	}

	turn := contracts.Turn{
		ID:          "turn-running",
		SessionID:   session.ID,
		Kind:        "user_message",
		State:       "running",
		UserMessage: "continue",
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := s.Turns().Create(ctx, turn); err != nil {
		t.Fatalf("create turn: %v", err)
	}

	task := contracts.Task{
		ID:            "task-running",
		WorkspaceRoot: workspaceRoot,
		SessionID:     session.ID,
		Kind:          "agent",
		State:         "running",
		Prompt:        "Finish the draft.",
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := s.Tasks().Create(ctx, task); err != nil {
		t.Fatalf("create task: %v", err)
	}

	workflow := contracts.Workflow{
		ID:            "workflow-running",
		WorkspaceRoot: workspaceRoot,
		Name:          "Recovery workflow",
		Trigger:       "manual",
		Steps:         `[{"kind":"role_task","role_id":"reviewer","prompt":"Review"}]`,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := s.Workflows().Create(ctx, workflow); err != nil {
		t.Fatalf("create workflow: %v", err)
	}

	runningRun := contracts.WorkflowRun{
		ID:            "wfrun-running",
		WorkflowID:    workflow.ID,
		WorkspaceRoot: workspaceRoot,
		State:         "running",
		TaskIDs:       `["task-running"]`,
		Trace:         `[{"event":"run_created","state":"queued"}]`,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := s.Workflows().CreateRun(ctx, runningRun); err != nil {
		t.Fatalf("create running workflow run: %v", err)
	}

	waitingRun := contracts.WorkflowRun{
		ID:            "wfrun-waiting",
		WorkflowID:    workflow.ID,
		WorkspaceRoot: workspaceRoot,
		State:         "waiting_approval",
		ApprovalIDs:   `["approval-1"]`,
		Trace:         `[{"event":"approval_requested","state":"pending"}]`,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := s.Workflows().CreateRun(ctx, waitingRun); err != nil {
		t.Fatalf("create waiting workflow run: %v", err)
	}

	if err := markInterrupted(ctx, s); err != nil {
		t.Fatalf("markInterrupted: %v", err)
	}

	gotTurn, err := s.Turns().Get(ctx, turn.ID)
	if err != nil {
		t.Fatalf("get turn: %v", err)
	}
	if gotTurn.State != "interrupted" {
		t.Fatalf("turn state = %q, want interrupted", gotTurn.State)
	}

	gotTask, err := s.Tasks().Get(ctx, task.ID)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if gotTask.State != "failed" || gotTask.Outcome != "failure" {
		t.Fatalf("task = %+v", gotTask)
	}
	if gotTask.FinalText != "Task interrupted because the daemon restarted." {
		t.Fatalf("task final_text = %q", gotTask.FinalText)
	}

	gotRunningRun, err := s.Workflows().GetRun(ctx, runningRun.ID)
	if err != nil {
		t.Fatalf("get running workflow run: %v", err)
	}
	if gotRunningRun.State != "interrupted" {
		t.Fatalf("workflow run state = %q, want interrupted", gotRunningRun.State)
	}
	trace := decodeWorkflowRunTrace(t, gotRunningRun.Trace)
	if !hasWorkflowRunTraceState(trace, "run_interrupted", "interrupted") {
		t.Fatalf("workflow run trace = %+v", trace)
	}

	gotWaitingRun, err := s.Workflows().GetRun(ctx, waitingRun.ID)
	if err != nil {
		t.Fatalf("get waiting workflow run: %v", err)
	}
	if gotWaitingRun.State != "waiting_approval" {
		t.Fatalf("waiting workflow run state = %q, want waiting_approval", gotWaitingRun.State)
	}
}

type workflowRunTraceEvent struct {
	Event string `json:"event"`
	State string `json:"state,omitempty"`
}

func decodeWorkflowRunTrace(t *testing.T, raw string) []workflowRunTraceEvent {
	t.Helper()
	var out []workflowRunTraceEvent
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		t.Fatalf("decode workflow run trace %q: %v", raw, err)
	}
	return out
}

func hasWorkflowRunTraceState(trace []workflowRunTraceEvent, event, state string) bool {
	for _, item := range trace {
		if item.Event == event && item.State == state {
			return true
		}
	}
	return false
}
