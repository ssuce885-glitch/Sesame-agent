package tools

import (
	"context"
	"strings"
	"testing"
	"time"

	"go-agent/internal/v2/contracts"
	"go-agent/internal/v2/store"
)

func TestAutomationCreateSimpleRequiresRoleSkillAndSetsRoleOwner(t *testing.T) {
	runtime := &fakeAutomationRuntime{}
	tool := NewAutomationCreateSimpleTool()
	call := contracts.ToolCall{
		Name: "automation_create_simple",
		Args: map[string]any{
			"title":        "Check inbox",
			"goal":         "Find important updates.",
			"watcher_path": "roles/inbox/automations/check.py",
		},
	}
	execCtx := contracts.ExecContext{
		WorkspaceRoot: "/workspace",
		Automation:    runtime,
		RoleSpec:      &contracts.RoleSpec{ID: "inbox"},
	}

	result, err := tool.Execute(context.Background(), call, execCtx)
	if err != nil {
		t.Fatalf("Execute without skill returned error: %v", err)
	}
	if !result.IsError || !strings.Contains(result.Output, "automation-standard-behavior skill is required") {
		t.Fatalf("expected missing skill error, got %+v", result)
	}

	execCtx.ActiveSkills = []string{automationStandardBehaviorSkill}
	result, err = tool.Execute(context.Background(), call, execCtx)
	if err != nil {
		t.Fatalf("Execute with skill returned error: %v", err)
	}
	if result.IsError {
		t.Fatalf("automation_create_simple failed: %s", result.Output)
	}
	if len(runtime.created) != 1 || runtime.created[0].Owner != "role:inbox" {
		t.Fatalf("unexpected created automations: %+v", runtime.created)
	}
}

func TestAutomationControlRequiresRoleOwnership(t *testing.T) {
	s, err := store.OpenInMemory()
	if err != nil {
		t.Fatalf("OpenInMemory: %v", err)
	}
	defer s.Close()

	now := time.Now().UTC()
	for _, automation := range []contracts.Automation{
		{
			ID:            "automation_owned",
			WorkspaceRoot: "/workspace",
			Title:         "Owned",
			Goal:          "Owned automation",
			State:         "active",
			Owner:         "role:owned_role",
			WatcherPath:   "watch.py",
			WatcherCron:   "*/5 * * * *",
			CreatedAt:     now,
			UpdatedAt:     now,
		},
		{
			ID:            "automation_other",
			WorkspaceRoot: "/workspace",
			Title:         "Other",
			Goal:          "Other automation",
			State:         "active",
			Owner:         "role:other_role",
			WatcherPath:   "watch.py",
			WatcherCron:   "*/5 * * * *",
			CreatedAt:     now,
			UpdatedAt:     now,
		},
		{
			ID:            "automation_by_id",
			WorkspaceRoot: "/workspace",
			Title:         "By ID",
			Goal:          "Explicit automation ownership",
			State:         "active",
			Owner:         "role:other_role",
			WatcherPath:   "watch.py",
			WatcherCron:   "*/5 * * * *",
			CreatedAt:     now,
			UpdatedAt:     now,
		},
	} {
		if err := s.Automations().Create(context.Background(), automation); err != nil {
			t.Fatalf("Create automation: %v", err)
		}
	}

	runtime := &fakeAutomationRuntime{}
	tool := NewAutomationControlTool()
	execCtx := contracts.ExecContext{
		WorkspaceRoot:   "/workspace",
		Store:           s,
		Automation:      runtime,
		ActiveSkills:    []string{automationStandardBehaviorSkill},
		RoleSpec:        &contracts.RoleSpec{ID: "owned_role", AutomationOwners: []string{"automation_by_id"}},
		PermissionLevel: "workspace",
	}

	result, err := tool.Execute(context.Background(), contracts.ToolCall{
		Name: "automation_control",
		Args: map[string]any{"id": "automation_owned", "action": "pause"},
	}, execCtx)
	if err != nil {
		t.Fatalf("Execute owned returned error: %v", err)
	}
	if result.IsError || len(runtime.paused) != 1 || runtime.paused[0] != "automation_owned" {
		t.Fatalf("expected owned pause, result=%+v runtime=%+v", result, runtime)
	}

	result, err = tool.Execute(context.Background(), contracts.ToolCall{
		Name: "automation_control",
		Args: map[string]any{"id": "automation_by_id", "action": "resume"},
	}, execCtx)
	if err != nil {
		t.Fatalf("Execute explicit ownership returned error: %v", err)
	}
	if result.IsError || len(runtime.resumed) != 1 || runtime.resumed[0] != "automation_by_id" {
		t.Fatalf("expected explicit ownership resume, result=%+v runtime=%+v", result, runtime)
	}

	result, err = tool.Execute(context.Background(), contracts.ToolCall{
		Name: "automation_control",
		Args: map[string]any{"id": "automation_other", "action": "pause"},
	}, execCtx)
	if err != nil {
		t.Fatalf("Execute other returned error: %v", err)
	}
	if !result.IsError || !strings.Contains(result.Output, "cannot control automation") {
		t.Fatalf("expected ownership denial, got %+v", result)
	}
}

