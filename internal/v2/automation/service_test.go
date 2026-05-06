package automation

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	stdruntime "runtime"
	"strings"
	"testing"
	"time"

	"go-agent/internal/v2/contracts"
	"go-agent/internal/v2/roles"
	"go-agent/internal/v2/store"
	"go-agent/internal/v2/tasks"
	"go-agent/internal/v2/workflows"
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
	skipWindowsWatcherExecution(t)

	ctx := context.Background()
	s, err := store.OpenInMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	workspaceRoot := t.TempDir()
	createAutomationSession(t, s, "main_session", workspaceRoot)
	watcherPath := roleAutomationWatcherPath(workspaceRoot, "doc_writer")
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
	if tasksAfterFirst[0].ReportSessionID != "main_session" || tasksAfterFirst[0].ParentSessionID != "main_session" {
		t.Fatalf("automation task should report to main session: %+v", tasksAfterFirst[0])
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

func TestServiceAutomationTriggersWorkflowRunAsync(t *testing.T) {
	skipWindowsWatcherExecution(t)

	ctx := context.Background()
	s, err := store.OpenInMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	workspaceRoot := t.TempDir()
	createAutomationSession(t, s, "main-session", workspaceRoot)
	watcherPath := roleAutomationWatcherPath(workspaceRoot, "doc_writer")
	writeWatcher(t, watcherPath, "docs-stale")
	createWorkflowRecord(t, s, contracts.Workflow{
		ID:            "workflow-docs",
		WorkspaceRoot: workspaceRoot,
		Name:          "Docs workflow",
		Trigger:       "manual",
		Steps:         `{"steps":[{"type":"role_task","role":"doc_writer","prompt":"Review docs."}]}`,
		CreatedAt:     time.Now().UTC(),
		UpdatedAt:     time.Now().UTC(),
	})

	waitStarted := make(chan string, 2)
	waitRelease := make(chan struct{})
	manager := &workflowTaskManagerStub{
		store: s,
		waitFn: func(ctx context.Context, task contracts.Task) (contracts.Task, error) {
			waitStarted <- task.ID
			<-waitRelease
			task.State = "completed"
			task.Outcome = "success"
			task.FinalText = "Workflow finished."
			task.UpdatedAt = time.Now().UTC()
			if err := s.Reports().Create(context.WithoutCancel(ctx), contracts.Report{
				ID:         "report-workflow-docs",
				SessionID:  task.ReportSessionID,
				SourceKind: "task_result",
				SourceID:   task.ID,
				Status:     task.State,
				Severity:   "info",
				Title:      "Task result: agent",
				Summary:    task.FinalText,
				CreatedAt:  time.Now().UTC(),
			}); err != nil {
				return contracts.Task{}, err
			}
			return task, nil
		},
	}
	workflowService := workflows.NewService(s, manager, "main-session")
	service := NewService(s, nil, nil)
	service.SetWorkflowTrigger(workflowService)
	automation := contracts.Automation{
		ID:            "automation-1",
		WorkspaceRoot: workspaceRoot,
		Title:         "Watch docs",
		Goal:          "Kick off the docs workflow",
		Owner:         "role:doc_writer",
		WorkflowID:    "workflow-docs",
		WatcherPath:   watcherPath,
		State:         "active",
	}
	if err := service.Create(ctx, automation); err != nil {
		t.Fatalf("Create: %v", err)
	}

	reconcileDone := make(chan error, 1)
	go func() {
		reconcileDone <- service.Reconcile(ctx)
	}()
	select {
	case err := <-reconcileDone:
		if err != nil {
			t.Fatalf("Reconcile: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Reconcile blocked on workflow completion")
	}

	var workflowTaskID string
	select {
	case workflowTaskID = <-waitStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("workflow wait was not started")
	}

	tasksAfter, err := s.Tasks().ListByWorkspace(ctx, workspaceRoot)
	if err != nil {
		t.Fatalf("ListByWorkspace tasks: %v", err)
	}
	if len(tasksAfter) != 1 {
		t.Fatalf("workflow-backed automation should create exactly one workflow task, got %+v", tasksAfter)
	}
	if tasksAfter[0].ID != workflowTaskID || tasksAfter[0].Prompt != "Review docs." {
		t.Fatalf("unexpected workflow task: %+v", tasksAfter[0])
	}

	run, err := s.Automations().GetRunByDedupeKey(ctx, automation.ID, "docs-stale")
	if err != nil {
		t.Fatalf("GetRunByDedupeKey: %v", err)
	}
	if run.TaskID != "" || run.WorkflowRunID == "" || run.Status != "workflow:queued" {
		t.Fatalf("unexpected run: %+v", run)
	}
	workflowRun, err := s.Workflows().GetRun(ctx, run.WorkflowRunID)
	if err != nil {
		t.Fatalf("GetRun workflow: %v", err)
	}
	if workflowRun.TriggerRef != "automation:automation-1:docs-stale" || workflowRun.State != "running" {
		t.Fatalf("unexpected workflow run: %+v", workflowRun)
	}

	if err := service.Reconcile(ctx); err != nil {
		t.Fatalf("Reconcile dedupe: %v", err)
	}
	if tasksAfter, err = s.Tasks().ListByWorkspace(ctx, workspaceRoot); err != nil {
		t.Fatalf("ListByWorkspace tasks after dedupe: %v", err)
	} else if len(tasksAfter) != 1 {
		t.Fatalf("workflow trigger should be deduped, got tasks %+v", tasksAfter)
	}
	runs, err := s.Automations().ListRunsByAutomation(ctx, automation.ID, 10)
	if err != nil {
		t.Fatalf("ListRunsByAutomation: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("dedupe should keep one automation run, got %+v", runs)
	}
	workflowRuns, err := s.Workflows().ListRunsByWorkspace(ctx, workspaceRoot, contracts.WorkflowRunListOptions{})
	if err != nil {
		t.Fatalf("ListRunsByWorkspace: %v", err)
	}
	if len(workflowRuns) != 1 {
		t.Fatalf("workflow trigger should only start one run, got %+v", workflowRuns)
	}

	close(waitRelease)
	workflowRun = waitForWorkflowRunState(t, s, run.WorkflowRunID, "completed")
	if workflowRun.State != "completed" {
		t.Fatalf("workflow run state = %q, want completed", workflowRun.State)
	}
}

func TestServiceAutomationWorkflowWorkspaceMismatchRecordsError(t *testing.T) {
	skipWindowsWatcherExecution(t)

	ctx := context.Background()
	s, err := store.OpenInMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	workspaceRoot := t.TempDir()
	otherWorkspaceRoot := t.TempDir()
	watcherPath := roleAutomationWatcherPath(workspaceRoot, "doc_writer")
	writeWatcher(t, watcherPath, "docs-stale")
	createWorkflowRecord(t, s, contracts.Workflow{
		ID:            "workflow-other",
		WorkspaceRoot: otherWorkspaceRoot,
		Name:          "Other workflow",
		Trigger:       "manual",
		Steps:         `{"steps":[{"type":"role_task","role":"doc_writer","prompt":"Review docs."}]}`,
		CreatedAt:     time.Now().UTC(),
		UpdatedAt:     time.Now().UTC(),
	})

	stub := &workflowTriggerStub{
		triggerFn: func(ctx context.Context, workflow contracts.Workflow, input workflows.TriggerInput) (contracts.WorkflowRun, error) {
			t.Fatalf("workflow trigger should not be called on workspace mismatch")
			return contracts.WorkflowRun{}, nil
		},
	}
	service := NewService(s, nil, nil)
	service.SetWorkflowTrigger(stub)
	automation := contracts.Automation{
		ID:            "automation-1",
		WorkspaceRoot: workspaceRoot,
		Title:         "Watch docs",
		Goal:          "Kick off the docs workflow",
		Owner:         "role:doc_writer",
		WorkflowID:    "workflow-other",
		WatcherPath:   watcherPath,
		State:         "active",
	}
	if err := service.Create(ctx, automation); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := service.Reconcile(ctx); err == nil {
		t.Fatalf("Reconcile expected error for workspace mismatch")
	}
	if stub.calls != 0 {
		t.Fatalf("workflow trigger calls = %d, want 0", stub.calls)
	}
	runs, err := s.Automations().ListRunsByAutomation(ctx, automation.ID, 10)
	if err != nil {
		t.Fatalf("ListRunsByAutomation: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("expected one error run, got %+v", runs)
	}
	if runs[0].Status != "error" || !strings.Contains(runs[0].Summary, "workspace mismatch") || !strings.HasPrefix(runs[0].DedupeKey, "docs-stale-") {
		t.Fatalf("unexpected mismatch run: %+v", runs[0])
	}
}

func TestServiceAutomationWorkflowRequiresTriggerService(t *testing.T) {
	skipWindowsWatcherExecution(t)

	ctx := context.Background()
	s, err := store.OpenInMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	workspaceRoot := t.TempDir()
	watcherPath := roleAutomationWatcherPath(workspaceRoot, "doc_writer")
	writeWatcher(t, watcherPath, "docs-stale")
	createWorkflowRecord(t, s, contracts.Workflow{
		ID:            "workflow-docs",
		WorkspaceRoot: workspaceRoot,
		Name:          "Docs workflow",
		Trigger:       "manual",
		Steps:         `{"steps":[{"type":"role_task","role":"doc_writer","prompt":"Review docs."}]}`,
		CreatedAt:     time.Now().UTC(),
		UpdatedAt:     time.Now().UTC(),
	})

	service := NewService(s, nil, nil)
	automation := contracts.Automation{
		ID:            "automation-1",
		WorkspaceRoot: workspaceRoot,
		Title:         "Watch docs",
		Goal:          "Kick off the docs workflow",
		Owner:         "role:doc_writer",
		WorkflowID:    "workflow-docs",
		WatcherPath:   watcherPath,
		State:         "active",
	}
	if err := service.Create(ctx, automation); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := service.Reconcile(ctx); err == nil {
		t.Fatalf("Reconcile expected error when workflow trigger service is missing")
	}
	runs, err := s.Automations().ListRunsByAutomation(ctx, automation.ID, 10)
	if err != nil {
		t.Fatalf("ListRunsByAutomation: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("expected one error run, got %+v", runs)
	}
	if runs[0].Status != "error" || !strings.Contains(runs[0].Summary, "workflow trigger service unavailable") {
		t.Fatalf("unexpected missing-trigger run: %+v", runs[0])
	}
}

func TestServiceCreateRejectsWatcherPathOutsideWorkspace(t *testing.T) {
	ctx := context.Background()
	s, err := store.OpenInMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	workspaceRoot := t.TempDir()
	otherRoot := t.TempDir()
	service := NewService(s, nil, nil)

	tests := []struct {
		name        string
		watcherPath string
	}{
		{name: "absolute outside workspace", watcherPath: filepath.Join(otherRoot, "watcher.sh")},
		{name: "relative parent escape", watcherPath: filepath.Join("..", filepath.Base(otherRoot), "watcher.sh")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := service.Create(ctx, contracts.Automation{
				ID:            "automation-" + strings.ReplaceAll(tt.name, " ", "-"),
				WorkspaceRoot: workspaceRoot,
				Title:         "Watch docs",
				Goal:          "Fix stale docs",
				Owner:         "role:doc_writer",
				WatcherPath:   tt.watcherPath,
				State:         "active",
			})
			if err == nil || !strings.Contains(err.Error(), "escapes workspace root") {
				t.Fatalf("expected workspace escape error, got %v", err)
			}
		})
	}
}

func TestServiceCreateRejectsNonRoleOwner(t *testing.T) {
	ctx := context.Background()
	s, err := store.OpenInMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	workspaceRoot := t.TempDir()
	service := NewService(s, nil, nil)
	err = service.Create(ctx, contracts.Automation{
		ID:            "automation-main-owner",
		WorkspaceRoot: workspaceRoot,
		Title:         "Watch docs",
		Goal:          "Fix stale docs",
		Owner:         "main",
		WatcherPath:   "roles/doc_writer/automations/watch.sh",
		State:         "active",
	})
	if err == nil || !strings.Contains(err.Error(), "automation owner must be a role") {
		t.Fatalf("expected role owner error, got %v", err)
	}
}

func TestServiceCreateRejectsRoleOwnedWatcherSymlinkEscape(t *testing.T) {
	ctx := context.Background()
	s, err := store.OpenInMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	workspaceRoot := t.TempDir()
	ownedDir := filepath.Join(workspaceRoot, "roles", "doc_writer", "automations")
	otherDir := filepath.Join(workspaceRoot, "roles", "other_role", "automations")
	if err := os.MkdirAll(ownedDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(otherDir, 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(otherDir, "watch.sh")
	if err := os.WriteFile(target, []byte("#!/bin/sh\nprintf '{}'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(ownedDir, "watch.sh")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}

	service := NewService(s, nil, nil)
	err = service.Create(ctx, contracts.Automation{
		ID:            "automation-symlink",
		WorkspaceRoot: workspaceRoot,
		Title:         "Watch docs",
		Goal:          "Fix stale docs",
		Owner:         "role:doc_writer",
		WatcherPath:   filepath.Join("roles", "doc_writer", "automations", "watch.sh"),
		State:         "active",
	})
	if err == nil || !strings.Contains(err.Error(), "roles/doc_writer/automations") {
		t.Fatalf("expected role-owned realpath error, got %v", err)
	}
}

func TestServiceAutomationReconcileRunRecordFailureDoesNotRetriggerWorkflow(t *testing.T) {
	skipWindowsWatcherExecution(t)

	ctx := context.Background()
	baseStore, err := store.OpenInMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer baseStore.Close()

	s := newAutomationRunFailureStore(baseStore, 1)
	workspaceRoot := t.TempDir()
	createAutomationSession(t, s, "main-session", workspaceRoot)
	watcherPath := roleAutomationWatcherPath(workspaceRoot, "doc_writer")
	writeWatcher(t, watcherPath, "docs-stale")
	createWorkflowRecord(t, s, contracts.Workflow{
		ID:            "workflow-docs",
		WorkspaceRoot: workspaceRoot,
		Name:          "Docs workflow",
		Trigger:       "manual",
		Steps:         `{"steps":[{"type":"role_task","role":"doc_writer","prompt":"Review docs."}]}`,
		CreatedAt:     time.Now().UTC(),
		UpdatedAt:     time.Now().UTC(),
	})

	waitStarted := make(chan string, 2)
	waitRelease := make(chan struct{})
	manager := &workflowTaskManagerStub{
		store: baseStore,
		waitFn: func(ctx context.Context, task contracts.Task) (contracts.Task, error) {
			waitStarted <- task.ID
			<-waitRelease
			task.State = "completed"
			task.Outcome = "success"
			task.FinalText = "Workflow finished."
			task.UpdatedAt = time.Now().UTC()
			return task, nil
		},
	}
	workflowService := workflows.NewService(s, manager, "main-session")
	service := NewService(s, nil, nil)
	service.SetWorkflowTrigger(workflowService)
	automation := contracts.Automation{
		ID:            "automation-1",
		WorkspaceRoot: workspaceRoot,
		Title:         "Watch docs",
		Goal:          "Kick off the docs workflow",
		Owner:         "role:doc_writer",
		WorkflowID:    "workflow-docs",
		WatcherPath:   watcherPath,
		State:         "active",
	}
	if err := service.Create(ctx, automation); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := service.Reconcile(ctx); err == nil {
		t.Fatalf("Reconcile expected first run record failure")
	}
	var firstTaskID string
	select {
	case firstTaskID = <-waitStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("workflow wait was not started")
	}

	if err := service.Reconcile(ctx); err != nil {
		t.Fatalf("Reconcile retry: %v", err)
	}
	select {
	case taskID := <-waitStarted:
		t.Fatalf("duplicate workflow task started: %s (first %s)", taskID, firstTaskID)
	case <-time.After(150 * time.Millisecond):
	}

	tasksAfter, err := baseStore.Tasks().ListByWorkspace(ctx, workspaceRoot)
	if err != nil {
		t.Fatalf("ListByWorkspace tasks: %v", err)
	}
	if len(tasksAfter) != 1 {
		t.Fatalf("workflow-backed automation should create one task, got %+v", tasksAfter)
	}

	workflowRuns, err := baseStore.Workflows().ListRunsByWorkspace(ctx, workspaceRoot, contracts.WorkflowRunListOptions{})
	if err != nil {
		t.Fatalf("ListRunsByWorkspace: %v", err)
	}
	if len(workflowRuns) != 1 {
		t.Fatalf("workflow trigger should reuse existing run, got %+v", workflowRuns)
	}

	runs, err := baseStore.Automations().ListRunsByAutomation(ctx, automation.ID, 10)
	if err != nil {
		t.Fatalf("ListRunsByAutomation: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("expected one persisted automation run after retry, got %+v", runs)
	}
	if runs[0].WorkflowRunID != workflowRuns[0].ID {
		t.Fatalf("automation run workflow_run_id = %q, want %q", runs[0].WorkflowRunID, workflowRuns[0].ID)
	}

	close(waitRelease)
	workflowRun := waitForWorkflowRunState(t, baseStore, workflowRuns[0].ID, "completed")
	if workflowRun.State != "completed" {
		t.Fatalf("workflow run state = %q, want completed", workflowRun.State)
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
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	body := "#!/bin/sh\nprintf '%s\\n' '{\"status\":\"needs_agent\",\"summary\":\"Docs are stale\",\"dedupe_key\":\"" + dedupeKey + "\",\"signal_kind\":\"docs\"}'\n"
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
}

func roleAutomationWatcherPath(workspaceRoot, roleID string) string {
	return filepath.Join(workspaceRoot, "roles", roleID, "automations", "watcher.sh")
}

func skipWindowsWatcherExecution(t *testing.T) {
	t.Helper()
	if stdruntime.GOOS == "windows" {
		t.Skip("unix watcher script execution is not supported on Windows")
	}
}

func createAutomationSession(t *testing.T, s contracts.Store, id, workspaceRoot string) {
	t.Helper()
	now := time.Now().UTC()
	if err := s.Sessions().Create(context.Background(), contracts.Session{
		ID:                id,
		WorkspaceRoot:     workspaceRoot,
		SystemPrompt:      "system",
		PermissionProfile: "default",
		State:             "idle",
		CreatedAt:         now,
		UpdatedAt:         now,
	}); err != nil {
		t.Fatal(err)
	}
}

type workflowTriggerStub struct {
	calls     int
	triggerFn func(ctx context.Context, workflow contracts.Workflow, input workflows.TriggerInput) (contracts.WorkflowRun, error)
}

func (s *workflowTriggerStub) TriggerAsync(ctx context.Context, workflow contracts.Workflow, input workflows.TriggerInput) (contracts.WorkflowRun, error) {
	s.calls++
	if s.triggerFn == nil {
		return contracts.WorkflowRun{}, nil
	}
	return s.triggerFn(ctx, workflow, input)
}

type workflowTaskManagerStub struct {
	store  contracts.Store
	tasks  map[string]contracts.Task
	waitFn func(ctx context.Context, task contracts.Task) (contracts.Task, error)
}

func (m *workflowTaskManagerStub) Create(ctx context.Context, task contracts.Task) error {
	if m.tasks == nil {
		m.tasks = map[string]contracts.Task{}
	}
	m.tasks[task.ID] = task
	return m.store.Tasks().Create(ctx, task)
}

func (m *workflowTaskManagerStub) Start(ctx context.Context, taskID string) error {
	task := m.tasks[taskID]
	task.State = "running"
	task.UpdatedAt = time.Now().UTC()
	m.tasks[taskID] = task
	return m.store.Tasks().Update(ctx, task)
}

func (m *workflowTaskManagerStub) Wait(ctx context.Context, taskID string) (contracts.Task, error) {
	task := m.tasks[taskID]
	if m.waitFn != nil {
		completedTask, err := m.waitFn(ctx, task)
		if err != nil {
			return contracts.Task{}, err
		}
		m.tasks[taskID] = completedTask
		if err := m.store.Tasks().Update(context.WithoutCancel(ctx), completedTask); err != nil {
			return contracts.Task{}, err
		}
		return completedTask, nil
	}
	task.State = "completed"
	task.UpdatedAt = time.Now().UTC()
	m.tasks[taskID] = task
	if err := m.store.Tasks().Update(context.WithoutCancel(ctx), task); err != nil {
		return contracts.Task{}, err
	}
	return task, nil
}

func waitForWorkflowRunState(t *testing.T, s contracts.Store, runID, wantState string) contracts.WorkflowRun {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		run, err := s.Workflows().GetRun(context.Background(), runID)
		if err == nil && run.State == wantState {
			return run
		}
		time.Sleep(10 * time.Millisecond)
	}
	run, err := s.Workflows().GetRun(context.Background(), runID)
	if err != nil {
		t.Fatalf("GetRun after wait: %v", err)
	}
	t.Fatalf("workflow run state = %q, want %q", run.State, wantState)
	return contracts.WorkflowRun{}
}

func createWorkflowRecord(t *testing.T, s contracts.Store, workflow contracts.Workflow) {
	t.Helper()
	if err := s.Workflows().Create(context.Background(), workflow); err != nil {
		t.Fatalf("create workflow: %v", err)
	}
}

type automationRunFailureStore struct {
	contracts.Store
	automations contracts.AutomationRepository
}

func newAutomationRunFailureStore(base contracts.Store, failures int) *automationRunFailureStore {
	return &automationRunFailureStore{
		Store: base,
		automations: &automationRunFailureRepo{
			AutomationRepository: base.Automations(),
			remainingFailures:    failures,
		},
	}
}

func (s *automationRunFailureStore) Automations() contracts.AutomationRepository {
	return s.automations
}

type automationRunFailureRepo struct {
	contracts.AutomationRepository
	remainingFailures int
}

func (r *automationRunFailureRepo) CreateRun(ctx context.Context, run contracts.AutomationRun) error {
	if r.remainingFailures > 0 {
		r.remainingFailures--
		return fmt.Errorf("forced automation run create failure for %s", run.DedupeKey)
	}
	return r.AutomationRepository.CreateRun(ctx, run)
}
