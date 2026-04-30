package task

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"go-agent/internal/types"
)

type mockWorkspaceStore struct {
	tasks map[string][]Task
	todos map[string][]TodoItem
}

func newMockWorkspaceStore() *mockWorkspaceStore {
	return &mockWorkspaceStore{
		tasks: make(map[string][]Task),
		todos: make(map[string][]TodoItem),
	}
}

func (m *mockWorkspaceStore) ListWorkspaceTasks(_ context.Context, workspaceRoot string) ([]Task, error) {
	tasks := m.tasks[workspaceRoot]
	out := make([]Task, len(tasks))
	for i, item := range tasks {
		out[i] = copyTask(item)
	}
	return out, nil
}

func (m *mockWorkspaceStore) UpsertWorkspaceTask(_ context.Context, task Task) error {
	tasks := m.tasks[task.WorkspaceRoot]
	for i := range tasks {
		if tasks[i].ID == task.ID {
			tasks[i] = copyTask(task)
			m.tasks[task.WorkspaceRoot] = tasks
			return nil
		}
	}
	m.tasks[task.WorkspaceRoot] = append(tasks, copyTask(task))
	return nil
}

func (m *mockWorkspaceStore) GetWorkspaceTodos(_ context.Context, workspaceRoot string) ([]TodoItem, error) {
	todos := m.todos[workspaceRoot]
	if len(todos) == 0 {
		return nil, nil
	}
	out := make([]TodoItem, len(todos))
	copy(out, todos)
	return out, nil
}

func (m *mockWorkspaceStore) ReplaceWorkspaceTodos(_ context.Context, workspaceRoot string, todos []TodoItem) error {
	if len(todos) == 0 {
		m.todos[workspaceRoot] = nil
		return nil
	}
	out := make([]TodoItem, len(todos))
	copy(out, todos)
	m.todos[workspaceRoot] = out
	return nil
}

type mockTerminalNotifier struct {
	mu       sync.Mutex
	notified []Task
}

func (m *mockTerminalNotifier) NotifyTaskTerminal(_ context.Context, task Task) error {
	m.mu.Lock()
	m.notified = append(m.notified, copyTask(task))
	m.mu.Unlock()
	return nil
}

func (m *mockTerminalNotifier) notifiedCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.notified)
}

type mockAgentExecutor struct {
	runFn func(ctx context.Context, taskID, workspaceRoot, prompt string, activatedSkillNames []string, targetRole string, observer AgentTaskObserver) error
}

func (m mockAgentExecutor) RunTask(ctx context.Context, taskID, workspaceRoot, prompt string, activatedSkillNames []string, targetRole string, observer AgentTaskObserver) error {
	if m.runFn != nil {
		return m.runFn(ctx, taskID, workspaceRoot, prompt, activatedSkillNames, targetRole, observer)
	}
	return nil
}

type mockRunner struct {
	runFn func(ctx context.Context, task *Task, sink OutputSink) error
}

func (m mockRunner) Run(ctx context.Context, task *Task, sink OutputSink) error {
	if m.runFn != nil {
		return m.runFn(ctx, task, sink)
	}
	return nil
}

func newTestManager(cfg Config, runners map[TaskType]Runner, agentExecutor AgentExecutor) (*Manager, *mockWorkspaceStore, *mockTerminalNotifier) {
	workspaceStore := newMockWorkspaceStore()
	notifier := &mockTerminalNotifier{}
	cfg.WorkspaceStore = workspaceStore
	cfg.TerminalNotifier = notifier
	return NewManager(cfg, runners, agentExecutor), workspaceStore, notifier
}

func TestValidateStatusTransition(t *testing.T) {
	statuses := []TaskStatus{
		TaskStatusPending,
		TaskStatusRunning,
		TaskStatusCompleted,
		TaskStatusFailed,
		TaskStatusStopped,
	}

	for _, from := range statuses {
		for _, to := range statuses {
			from := from
			to := to
			t.Run(string(from)+"->"+string(to), func(t *testing.T) {
				err := validateStatusTransition(from, to)
				wantValid := from == to ||
					(from == TaskStatusPending && (to == TaskStatusRunning || to == TaskStatusStopped)) ||
					(from == TaskStatusRunning && (to == TaskStatusCompleted || to == TaskStatusFailed || to == TaskStatusStopped))
				if wantValid && err != nil {
					t.Errorf("validateStatusTransition(%q, %q) returned error: %v", from, to, err)
				}
				if !wantValid && err == nil {
					t.Errorf("validateStatusTransition(%q, %q) returned nil error, want invalid transition", from, to)
				}
			})
		}
	}
}

func TestIsTerminalStatus(t *testing.T) {
	cases := []struct {
		status TaskStatus
		want   bool
	}{
		{status: TaskStatusPending, want: false},
		{status: TaskStatusRunning, want: false},
		{status: TaskStatusCompleted, want: true},
		{status: TaskStatusFailed, want: true},
		{status: TaskStatusStopped, want: true},
	}

	for _, tc := range cases {
		t.Run(string(tc.status), func(t *testing.T) {
			got := isTerminalStatus(tc.status)
			if got != tc.want {
				t.Errorf("isTerminalStatus(%q) = %t, want %t", tc.status, got, tc.want)
			}
		})
	}
}