func TestAutomationQueryIncludesWorkflowFields(t *testing.T) {
	s, err := store.OpenInMemory()
	if err != nil {
		t.Fatalf("OpenInMemory: %v", err)
	}
	defer s.Close()

	now := time.Now().UTC()
	automation := contracts.Automation{
		ID:            "automation_docs",
		WorkspaceRoot: "/workspace",
		Title:         "Watch docs",
		Goal:          "Start the docs workflow",
		State:         "active",
		Owner:         "role:reviewer",
		WorkflowID:    "workflow_docs",
		WatcherPath:   "roles/reviewer/automations/watch.sh",
		WatcherCron:   "@every 5m",
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := s.Automations().Create(context.Background(), automation); err != nil {
		t.Fatalf("Create automation: %v", err)
	}
	if err := s.Automations().CreateRun(context.Background(), contracts.AutomationRun{
		AutomationID:  automation.ID,
		DedupeKey:     "docs-stale",
		WorkflowRunID: "wfrun_docs_1",
		Status:        "workflow:queued",
		Summary:       "Docs changed.",
		CreatedAt:     now,
	}); err != nil {
		t.Fatalf("Create automation run: %v", err)
	}

	result, err := NewAutomationQueryTool().Execute(context.Background(), contracts.ToolCall{Name: "automation_query"}, contracts.ExecContext{
		WorkspaceRoot: "/workspace",
		Store:         s,
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.IsError {
		t.Fatalf("Execute returned error result: %+v", result)
	}
	data, ok := result.Data.(automationQueryResult)
	if !ok {
		t.Fatalf("result.Data type = %T, want automationQueryResult", result.Data)
	}
	if len(data.Automations) != 1 {
		t.Fatalf("automations len = %d, want 1", len(data.Automations))
	}
	if data.Automations[0].WorkflowID != "workflow_docs" {
		t.Fatalf("workflow_id = %q, want workflow_docs", data.Automations[0].WorkflowID)
	}
	if len(data.Automations[0].RecentRuns) != 1 {
		t.Fatalf("recent runs = %+v, want 1", data.Automations[0].RecentRuns)
	}
	if data.Automations[0].RecentRuns[0].WorkflowRunID != "wfrun_docs_1" {
		t.Fatalf("workflow_run_id = %q, want wfrun_docs_1", data.Automations[0].RecentRuns[0].WorkflowRunID)
	}
}

type fakeAutomationRuntime struct {
	created []contracts.Automation
	paused  []string
	resumed []string
}

func (f *fakeAutomationRuntime) Create(_ context.Context, automation contracts.Automation) error {
	f.created = append(f.created, automation)
	return nil
}

func (f *fakeAutomationRuntime) Pause(_ context.Context, id string) error {
	f.paused = append(f.paused, id)
	return nil
}

func (f *fakeAutomationRuntime) Resume(_ context.Context, id string) error {
	f.resumed = append(f.resumed, id)
	return nil
}
