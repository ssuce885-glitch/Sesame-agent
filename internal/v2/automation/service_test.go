package automation

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"go-agent/internal/v2/contracts"
	"go-agent/internal/v2/roles"
	"go-agent/internal/v2/store"
	"go-agent/internal/v2/tasks"
)

type fakeRoleService struct {
	roles map[string]roles.RoleSpec
}

func (f fakeRoleService) List() ([]roles.RoleSpec, error) {
	out := make([]roles.RoleSpec, 0, len(f.roles))
	for _, spec := range f.roles {
		out = append(out, spec)
	}
	return out, nil
}

func (f fakeRoleService) Get(id string) (roles.RoleSpec, bool, error) {
	spec, ok := f.roles[id]
	return spec, ok, nil
}

func TestServiceAutomationLifecycle(t *testing.T) {
	ctx := context.Background()
	s, err := store.OpenInMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	workspaceRoot := t.TempDir()
	watcherPath := filepath.Join(workspaceRoot, "watcher.sh")
	writeWatcher(t, watcherPath, "docs-stale")

	manager := tasks.NewManager(s, t.TempDir())
	manager.RegisterRunner("agent", fakeAgentRunner{})
	roleService := fakeRoleService{roles: map[string]roles.RoleSpec{
		"doc_writer": {ID: "doc_writer", Name: "Doc Writer"},
	}}
	service := NewService(s, manager, roleService)
	automation := contracts.Automation{
		ID:            "automation-1",
		WorkspaceRoot: workspaceRoot,
		Title:         "Watch docs",
		Goal:          "Fix stale docs",
		Owner:         "role:doc_writer",
		WatcherPath:   watcherPath,
		State:         "active",
	}
	if err := service.Create(ctx, automation); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := service.Reconcile(ctx); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	tasksAfterFirst := manager.ListByWorkspace(workspaceRoot)
	if len(tasksAfterFirst) != 1 {
		t.Fatalf("expected one task after first reconcile, got %+v", tasksAfterFirst)
	}
	if tasksAfterFirst[0].RoleID != "doc_writer" {
		t.Fatalf("task RoleID = %q, want doc_writer", tasksAfterFirst[0].RoleID)
	}
	if tasksAfterFirst[0].SessionID != "" {
		t.Fatalf("automation task SessionID = %q, want empty", tasksAfterFirst[0].SessionID)
	}
	run, err := s.Automations().GetRunByDedupeKey(ctx, automation.ID, "docs-stale")
	if err != nil {
		t.Fatalf("GetRunByDedupeKey: %v", err)
	}
	if run.TaskID != tasksAfterFirst[0].ID || run.Status != "needs_agent" {
		t.Fatalf("unexpected run: %+v", run)
	}

	if err := service.Reconcile(ctx); err != nil {
		t.Fatalf("Reconcile dedupe: %v", err)
	}
	if got := manager.ListByWorkspace(workspaceRoot); len(got) != 1 {
		t.Fatalf("dedupe should prevent second task, got %+v", got)
	}

	if err := service.Pause(ctx, automation.ID); err != nil {
		t.Fatalf("Pause: %v", err)
	}
	writeWatcher(t, watcherPath, "docs-stale-again")
	if err := service.Reconcile(ctx); err != nil {
		t.Fatalf("Reconcile paused: %v", err)
	}
	if got := manager.ListByWorkspace(workspaceRoot); len(got) != 1 {
		t.Fatalf("paused automation should not create task, got %+v", got)
	}

	if err := service.Resume(ctx, automation.ID); err != nil {
		t.Fatalf("Resume: %v", err)
	}
	if err := service.Reconcile(ctx); err != nil {
		t.Fatalf("Reconcile resumed: %v", err)
	}
	if got := manager.ListByWorkspace(workspaceRoot); len(got) != 2 {
		t.Fatalf("resume should allow new dedupe key task, got %+v", got)
	}
}

type fakeAgentRunner struct{}

func (fakeAgentRunner) Run(_ context.Context, task contracts.Task, _ tasks.OutputSink) error {
	if task.Kind != "agent" {
		return fmt.Errorf("task kind = %q, want agent", task.Kind)
	}
	if task.RoleID == "" {
		return fmt.Errorf("role_id is required")
	}
	if task.SessionID != "" {
		return fmt.Errorf("session_id = %q, want empty", task.SessionID)
	}
	return nil
}

func writeWatcher(t *testing.T, path string, dedupeKey string) {
	t.Helper()
	body := "#!/bin/sh\nprintf '%s\\n' '{\"status\":\"needs_agent\",\"summary\":\"Docs are stale\",\"dedupe_key\":\"" + dedupeKey + "\",\"signal_kind\":\"docs\"}'\n"
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
}