func TestCopyTask(t *testing.T) {
	t.Run("copies all fields correctly", func(t *testing.T) {
		endTime := time.Date(2026, 4, 29, 12, 0, 0, 0, time.UTC)
		readyAt := endTime.Add(time.Minute)
		notifiedAt := readyAt.Add(time.Minute)
		original := Task{
			ID:                   "task-1",
			Type:                 TaskTypeAgent,
			Status:               TaskStatusCompleted,
			Command:              "run",
			Description:          "desc",
			Owner:                "owner",
			Kind:                 "kind",
			ScheduledJobID:       "job-1",
			ActivatedSkillNames:  []string{"plan", "report"},
			WorkspaceRoot:        filepath.ToSlash("/tmp/workspace"),
			Output:               "output",
			Error:                "err",
			TimeoutSeconds:       30,
			StartTime:            endTime.Add(-time.Hour),
			EndTime:              &endTime,
			Outcome:              types.ChildAgentOutcomeSuccess,
			OutcomeSummary:       "summary",
			FinalResultKind:      FinalResultKindAssistantText,
			FinalResultText:      "done",
			FinalResultReadyAt:   &readyAt,
			CompletionNotifiedAt: &notifiedAt,
		}

		copied := copyTask(original)
		if copied.ID != original.ID {
			t.Errorf("ID = %q, want %q", copied.ID, original.ID)
		}
		if copied.Type != original.Type {
			t.Errorf("Type = %q, want %q", copied.Type, original.Type)
		}
		if copied.Status != original.Status {
			t.Errorf("Status = %q, want %q", copied.Status, original.Status)
		}
		if copied.Command != original.Command {
			t.Errorf("Command = %q, want %q", copied.Command, original.Command)
		}
		if copied.Description != original.Description {
			t.Errorf("Description = %q, want %q", copied.Description, original.Description)
		}
		if copied.Owner != original.Owner {
			t.Errorf("Owner = %q, want %q", copied.Owner, original.Owner)
		}
		if copied.Kind != original.Kind {
			t.Errorf("Kind = %q, want %q", copied.Kind, original.Kind)
		}
		if copied.ScheduledJobID != original.ScheduledJobID {
			t.Errorf("ScheduledJobID = %q, want %q", copied.ScheduledJobID, original.ScheduledJobID)
		}
		if !slices.Equal(copied.ActivatedSkillNames, original.ActivatedSkillNames) {
			t.Errorf("ActivatedSkillNames = %v, want %v", copied.ActivatedSkillNames, original.ActivatedSkillNames)
		}
		if copied.WorkspaceRoot != original.WorkspaceRoot {
			t.Errorf("WorkspaceRoot = %q, want %q", copied.WorkspaceRoot, original.WorkspaceRoot)
		}
		if copied.Output != original.Output {
			t.Errorf("Output = %q, want %q", copied.Output, original.Output)
		}
		if copied.Error != original.Error {
			t.Errorf("Error = %q, want %q", copied.Error, original.Error)
		}
		if copied.TimeoutSeconds != original.TimeoutSeconds {
			t.Errorf("TimeoutSeconds = %d, want %d", copied.TimeoutSeconds, original.TimeoutSeconds)
		}
		if !copied.StartTime.Equal(original.StartTime) {
			t.Errorf("StartTime = %v, want %v", copied.StartTime, original.StartTime)
		}
		if copied.EndTime == original.EndTime {
			t.Errorf("EndTime pointer = same pointer, want copy")
		}
		if copied.EndTime == nil || !copied.EndTime.Equal(*original.EndTime) {
			t.Errorf("EndTime = %v, want %v", copied.EndTime, original.EndTime)
		}
		if copied.FinalResultReadyAt == original.FinalResultReadyAt {
			t.Errorf("FinalResultReadyAt pointer = same pointer, want copy")
		}
		if copied.FinalResultReadyAt == nil || !copied.FinalResultReadyAt.Equal(*original.FinalResultReadyAt) {
			t.Errorf("FinalResultReadyAt = %v, want %v", copied.FinalResultReadyAt, original.FinalResultReadyAt)
		}
		if copied.CompletionNotifiedAt == original.CompletionNotifiedAt {
			t.Errorf("CompletionNotifiedAt pointer = same pointer, want copy")
		}
		if copied.CompletionNotifiedAt == nil || !copied.CompletionNotifiedAt.Equal(*original.CompletionNotifiedAt) {
			t.Errorf("CompletionNotifiedAt = %v, want %v", copied.CompletionNotifiedAt, original.CompletionNotifiedAt)
		}

		original.ActivatedSkillNames[0] = "changed"
		if copied.ActivatedSkillNames[0] != "plan" {
			t.Errorf("ActivatedSkillNames copy changed to %q, want %q", copied.ActivatedSkillNames[0], "plan")
		}
	})

	t.Run("handles nil times", func(t *testing.T) {
		original := Task{
			ID:            "task-1",
			WorkspaceRoot: filepath.ToSlash("/tmp/workspace"),
		}
		copied := copyTask(original)
		if copied.EndTime != nil {
			t.Errorf("EndTime = %v, want nil", copied.EndTime)
		}
		if copied.FinalResultReadyAt != nil {
			t.Errorf("FinalResultReadyAt = %v, want nil", copied.FinalResultReadyAt)
		}
	})
}

