package task

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestManagerCreateListGetAndUpdateTask(t *testing.T) {
	root := t.TempDir()
	manager := NewManager(Config{MaxConcurrentTasks: 8, TaskOutputMaxBytes: 1 << 20}, nil, nil)

	task, err := manager.Create(context.Background(), CreateTaskInput{
		Type:          TaskTypeShell,
		Command:       "echo hello",
		Description:   "run echo",
		WorkspaceRoot: root,
		Start:         false,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if task.Status != TaskStatusPending {
		t.Fatalf("Status = %q, want %q", task.Status, TaskStatusPending)
	}

	listed, err := manager.List(root)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(listed) != 1 || listed[0].ID != task.ID {
		t.Fatalf("List() = %#v, want task %q", listed, task.ID)
	}

	got, ok, err := manager.Get(task.ID, root)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if !ok || got.ID != task.ID {
		t.Fatalf("Get() = %#v, %v, want task %q", got, ok, task.ID)
	}

	if err := manager.Update(task.ID, root, UpdateTaskInput{
		Status:      TaskStatusStopped,
		Description: "stopped before start",
	}); err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	updated, ok, err := manager.Get(task.ID, root)
	if err != nil {
		t.Fatalf("Get() after update error = %v", err)
	}
	if !ok {
		t.Fatal("Get() after update ok = false, want true")
	}
	if updated.Status != TaskStatusStopped {
		t.Fatalf("updated.Status = %q, want %q", updated.Status, TaskStatusStopped)
	}
	if updated.Description != "stopped before start" {
		t.Fatalf("updated.Description = %q, want %q", updated.Description, "stopped before start")
	}
	if updated.EndTime == nil {
		t.Fatal("updated.EndTime = nil, want terminal timestamp")
	}
}

func TestManagerReloadMarksRunningTasksFailed(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	raw := `{"tasks":[{"id":"task_1","type":"shell","status":"running","command":"echo hi","workspace_root":"` + filepath.ToSlash(root) + `","start_time":"2026-04-06T10:00:00Z"}]}`
	if err := os.WriteFile(filepath.Join(root, ".claude", "tasks.json"), []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}

	manager := NewManager(Config{MaxConcurrentTasks: 8, TaskOutputMaxBytes: 1 << 20}, nil, nil)

	task, ok, err := manager.Get("task_1", root)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if !ok {
		t.Fatal("Get() ok = false, want true")
	}
	if task.Status != TaskStatusFailed {
		t.Fatalf("Status = %q, want %q", task.Status, TaskStatusFailed)
	}
	if !strings.Contains(task.Error, "process restart") {
		t.Fatalf("Error = %q, want restart marker", task.Error)
	}
}

func TestManagerRunsShellTaskAndCapturesOutput(t *testing.T) {
	root := t.TempDir()
	manager := NewManager(Config{MaxConcurrentTasks: 8, TaskOutputMaxBytes: 1 << 20}, nil, nil)

	task, err := manager.Create(context.Background(), CreateTaskInput{
		Type:          TaskTypeShell,
		Command:       "echo shell-task",
		WorkspaceRoot: root,
		Start:         true,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	waitForTaskTerminal(t, manager, task.ID, root)
	got, ok, err := manager.Get(task.ID, root)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if !ok {
		t.Fatal("Get() ok = false, want true")
	}
	if got.Status != TaskStatusCompleted {
		t.Fatalf("Status = %q, want %q", got.Status, TaskStatusCompleted)
	}

	output, err := manager.ReadOutput(task.ID, root)
	if err != nil {
		t.Fatalf("ReadOutput() error = %v", err)
	}
	if !strings.Contains(output, "shell-task") {
		t.Fatalf("output = %q, want shell-task", output)
	}
}

func TestManagerStopsRunningShellTask(t *testing.T) {
	root := t.TempDir()
	manager := NewManager(Config{MaxConcurrentTasks: 8, TaskOutputMaxBytes: 1 << 20}, nil, nil)

	task, err := manager.Create(context.Background(), CreateTaskInput{
		Type:          TaskTypeShell,
		Command:       "ping -n 6 127.0.0.1 > nul",
		WorkspaceRoot: root,
		Start:         true,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if err := manager.Stop(task.ID, root); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	waitForTaskTerminal(t, manager, task.ID, root)

	got, ok, err := manager.Get(task.ID, root)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if !ok {
		t.Fatal("Get() ok = false, want true")
	}
	if got.Status != TaskStatusStopped {
		t.Fatalf("Status = %q, want %q", got.Status, TaskStatusStopped)
	}
}

func TestManagerRunsAgentTaskThroughExecutor(t *testing.T) {
	root := t.TempDir()
	executor := &fakeAgentExecutor{output: "agent:summarize the workspace"}
	manager := NewManager(Config{MaxConcurrentTasks: 8, TaskOutputMaxBytes: 1 << 20}, nil, executor)

	task, err := manager.Create(context.Background(), CreateTaskInput{
		Type:          TaskTypeAgent,
		Command:       "summarize the workspace",
		WorkspaceRoot: root,
		Start:         true,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	waitForTaskTerminal(t, manager, task.ID, root)
	output, err := manager.ReadOutput(task.ID, root)
	if err != nil {
		t.Fatalf("ReadOutput() error = %v", err)
	}
	if output != "agent:summarize the workspace" {
		t.Fatalf("output = %q, want %q", output, "agent:summarize the workspace")
	}
}

func TestManagerRunsRemoteTaskThroughShim(t *testing.T) {
	root := t.TempDir()
	shim := filepath.Join(root, "remote-shim.cmd")
	script := "@echo off\r\necho remote:%~1\r\n"
	if err := os.WriteFile(shim, []byte(script), 0o644); err != nil {
		t.Fatal(err)
	}

	manager := NewManager(Config{MaxConcurrentTasks: 8, TaskOutputMaxBytes: 1 << 20}, nil, nil)
	manager.SetRemoteConfig(RemoteExecutorConfig{ShimCommand: shim, TimeoutSeconds: 30})

	task, err := manager.Create(context.Background(), CreateTaskInput{
		Type:          TaskTypeRemote,
		Command:       "deploy now",
		WorkspaceRoot: root,
		Start:         true,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	waitForTaskTerminal(t, manager, task.ID, root)
	output, err := manager.ReadOutput(task.ID, root)
	if err != nil {
		t.Fatalf("ReadOutput() error = %v", err)
	}
	if !strings.Contains(output, "remote:deploy now") {
		t.Fatalf("output = %q, want remote command echo", output)
	}
}

func TestManagerRejectsRemoteTaskWhenShimMissing(t *testing.T) {
	root := t.TempDir()
	manager := NewManager(Config{MaxConcurrentTasks: 8, TaskOutputMaxBytes: 1 << 20}, nil, nil)

	_, err := manager.Create(context.Background(), CreateTaskInput{
		Type:          TaskTypeRemote,
		Command:       "deploy now",
		WorkspaceRoot: root,
		Start:         true,
	})
	if err == nil || !strings.Contains(err.Error(), "not configured") {
		t.Fatalf("Create() error = %v, want not configured", err)
	}
}

func waitForTaskTerminal(t *testing.T, manager *Manager, taskID, workspaceRoot string) {
	t.Helper()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		got, ok, err := manager.Get(taskID, workspaceRoot)
		if err != nil {
			t.Fatalf("Get() error = %v", err)
		}
		if ok && isTerminalStatus(got.Status) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}

	t.Fatalf("task %q did not reach terminal state", taskID)
}

func TestStatusTransitionHelpers(t *testing.T) {
	t.Run("terminal status detection", func(t *testing.T) {
		if !isTerminalStatus(TaskStatusCompleted) {
			t.Fatal("TaskStatusCompleted should be terminal")
		}
		if !isTerminalStatus(TaskStatusFailed) {
			t.Fatal("TaskStatusFailed should be terminal")
		}
		if !isTerminalStatus(TaskStatusStopped) {
			t.Fatal("TaskStatusStopped should be terminal")
		}
		if isTerminalStatus(TaskStatusPending) {
			t.Fatal("TaskStatusPending should not be terminal")
		}
		if isTerminalStatus(TaskStatusRunning) {
			t.Fatal("TaskStatusRunning should not be terminal")
		}
	})

	t.Run("valid transitions", func(t *testing.T) {
		valid := [][2]TaskStatus{
			{TaskStatusPending, TaskStatusPending},
			{TaskStatusRunning, TaskStatusRunning},
			{TaskStatusCompleted, TaskStatusCompleted},
			{TaskStatusPending, TaskStatusRunning},
			{TaskStatusPending, TaskStatusStopped},
			{TaskStatusRunning, TaskStatusCompleted},
			{TaskStatusRunning, TaskStatusFailed},
			{TaskStatusRunning, TaskStatusStopped},
		}

		for _, pair := range valid {
			if err := validateStatusTransition(pair[0], pair[1]); err != nil {
				t.Fatalf("validateStatusTransition(%q, %q) error = %v, want nil", pair[0], pair[1], err)
			}
		}
	})

	t.Run("invalid transitions", func(t *testing.T) {
		invalid := [][2]TaskStatus{
			{TaskStatusPending, TaskStatusCompleted},
			{TaskStatusPending, TaskStatusFailed},
			{TaskStatusRunning, TaskStatusPending},
			{TaskStatusCompleted, TaskStatusRunning},
			{TaskStatusFailed, TaskStatusRunning},
			{TaskStatusStopped, TaskStatusRunning},
		}

		for _, pair := range invalid {
			if err := validateStatusTransition(pair[0], pair[1]); err == nil {
				t.Fatalf("validateStatusTransition(%q, %q) error = nil, want error", pair[0], pair[1])
			}
		}
	})
}

func TestTaskStoreRoundTrip(t *testing.T) {
	t.Run("tasks round trip through write and load", func(t *testing.T) {
		start := time.Date(2026, 4, 6, 10, 0, 0, 0, time.UTC)
		end := start.Add(2 * time.Minute)
		tasks := []Task{
			{
				ID:            "task_1",
				Type:          TaskTypeShell,
				Status:        TaskStatusCompleted,
				Command:       "echo hi",
				Description:   "test task",
				WorkspaceRoot: filepath.ToSlash(t.TempDir()),
				Output:        "hi",
				OutputPath:    "output/task_1.log",
				StartTime:     start,
				EndTime:       &end,
			},
			{
				ID:            "task_2",
				Type:          TaskTypeAgent,
				Status:        TaskStatusRunning,
				Command:       "summarize",
				WorkspaceRoot: filepath.ToSlash(t.TempDir()),
				StartTime:     start.Add(time.Minute),
			},
		}

		path := filepath.Join(t.TempDir(), "tasks.json")
		if err := writeTasksFile(path, tasks); err != nil {
			t.Fatalf("writeTasksFile() error = %v", err)
		}

		got, err := loadTasksFile(path)
		if err != nil {
			t.Fatalf("loadTasksFile() error = %v", err)
		}
		if !reflect.DeepEqual(got, tasks) {
			t.Fatalf("loadTasksFile() = %#v, want %#v", got, tasks)
		}
	})

	t.Run("todos file writes valid json payload", func(t *testing.T) {
		todos := []TodoItem{
			{Content: "Plan task runner", Status: "in_progress", ActiveForm: "Planning task runner"},
			{Content: "Add tests", Status: "pending"},
		}

		path := filepath.Join(t.TempDir(), "todos.json")
		if err := writeTodosFile(path, todos); err != nil {
			t.Fatalf("writeTodosFile() error = %v", err)
		}

		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("ReadFile() error = %v", err)
		}

		var got []TodoItem
		if err := json.Unmarshal(data, &got); err != nil {
			t.Fatalf("Unmarshal() error = %v", err)
		}
		if !reflect.DeepEqual(got, todos) {
			t.Fatalf("todos json = %#v, want %#v", got, todos)
		}
	})
}

func TestAgentRunnerRun(t *testing.T) {
	t.Run("forwards workspace, prompt, and output", func(t *testing.T) {
		executor := &fakeAgentExecutor{
			output: "agent:workspace summary",
		}
		sink := &fakeOutputSink{}
		runner := NewAgentRunner(executor)

		task := &Task{
			ID:            "task_123",
			WorkspaceRoot: "E:/project/go-agent",
			Command:       "workspace summary",
		}

		if err := runner.Run(context.Background(), task, sink); err != nil {
			t.Fatalf("Run() error = %v", err)
		}
		if executor.gotWorkspaceRoot != task.WorkspaceRoot {
			t.Fatalf("workspaceRoot = %q, want %q", executor.gotWorkspaceRoot, task.WorkspaceRoot)
		}
		if executor.gotPrompt != task.Command {
			t.Fatalf("prompt = %q, want %q", executor.gotPrompt, task.Command)
		}
		if got := sink.outputFor(task.ID); got != executor.output {
			t.Fatalf("sink output = %q, want %q", got, executor.output)
		}
	})

	t.Run("returns configuration error when executor is nil", func(t *testing.T) {
		runner := NewAgentRunner(nil)
		err := runner.Run(context.Background(), &Task{ID: "task_nil"}, &fakeOutputSink{})
		if !errors.Is(err, errAgentExecutorNotConfigured) {
			t.Fatalf("Run() error = %v, want %v", err, errAgentExecutorNotConfigured)
		}
	})
}

type fakeAgentExecutor struct {
	gotWorkspaceRoot string
	gotPrompt        string
	output           string
	err              error
}

func (f *fakeAgentExecutor) RunTask(ctx context.Context, workspaceRoot string, prompt string, sink io.Writer) error {
	f.gotWorkspaceRoot = workspaceRoot
	f.gotPrompt = prompt
	if f.err != nil {
		return f.err
	}
	_, err := sink.Write([]byte(f.output))
	return err
}

type fakeOutputSink struct {
	chunks map[string][]byte
}

func (f *fakeOutputSink) Append(taskID string, chunk []byte) error {
	if f.chunks == nil {
		f.chunks = make(map[string][]byte)
	}
	f.chunks[taskID] = append(f.chunks[taskID], chunk...)
	return nil
}

func (f *fakeOutputSink) outputFor(taskID string) string {
	if f.chunks == nil {
		return ""
	}
	return string(f.chunks[taskID])
}
