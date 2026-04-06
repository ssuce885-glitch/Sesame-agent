package task

import (
	"context"
	"fmt"
	"io"
	"time"
)

type TaskType string

const (
	TaskTypeShell  TaskType = "shell"
	TaskTypeAgent  TaskType = "agent"
	TaskTypeRemote TaskType = "remote"
)

type TaskStatus string

const (
	TaskStatusPending   TaskStatus = "pending"
	TaskStatusRunning   TaskStatus = "running"
	TaskStatusCompleted TaskStatus = "completed"
	TaskStatusFailed    TaskStatus = "failed"
	TaskStatusStopped   TaskStatus = "stopped"
)

type Task struct {
	ID            string     `json:"id"`
	Type          TaskType   `json:"type"`
	Status        TaskStatus `json:"status"`
	Command       string     `json:"command"`
	Description   string     `json:"description,omitempty"`
	WorkspaceRoot string     `json:"workspace_root"`
	Output        string     `json:"output,omitempty"`
	OutputPath    string     `json:"output_path,omitempty"`
	Error         string     `json:"error,omitempty"`
	StartTime     time.Time  `json:"start_time"`
	EndTime       *time.Time `json:"end_time,omitempty"`
}

type TodoItem struct {
	Content    string `json:"content"`
	Status     string `json:"status"`
	ActiveForm string `json:"activeForm,omitempty"`
}

type CreateTaskInput struct {
	Type          TaskType
	Command       string
	Description   string
	WorkspaceRoot string
	Start         bool
}

type UpdateTaskInput struct {
	Status      TaskStatus
	Description string
}

func isTerminalStatus(status TaskStatus) bool {
	switch status {
	case TaskStatusCompleted, TaskStatusFailed, TaskStatusStopped:
		return true
	default:
		return false
	}
}

func validateStatusTransition(from, to TaskStatus) error {
	if from == to {
		return nil
	}
	allowed := map[TaskStatus]map[TaskStatus]struct{}{
		TaskStatusPending: {
			TaskStatusRunning: {},
			TaskStatusStopped: {},
		},
		TaskStatusRunning: {
			TaskStatusCompleted: {},
			TaskStatusFailed:    {},
			TaskStatusStopped:   {},
		},
	}
	if _, ok := allowed[from][to]; ok {
		return nil
	}
	return fmt.Errorf("invalid status transition from %q to %q", from, to)
}

type OutputSink interface {
	Append(taskID string, chunk []byte) error
}

type Runner interface {
	Run(ctx context.Context, task *Task, sink OutputSink) error
}

type AgentExecutor interface {
	RunTask(ctx context.Context, workspaceRoot string, prompt string, sink io.Writer) error
}

type RemoteExecutorConfig struct {
	ShimCommand    string
	TimeoutSeconds int
}