func TestManagerCreate(t *testing.T) {
	t.Run("create without start", func(t *testing.T) {
		manager, workspaceStore, _ := newTestManager(Config{}, nil, nil)
		workspaceRoot := filepath.ToSlash(t.TempDir())

		created, err := manager.Create(context.Background(), CreateTaskInput{
			ID:            "task-1",
			Type:          TaskTypeAgent,
			Command:       "run",
			WorkspaceRoot: workspaceRoot,
		})
		if err != nil {
			t.Errorf("Create returned error: %v", err)
			return
		}
		if created.Status != TaskStatusPending {
			t.Errorf("Status = %q, want %q", created.Status, TaskStatusPending)
		}
		if len(workspaceStore.tasks[workspaceRoot]) != 1 {
			t.Errorf("stored tasks = %d, want 1", len(workspaceStore.tasks[workspaceRoot]))
		}
	})

	t.Run("create with start true runner available", func(t *testing.T) {
		runStarted := make(chan string, 1)
		release := make(chan struct{})
		runner := mockRunner{
			runFn: func(ctx context.Context, task *Task, sink OutputSink) error {
				runStarted <- task.ID
				<-release
				return nil
			},
		}
		manager, _, notifier := newTestManager(Config{}, map[TaskType]Runner{
			TaskTypeAgent: runner,
		}, nil)
		workspaceRoot := filepath.ToSlash(t.TempDir())

		created, err := manager.Create(context.Background(), CreateTaskInput{
			ID:            "task-1",
			Type:          TaskTypeAgent,
			Command:       "run",
			WorkspaceRoot: workspaceRoot,
			Start:         true,
		})
		if err != nil {
			t.Errorf("Create returned error: %v", err)
			return
		}
		select {
		case taskID := <-runStarted:
			if taskID != created.ID {
				t.Errorf("runner task ID = %q, want %q", taskID, created.ID)
			}
		case <-time.After(time.Second):
			t.Errorf("runner did not start")
			return
		}

		got, ok, err := manager.Get(created.ID, workspaceRoot)
		if err != nil {
			t.Errorf("Get returned error: %v", err)
		} else if !ok {
			t.Errorf("Get returned ok=false, want true")
		} else if got.Status != TaskStatusRunning {
			t.Errorf("Status = %q, want %q", got.Status, TaskStatusRunning)
		}

		close(release)

		waitCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		completed, timeout, err := manager.Wait(waitCtx, created.ID, workspaceRoot)
		if err != nil {
			t.Errorf("Wait returned error: %v", err)
			return
		}
		if timeout {
			t.Errorf("timeout = true, want false")
		}
		if completed.Status != TaskStatusCompleted {
			t.Errorf("Status = %q, want %q", completed.Status, TaskStatusCompleted)
		}
		if n := notifier.notifiedCount(); n != 1 {
			t.Errorf("notified = %d, want 1", n)
		}
	})

	t.Run("create with start true runner returns error", func(t *testing.T) {
		runner := mockRunner{
			runFn: func(ctx context.Context, task *Task, sink OutputSink) error {
				return errors.New("runner failed")
			},
		}
		manager, _, _ := newTestManager(Config{}, map[TaskType]Runner{
			TaskTypeAgent: runner,
		}, nil)
		workspaceRoot := filepath.ToSlash(t.TempDir())

		created, err := manager.Create(context.Background(), CreateTaskInput{
			ID:            "task-1",
			Type:          TaskTypeAgent,
			Command:       "run",
			WorkspaceRoot: workspaceRoot,
			Start:         true,
		})
		if err != nil {
			t.Errorf("Create returned error: %v", err)
			return
		}

		waitCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		completed, timeout, err := manager.Wait(waitCtx, created.ID, workspaceRoot)
		if err != nil {
			t.Errorf("Wait returned error: %v", err)
			return
		}
		if timeout {
			t.Errorf("timeout = true, want false")
		}
		if completed.Status != TaskStatusFailed {
			t.Errorf("Status = %q, want %q", completed.Status, TaskStatusFailed)
		}
		if completed.Error != "runner failed" {
			t.Errorf("Error = %q, want %q", completed.Error, "runner failed")
		}
	})

	t.Run("create with unsupported type", func(t *testing.T) {
		manager, _, _ := newTestManager(Config{}, nil, nil)
		workspaceRoot := filepath.ToSlash(t.TempDir())

		_, err := manager.Create(context.Background(), CreateTaskInput{
			ID:            "task-1",
			Type:          TaskType("custom"),
			Command:       "run",
			WorkspaceRoot: workspaceRoot,
			Start:         true,
		})
		if err == nil {
			t.Errorf("error = nil, want unsupported type error")
		}
		_, ok, getErr := manager.Get("task-1", workspaceRoot)
		if getErr != nil {
			t.Errorf("Get returned error: %v", getErr)
		}
		if ok {
			t.Errorf("ok = true, want false")
		}
	})

	t.Run("create with same id overwrites first", func(t *testing.T) {
		manager, workspaceStore, _ := newTestManager(Config{}, nil, nil)
		workspaceRoot := filepath.ToSlash(t.TempDir())

		_, err := manager.Create(context.Background(), CreateTaskInput{
			ID:            "task-1",
			Type:          TaskTypeAgent,
			Command:       "first",
			Description:   "first description",
			WorkspaceRoot: workspaceRoot,
		})
		if err != nil {
			t.Fatalf("first Create returned error: %v", err)
		}
		second, err := manager.Create(context.Background(), CreateTaskInput{
			ID:            "task-1",
			Type:          TaskTypeAgent,
			Command:       "second",
			Description:   "second description",
			WorkspaceRoot: workspaceRoot,
		})
		if err != nil {
			t.Errorf("second Create returned error: %v", err)
			return
		}

		got, ok, err := manager.Get(second.ID, workspaceRoot)
		if err != nil {
			t.Errorf("Get returned error: %v", err)
		} else if !ok {
			t.Errorf("Get returned ok=false, want true")
		} else {
			if got.Command != "second" {
				t.Errorf("Command = %q, want %q", got.Command, "second")
			}
			if got.Description != "second description" {
				t.Errorf("Description = %q, want %q", got.Description, "second description")
			}
		}
		if len(workspaceStore.tasks[workspaceRoot]) != 1 {
			t.Errorf("stored tasks = %d, want 1", len(workspaceStore.tasks[workspaceRoot]))
		}
	})

	t.Run("auto generated id", func(t *testing.T) {
		manager, _, _ := newTestManager(Config{}, nil, nil)
		workspaceRoot := filepath.ToSlash(t.TempDir())

		created, err := manager.Create(context.Background(), CreateTaskInput{
			Type:          TaskTypeAgent,
			Command:       "run",
			WorkspaceRoot: workspaceRoot,
		})
		if err != nil {
			t.Errorf("Create returned error: %v", err)
			return
		}
		if !strings.HasPrefix(created.ID, "task_") {
			t.Errorf("ID = %q, want task_ prefix", created.ID)
		}
	})

	t.Run("normalizes workspace root", func(t *testing.T) {
		manager, _, _ := newTestManager(Config{}, nil, nil)
		workspaceRoot := filepath.ToSlash(t.TempDir())

		created, err := manager.Create(context.Background(), CreateTaskInput{
			ID:            "task-1",
			Type:          TaskTypeAgent,
			Command:       "run",
			WorkspaceRoot: workspaceRoot + "/",
		})
		if err != nil {
			t.Errorf("Create returned error: %v", err)
			return
		}
		if created.WorkspaceRoot != workspaceRoot {
			t.Errorf("WorkspaceRoot = %q, want %q", created.WorkspaceRoot, workspaceRoot)
		}
	})
}

