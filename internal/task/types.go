package task

import (
	"context"
	"fmt"
	"go-agent/internal/types"
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

type FinalResultKind string

const (
	FinalResultKindAssistantText FinalResultKind = "assistant_text"
)

type FinalResult struct {
	Kind       FinalResultKind `json:"kind"`
	Text       string          `json:"text,omitempty"`
	ObservedAt time.Time       `json:"observed_at"`
}

type Task struct {
	ID                   string                  `json:"id"`
	Type                 TaskType                `json:"type"`
	Status               TaskStatus              `json:"status"`
	Command              string                  `json:"command"`
	Description          string                  `json:"description,omitempty"`
	ParentTaskID         string                  `json:"parent_task_id,omitempty"`
	Owner                string                  `json:"owner,omitempty"`
	Kind                 string                  `json:"kind,omitempty"`
	ExecutionTaskID      string                  `json:"execution_task_id,omitempty"`
	WorktreeID           string                  `json:"worktree_id,omitempty"`
	ScheduledJobID       string                  `json:"scheduled_job_id,omitempty"`
	ActivatedSkillNames  []string                `json:"activated_skill_names,omitempty"`
	TargetRole           string                  `json:"target_role,omitempty"`
	WorkspaceRoot        string                  `json:"workspace_root"`
	Output               string                  `json:"output,omitempty"`
	OutputPath           string                  `json:"output_path,omitempty"`
	Error                string                  `json:"error,omitempty"`
	TimeoutSeconds       int                     `json:"timeout_seconds,omitempty"`
	StartTime            time.Time               `json:"start_time"`
	EndTime              *time.Time              `json:"end_time,omitempty"`
	ParentSessionID      string                  `json:"-"`
	ParentTurnID         string                  `json:"-"`
	Outcome              types.ChildAgentOutcome `json:"outcome,omitempty"`
	OutcomeSummary       string                  `json:"outcome_summary,omitempty"`
	FinalResultKind      FinalResultKind         `json:"-"`
	FinalResultText      string                  `json:"-"`
	FinalResultReadyAt   *time.Time              `json:"-"`
	CompletionNotifiedAt *time.Time              `json:"-"`
}

type TodoItem struct {
	Content    string `json:"content"`
	Status     string `json:"status"`
	ActiveForm string `json:"activeForm,omitempty"`
}

type WorkspaceStore interface {
	ListWorkspaceTasks(ctx context.Context, workspaceRoot string) ([]Task, error)
	UpsertWorkspaceTask(ctx context.Context, task Task) error
	GetWorkspaceTodos(ctx context.Context, workspaceRoot string) ([]TodoItem, error)
	ReplaceWorkspaceTodos(ctx context.Context, workspaceRoot string, todos []TodoItem) error
}

type CreateTaskInput struct {
	ID                  string
	Type                TaskType
	Command             string
	Description         string
	ParentTaskID        string
	ParentSessionID     string
	ParentTurnID        string
	Owner               string
	Kind                string
	WorktreeID          string
	ScheduledJobID      string
	ActivatedSkillNames []string
	TargetRole          string
	WorkspaceRoot       string
	TimeoutSeconds      int
	Start               bool
}

type UpdateTaskInput struct {
	Status      TaskStatus
	Description string
	Owner       string
	WorktreeID  string
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

type AgentTaskObserver interface {
	AppendLog(chunk []byte) error
	SetFinalText(text string) error
	SetOutcome(outcome types.ChildAgentOutcome, summary string) error
	SetRunContext(sessionID, turnID string) error
}

type AgentExecutor interface {
	RunTask(ctx context.Context, taskID string, workspaceRoot string, prompt string, activatedSkillNames []string, targetRole string, observer AgentTaskObserver) error
}

type TerminalNotifier interface {
	NotifyTaskTerminal(ctx context.Context, task Task) error
}

type RemoteExecutorConfig struct {
	ShimCommand    string
	TimeoutSeconds int
}

func (t Task) ResultReady() bool {
	return t.FinalResultReadyAt != nil && t.FinalResultKind != ""
}

func (t Task) FinalResult() (FinalResult, bool) {
	if !t.ResultReady() || t.FinalResultReadyAt == nil {
		return FinalResult{}, false
	}
	return FinalResult{
		Kind:       t.FinalResultKind,
		Text:       t.FinalResultText,
		ObservedAt: *t.FinalResultReadyAt,
	}, true
}
