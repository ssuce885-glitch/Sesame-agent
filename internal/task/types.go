package task

import (
	"context"
	"errors"
	"fmt"
	"time"
)

type TaskType string

const (
	TaskTypeTodo   TaskType = "todo"
	TaskTypeAction TaskType = "action"
)

type TaskStatus string

const (
	TaskStatusPending   TaskStatus = "pending"
	TaskStatusRunning   TaskStatus = "running"
	TaskStatusCompleted TaskStatus = "completed"
	TaskStatusFailed    TaskStatus = "failed"
	TaskStatusCancelled TaskStatus = "cancelled"
)

type Task struct {
	ID              string     `json:"id"`
	ParentSessionID string     `json:"parent_session_id"`
	Type            TaskType   `json:"type"`
	Status          TaskStatus `json:"status"`
	Title           string     `json:"title,omitempty"`
	Instruction     string     `json:"instruction,omitempty"`
	Output          string     `json:"output,omitempty"`
	Error           string     `json:"error,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
	StartedAt       *time.Time `json:"started_at,omitempty"`
	CompletedAt     *time.Time `json:"completed_at,omitempty"`
}

type TodoItem struct {
	ID        string    `json:"id"`
	TaskID    string    `json:"task_id"`
	Content   string    `json:"content"`
	Done      bool      `json:"done"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type CreateTaskInput struct {
	ParentSessionID string
	Type            TaskType
	Title           string
	Instruction     string
}

type UpdateTaskInput struct {
	Status  *TaskStatus
	Output  *string
	Error   *string
	Todo    *[]TodoItem
	Updated time.Time
}

var errInvalidStatusTransition = errors.New("invalid task status transition")

func isTerminalStatus(status TaskStatus) bool {
	switch status {
	case TaskStatusCompleted, TaskStatusFailed, TaskStatusCancelled:
		return true
	default:
		return false
	}
}

func validateStatusTransition(current, next TaskStatus) error {
	if current == next {
		return nil
	}

	switch current {
	case TaskStatusPending:
		if next == TaskStatusRunning || next == TaskStatusCancelled {
			return nil
		}
	case TaskStatusRunning:
		if next == TaskStatusCompleted || next == TaskStatusFailed || next == TaskStatusCancelled {
			return nil
		}
	}

	if isTerminalStatus(current) {
		return fmt.Errorf("%w: %s -> %s", errInvalidStatusTransition, current, next)
	}

	return fmt.Errorf("%w: %s -> %s", errInvalidStatusTransition, current, next)
}

type OutputSink interface {
	WriteOutput(taskID string, chunk []byte) error
}

type Runner interface {
	Run(ctx context.Context, task Task, sink OutputSink) error
}

type AgentExecutor interface {
	Execute(ctx context.Context, task Task, sink OutputSink, remote RemoteExecutorConfig) error
}

type RemoteExecutorConfig struct {
	ShimCommand    string
	TimeoutSeconds int
}