func TestManagerUpdate(t *testing.T) {
	t.Run("valid transition pending to running", func(t *testing.T) {
		manager, _, _ := newTestManager(Config{}, nil, nil)
		workspaceRoot := filepath.ToSlash(t.TempDir())
		created, err := manager.Create(context.Background(), CreateTaskInput{
			ID:            "task-1",
			Type:          TaskTypeAgent,
			Command:       "run",
			WorkspaceRoot: workspaceRoot,
		})
		if err != nil {
			t.Fatalf("Create returned error: %v", err)
		}

		err = manager.Update(created.ID, workspaceRoot, UpdateTaskInput{Status: TaskStatusRunning})
		if err != nil {
			t.Errorf("Update returned error: %v", err)
			return
		}

		got, ok, err := manager.Get(created.ID, workspaceRoot)
		if err != nil {
			t.Errorf("Get returned error: %v", err)
		} else if !ok {
			t.Errorf("Get returned ok=false, want true")
		} else if got.Status != TaskStatusRunning {
			t.Errorf("Status = %q, want %q", got.Status, TaskStatusRunning)
		}
	})

	t.Run("valid transition running to completed", func(t *testing.T) {
		manager, _, _ := newTestManager(Config{}, nil, nil)
		workspaceRoot := filepath.ToSlash(t.TempDir())
		created, err := manager.Create(context.Background(), CreateTaskInput{
			ID:            "task-1",
			Type:          TaskTypeAgent,
			Command:       "run",
			WorkspaceRoot: workspaceRoot,
		})
		if err != nil {
			t.Fatalf("Create returned error: %v", err)
		}
		if err := manager.Update(created.ID, workspaceRoot, UpdateTaskInput{Status: TaskStatusRunning}); err != nil {
			t.Fatalf("Update to running returned error: %v", err)
		}

		err = manager.Update(created.ID, workspaceRoot, UpdateTaskInput{Status: TaskStatusCompleted})
		if err != nil {
			t.Errorf("Update returned error: %v", err)
			return
		}

		got, ok, err := manager.Get(created.ID, workspaceRoot)
		if err != nil {
			t.Errorf("Get returned error: %v", err)
		} else if !ok {
			t.Errorf("Get returned ok=false, want true")
		} else {
			if got.Status != TaskStatusCompleted {
				t.Errorf("Status = %q, want %q", got.Status, TaskStatusCompleted)
			}
			if got.EndTime == nil {
				t.Errorf("EndTime = nil, want non-nil")
			}
		}
	})

	t.Run("invalid transition", func(t *testing.T) {
		manager, _, _ := newTestManager(Config{}, nil, nil)
		workspaceRoot := filepath.ToSlash(t.TempDir())
		created, err := manager.Create(context.Background(), CreateTaskInput{
			ID:            "task-1",
			Type:          TaskTypeAgent,
			Command:       "run",
			WorkspaceRoot: workspaceRoot,
		})
		if err != nil {
			t.Fatalf("Create returned error: %v", err)
		}

		err = manager.Update(created.ID, workspaceRoot, UpdateTaskInput{Status: TaskStatusCompleted})
		if err == nil {
			t.Errorf("error = nil, want invalid transition error")
			return
		}

		got, ok, getErr := manager.Get(created.ID, workspaceRoot)
		if getErr != nil {
			t.Errorf("Get returned error: %v", getErr)
		} else if !ok {
			t.Errorf("Get returned ok=false, want true")
		} else if got.Status != TaskStatusPending {
			t.Errorf("Status = %q, want %q", got.Status, TaskStatusPending)
		}
	})

	t.Run("update updates description and owner", func(t *testing.T) {
		manager, _, _ := newTestManager(Config{}, nil, nil)
		workspaceRoot := filepath.ToSlash(t.TempDir())
		created, err := manager.Create(context.Background(), CreateTaskInput{
			ID:            "task-1",
			Type:          TaskTypeAgent,
			Command:       "run",
			WorkspaceRoot: workspaceRoot,
		})
		if err != nil {
			t.Fatalf("Create returned error: %v", err)
		}

		err = manager.Update(created.ID, workspaceRoot, UpdateTaskInput{
			Status:      TaskStatusPending,
			Description: "updated description",
			Owner:       "updated owner",
		})
		if err != nil {
			t.Errorf("Update returned error: %v", err)
			return
		}

		got, ok, getErr := manager.Get(created.ID, workspaceRoot)
		if getErr != nil {
			t.Errorf("Get returned error: %v", getErr)
		} else if !ok {
			t.Errorf("Get returned ok=false, want true")
		} else {
			if got.Description != "updated description" {
				t.Errorf("Description = %q, want %q", got.Description, "updated description")
			}
			if got.Owner != "updated owner" {
				t.Errorf("Owner = %q, want %q", got.Owner, "updated owner")
			}
		}
	})

	t.Run("task not found", func(t *testing.T) {
		manager, _, _ := newTestManager(Config{}, nil, nil)
		err := manager.Update("missing", filepath.ToSlash(t.TempDir()), UpdateTaskInput{Status: TaskStatusRunning})
		if err == nil {
			t.Errorf("error = nil, want task not found error")
		}
	})
}

