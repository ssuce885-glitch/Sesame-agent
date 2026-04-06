package task

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestManagerRegistersTaskSession(t *testing.T) {
	manager := NewManager()
	task := manager.Create("sess_parent", "D:/work/demo")
	if task.ParentSessionID != "sess_parent" {
		t.Fatalf("ParentSessionID = %q, want %q", task.ParentSessionID, "sess_parent")
	}
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