func TestManagerStop(t *testing.T) {
	t.Run("stop pending task", func(t *testing.T) {
		manager, _, _ := newTestManager(Config{}, nil, nil)
		workspaceRoot := filepath.ToSlash(t.TempDir())
		created, err := manager.Create(context.Background(), CreateTaskInput{
			ID:            "task-1",
			Type:          TaskTypeAgent,
			Command:       "run",
			WorkspaceRoot: workspaceRoot,
		})
		if err != nil {
			t.Fatalf("Create returned error: %v", err)
		}

		err = manager.Stop(created.ID, workspaceRoot)
		if err != nil {
			t.Errorf("Stop returned error: %v", err)
			return
		}

		got, ok, getErr := manager.Get(created.ID, workspaceRoot)
		if getErr != nil {
			t.Errorf("Get returned error: %v", getErr)
		} else if !ok {
			t.Errorf("Get returned ok=false, want true")
		} else {
			if got.Status != TaskStatusStopped {
				t.Errorf("Status = %q, want %q", got.Status, TaskStatusStopped)
			}
			if got.EndTime == nil {
				t.Errorf("EndTime = nil, want non-nil")
			}
		}
	})

	t.Run("stop already terminal task", func(t *testing.T) {
		manager, _, _ := newTestManager(Config{}, nil, nil)
		workspaceRoot := filepath.ToSlash(t.TempDir())
		created, err := manager.Create(context.Background(), CreateTaskInput{
			ID:            "task-1",
			Type:          TaskTypeAgent,
			Command:       "run",
			WorkspaceRoot: workspaceRoot,
		})
		if err != nil {
			t.Fatalf("Create returned error: %v", err)
		}
		if err := manager.Stop(created.ID, workspaceRoot); err != nil {
			t.Fatalf("initial Stop returned error: %v", err)
		}

		err = manager.Stop(created.ID, workspaceRoot)
		if err != nil {
			t.Errorf("Stop returned error: %v", err)
		}
	})

	t.Run("stop non existent task", func(t *testing.T) {
		manager, _, _ := newTestManager(Config{}, nil, nil)
		err := manager.Stop("missing", filepath.ToSlash(t.TempDir()))
		if err == nil {
			t.Errorf("error = nil, want task not found error")
		}
	})

	t.Run("stop running task", func(t *testing.T) {
		runnerDone := make(chan error, 1)
		runStarted := make(chan struct{}, 1)
		runner := mockRunner{
			runFn: func(ctx context.Context, task *Task, sink OutputSink) error {
				runStarted <- struct{}{}
				<-ctx.Done()
				err := ctx.Err()
				runnerDone <- err
				return err
			},
		}
		manager, _, _ := newTestManager(Config{}, map[TaskType]Runner{
			TaskTypeAgent: runner,
		}, nil)
		workspaceRoot := filepath.ToSlash(t.TempDir())
		created, err := manager.Create(context.Background(), CreateTaskInput{
			ID:            "task-1",
			Type:          TaskTypeAgent,
			Command:       "run",
			WorkspaceRoot: workspaceRoot,
			Start:         true,
		})
		if err != nil {
			t.Fatalf("Create returned error: %v", err)
		}
		select {
		case <-runStarted:
		case <-time.After(time.Second):
			t.Fatalf("runner did not start")
		}

		err = manager.Stop(created.ID, workspaceRoot)
		if err != nil {
			t.Errorf("Stop returned error: %v", err)
			return
		}

		select {
		case doneErr := <-runnerDone:
			if !errors.Is(doneErr, context.Canceled) {
				t.Errorf("runner error = %v, want context.Canceled", doneErr)
			}
		case <-time.After(time.Second):
			t.Errorf("runner did not stop")
			return
		}

		waitCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		stopped, timeout, waitErr := manager.Wait(waitCtx, created.ID, workspaceRoot)
		if waitErr != nil {
			t.Errorf("Wait returned error: %v", waitErr)
			return
		}
		if timeout {
			t.Errorf("timeout = true, want false")
		}
		if stopped.Status != TaskStatusStopped {
			t.Errorf("Status = %q, want %q", stopped.Status, TaskStatusStopped)
		}
		if stopped.EndTime == nil {
			t.Errorf("EndTime = nil, want non-nil")
		}
	})
}

func TestManagerGetAndList(t *testing.T) {
	t.Run("get existing task", func(t *testing.T) {
		manager, _, _ := newTestManager(Config{}, nil, nil)
		workspaceRoot := filepath.ToSlash(t.TempDir())
		created, err := manager.Create(context.Background(), CreateTaskInput{
			ID:            "task-1",
			Type:          TaskTypeAgent,
			Command:       "run",
			WorkspaceRoot: workspaceRoot,
		})
		if err != nil {
			t.Fatalf("Create returned error: %v", err)
		}

		got, ok, err := manager.Get(created.ID, workspaceRoot)
		if err != nil {
			t.Errorf("Get returned error: %v", err)
		} else if !ok {
			t.Errorf("Get returned ok=false, want true")
		} else if got.ID != created.ID {
			t.Errorf("ID = %q, want %q", got.ID, created.ID)
		}
	})

	t.Run("get non existent task", func(t *testing.T) {
		manager, _, _ := newTestManager(Config{}, nil, nil)
		_, ok, err := manager.Get("missing", filepath.ToSlash(t.TempDir()))
		if err != nil {
			t.Errorf("Get returned error: %v", err)
		}
		if ok {
			t.Errorf("ok = true, want false")
		}
	})

	t.Run("get wrong workspace", func(t *testing.T) {
		manager, _, _ := newTestManager(Config{}, nil, nil)
		workspaceRoot := filepath.ToSlash(t.TempDir())
		otherWorkspace := filepath.ToSlash(t.TempDir())
		created, err := manager.Create(context.Background(), CreateTaskInput{
			ID:            "task-1",
			Type:          TaskTypeAgent,
			Command:       "run",
			WorkspaceRoot: workspaceRoot,
		})
		if err != nil {
			t.Fatalf("Create returned error: %v", err)
		}

		_, ok, err := manager.Get(created.ID, otherWorkspace)
		if err != nil {
			t.Errorf("Get returned error: %v", err)
		}
		if ok {
			t.Errorf("ok = true, want false")
		}
	})

	t.Run("list returns tasks sorted by start time descending", func(t *testing.T) {
		manager, _, _ := newTestManager(Config{}, nil, nil)
		workspaceRoot := filepath.ToSlash(t.TempDir())
		first, err := manager.Create(context.Background(), CreateTaskInput{
			ID:            "task-1",
			Type:          TaskTypeAgent,
			Command:       "first",
			WorkspaceRoot: workspaceRoot,
		})
		if err != nil {
			t.Fatalf("first Create returned error: %v", err)
		}
		second, err := manager.Create(context.Background(), CreateTaskInput{
			ID:            "task-2",
			Type:          TaskTypeAgent,
			Command:       "second",
			WorkspaceRoot: workspaceRoot,
		})
		if err != nil {
			t.Fatalf("second Create returned error: %v", err)
		}

		older := time.Date(2026, 4, 29, 10, 0, 0, 0, time.UTC)
		newer := older.Add(time.Hour)
		manager.mu.Lock()
		manager.tasks[first.ID].StartTime = older
		manager.tasks[second.ID].StartTime = newer
		manager.mu.Unlock()

		tasks, err := manager.List(workspaceRoot)
		if err != nil {
			t.Errorf("List returned error: %v", err)
			return
		}
		if len(tasks) != 2 {
			t.Errorf("len(tasks) = %d, want 2", len(tasks))
			return
		}
		if tasks[0].ID != second.ID {
			t.Errorf("tasks[0].ID = %q, want %q", tasks[0].ID, second.ID)
		}
		if tasks[1].ID != first.ID {
			t.Errorf("tasks[1].ID = %q, want %q", tasks[1].ID, first.ID)
		}
	})

	t.Run("list with empty workspace", func(t *testing.T) {
		t.Chdir(t.TempDir())
		manager, _, _ := newTestManager(Config{}, nil, nil)

		tasks, err := manager.List("")
		if err != nil {
			t.Errorf("List returned error: %v", err)
			return
		}
		if len(tasks) != 0 {
			t.Errorf("len(tasks) = %d, want 0", len(tasks))
		}
	})
}

func TestTaskFinalResultAndResultReady(t *testing.T) {
	t.Run("no result set", func(t *testing.T) {
		task := Task{}
		if task.ResultReady() {
			t.Errorf("ResultReady = true, want false")
		}
		if _, ok := task.FinalResult(); ok {
			t.Errorf("FinalResult ok = true, want false")
		}
	})

	t.Run("result set", func(t *testing.T) {
		readyAt := time.Date(2026, 4, 29, 12, 0, 0, 0, time.UTC)
		task := Task{
			FinalResultKind:    FinalResultKindAssistantText,
			FinalResultText:    "done",
			FinalResultReadyAt: &readyAt,
		}
		if !task.ResultReady() {
			t.Errorf("ResultReady = false, want true")
		}
		result, ok := task.FinalResult()
		if !ok {
			t.Errorf("FinalResult ok = false, want true")
			return
		}
		if result.Kind != FinalResultKindAssistantText {
			t.Errorf("Kind = %q, want %q", result.Kind, FinalResultKindAssistantText)
		}
		if result.Text != "done" {
			t.Errorf("Text = %q, want %q", result.Text, "done")
		}
		if !result.ObservedAt.Equal(readyAt) {
			t.Errorf("ObservedAt = %v, want %v", result.ObservedAt, readyAt)
		}
	})

	t.Run("final result ready at nil with kind set", func(t *testing.T) {
		task := Task{
			FinalResultKind: FinalResultKindAssistantText,
			FinalResultText: "done",
		}
		if task.ResultReady() {
			t.Errorf("ResultReady = true, want false")
		}
		if _, ok := task.FinalResult(); ok {
			t.Errorf("FinalResult ok = true, want false")
		}
	})
}

func TestManagerWait(t *testing.T) {
	t.Run("already terminal", func(t *testing.T) {
		manager, _, _ := newTestManager(Config{}, nil, nil)
		workspaceRoot := filepath.ToSlash(t.TempDir())
		created, err := manager.Create(context.Background(), CreateTaskInput{
			ID:            "task-1",
			Type:          TaskTypeAgent,
			Command:       "run",
			WorkspaceRoot: workspaceRoot,
		})
		if err != nil {
			t.Fatalf("Create returned error: %v", err)
		}
		if err := manager.Stop(created.ID, workspaceRoot); err != nil {
			t.Fatalf("Stop returned error: %v", err)
		}

		got, timeout, err := manager.Wait(context.Background(), created.ID, workspaceRoot)
		if err != nil {
			t.Errorf("Wait returned error: %v", err)
			return
		}
		if timeout {
			t.Errorf("timeout = true, want false")
		}
		if got.Status != TaskStatusStopped {
			t.Errorf("Status = %q, want %q", got.Status, TaskStatusStopped)
		}
	})

	t.Run("context cancelled before terminal", func(t *testing.T) {
		manager, _, _ := newTestManager(Config{}, nil, nil)
		workspaceRoot := filepath.ToSlash(t.TempDir())
		created, err := manager.Create(context.Background(), CreateTaskInput{
			ID:            "task-1",
			Type:          TaskTypeAgent,
			Command:       "run",
			WorkspaceRoot: workspaceRoot,
		})
		if err != nil {
			t.Fatalf("Create returned error: %v", err)
		}

		ctx, cancel := context.WithCancel(context.Background())
		go func() {
			time.Sleep(20 * time.Millisecond)
			cancel()
		}()

		got, timeout, err := manager.Wait(ctx, created.ID, workspaceRoot)
		if !errors.Is(err, context.Canceled) {
			t.Errorf("error = %v, want %v", err, context.Canceled)
		}
		if timeout {
			t.Errorf("timeout = true, want false")
		}
		if got.ID != "" {
			t.Errorf("ID = %q, want empty task", got.ID)
		}
	})

	t.Run("context deadline exceeded", func(t *testing.T) {
		manager, _, _ := newTestManager(Config{}, nil, nil)
		workspaceRoot := filepath.ToSlash(t.TempDir())
		created, err := manager.Create(context.Background(), CreateTaskInput{
			ID:            "task-1",
			Type:          TaskTypeAgent,
			Command:       "run",
			WorkspaceRoot: workspaceRoot,
		})
		if err != nil {
			t.Fatalf("Create returned error: %v", err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		defer cancel()
		got, timeout, err := manager.Wait(ctx, created.ID, workspaceRoot)
		if err != nil {
			t.Errorf("Wait returned error: %v", err)
			return
		}
		if !timeout {
			t.Errorf("timeout = false, want true")
		}
		if got.ID != created.ID {
			t.Errorf("ID = %q, want %q", got.ID, created.ID)
		}
	})
}

func TestManagerSetFinalTextAndSetOutcome(t *testing.T) {
	t.Run("set final text", func(t *testing.T) {
		manager, _, _ := newTestManager(Config{}, nil, nil)
		workspaceRoot := filepath.ToSlash(t.TempDir())
		created, err := manager.Create(context.Background(), CreateTaskInput{
			ID:            "task-1",
			Type:          TaskTypeAgent,
			Command:       "run",
			WorkspaceRoot: workspaceRoot,
		})
		if err != nil {
			t.Fatalf("Create returned error: %v", err)
		}

		err = manager.SetFinalText(created.ID, "done")
		if err != nil {
			t.Errorf("SetFinalText returned error: %v", err)
			return
		}

		got, ok, getErr := manager.Get(created.ID, workspaceRoot)
		if getErr != nil {
			t.Errorf("Get returned error: %v", getErr)
		} else if !ok {
			t.Errorf("Get returned ok=false, want true")
		} else {
			if got.FinalResultKind != FinalResultKindAssistantText {
				t.Errorf("FinalResultKind = %q, want %q", got.FinalResultKind, FinalResultKindAssistantText)
			}
			if got.FinalResultText != "done" {
				t.Errorf("FinalResultText = %q, want %q", got.FinalResultText, "done")
			}
			if got.FinalResultReadyAt == nil {
				t.Errorf("FinalResultReadyAt = nil, want non-nil")
			}
		}
	})

	t.Run("set final text on non existent task", func(t *testing.T) {
		manager, _, _ := newTestManager(Config{}, nil, nil)
		err := manager.SetFinalText("missing", "done")
		if err == nil {
			t.Errorf("error = nil, want task not found error")
		}
	})

	t.Run("set outcome", func(t *testing.T) {
		manager, _, _ := newTestManager(Config{}, nil, nil)
		workspaceRoot := filepath.ToSlash(t.TempDir())
		created, err := manager.Create(context.Background(), CreateTaskInput{
			ID:            "task-1",
			Type:          TaskTypeAgent,
			Command:       "run",
			WorkspaceRoot: workspaceRoot,
		})
		if err != nil {
			t.Fatalf("Create returned error: %v", err)
		}

		err = manager.SetOutcome(created.ID, types.ChildAgentOutcomeBlocked, " blocked ")
		if err != nil {
			t.Errorf("SetOutcome returned error: %v", err)
			return
		}

		got, ok, getErr := manager.Get(created.ID, workspaceRoot)
		if getErr != nil {
			t.Errorf("Get returned error: %v", getErr)
		} else if !ok {
			t.Errorf("Get returned ok=false, want true")
		} else {
			if got.Outcome != types.ChildAgentOutcomeBlocked {
				t.Errorf("Outcome = %q, want %q", got.Outcome, types.ChildAgentOutcomeBlocked)
			}
			if got.OutcomeSummary != "blocked" {
				t.Errorf("OutcomeSummary = %q, want %q", got.OutcomeSummary, "blocked")
			}
		}
	})

	t.Run("set outcome on non existent task", func(t *testing.T) {
		manager, _, _ := newTestManager(Config{}, nil, nil)
		err := manager.SetOutcome("missing", types.ChildAgentOutcomeSuccess, "done")
		if err == nil {
			t.Errorf("error = nil, want task not found error")
		}
	})
}

func TestManagerAppend(t *testing.T) {
	t.Run("append to existing task", func(t *testing.T) {
		manager, _, _ := newTestManager(Config{}, nil, nil)
		workspaceRoot := filepath.ToSlash(t.TempDir())
		created, err := manager.Create(context.Background(), CreateTaskInput{
			ID:            "task-1",
			Type:          TaskTypeAgent,
			Command:       "run",
			WorkspaceRoot: workspaceRoot,
		})
		if err != nil {
			t.Fatalf("Create returned error: %v", err)
		}

		err = manager.Append(created.ID, []byte("hello"))
		if err != nil {
			t.Errorf("Append returned error: %v", err)
			return
		}

		got, ok, getErr := manager.Get(created.ID, workspaceRoot)
		if getErr != nil {
			t.Errorf("Get returned error: %v", getErr)
		} else if !ok {
			t.Errorf("Get returned ok=false, want true")
		} else if got.Output != "hello" {
			t.Errorf("Output = %q, want %q", got.Output, "hello")
		}
	})

	t.Run("append respects max bytes", func(t *testing.T) {
		manager, _, _ := newTestManager(Config{TaskOutputMaxBytes: 5}, nil, nil)
		workspaceRoot := filepath.ToSlash(t.TempDir())
		created, err := manager.Create(context.Background(), CreateTaskInput{
			ID:            "task-1",
			Type:          TaskTypeAgent,
			Command:       "run",
			WorkspaceRoot: workspaceRoot,
		})
		if err != nil {
			t.Fatalf("Create returned error: %v", err)
		}
		if err := manager.Append(created.ID, []byte("hello")); err != nil {
			t.Fatalf("first Append returned error: %v", err)
		}

		err = manager.Append(created.ID, []byte("world"))
		if err != nil {
			t.Errorf("Append returned error: %v", err)
			return
		}

		got, ok, getErr := manager.Get(created.ID, workspaceRoot)
		if getErr != nil {
			t.Errorf("Get returned error: %v", getErr)
		} else if !ok {
			t.Errorf("Get returned ok=false, want true")
		} else if got.Output != "hello" {
			t.Errorf("Output = %q, want %q", got.Output, "hello")
		}
	})

	t.Run("append to non existent task", func(t *testing.T) {
		manager, _, _ := newTestManager(Config{}, nil, nil)
		err := manager.Append("missing", []byte("hello"))
		if err == nil {
			t.Errorf("error = nil, want task not found error")
		}
	})
}
