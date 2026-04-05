package types

import "time"

type RunState string
type PlanState string
type TaskState string
type ToolRunState string
type WorktreeState string

const (
	RunStatePending     RunState = "pending"
	RunStateRunning     RunState = "running"
	RunStateCompleted   RunState = "completed"
	RunStateFailed      RunState = "failed"
	RunStateInterrupted RunState = "interrupted"
)

const (
	PlanStateDraft     PlanState = "draft"
	PlanStateActive    PlanState = "active"
	PlanStateApproved  PlanState = "approved"
	PlanStateCompleted PlanState = "completed"
	PlanStateFailed    PlanState = "failed"
)

const (
	TaskStatePending   TaskState = "pending"
	TaskStateRunning   TaskState = "running"
	TaskStateCompleted TaskState = "completed"
	TaskStateFailed    TaskState = "failed"
	TaskStateCancelled TaskState = "cancelled"
)

const (
	ToolRunStatePending   ToolRunState = "pending"
	ToolRunStateRunning   ToolRunState = "running"
	ToolRunStateCompleted ToolRunState = "completed"
	ToolRunStateFailed    ToolRunState = "failed"
	ToolRunStateCancelled ToolRunState = "cancelled"
)

const (
	WorktreeStateActive   WorktreeState = "active"
	WorktreeStateDetached WorktreeState = "detached"
	WorktreeStateRemoved  WorktreeState = "removed"
)

type Run struct {
	ID        string    `json:"id"`
	SessionID string    `json:"session_id"`
	TurnID    string    `json:"turn_id,omitempty"`
	State     RunState  `json:"state"`
	Objective string    `json:"objective,omitempty"`
	Result    string    `json:"result,omitempty"`
	Error     string    `json:"error,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Plan struct {
	ID           string    `json:"id"`
	RunID        string    `json:"run_id"`
	State        PlanState `json:"state"`
	Title        string    `json:"title,omitempty"`
	Summary      string    `json:"summary,omitempty"`
	ParentPlanID string    `json:"parent_plan_id,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type Task struct {
	ID          string    `json:"id"`
	RunID       string    `json:"run_id"`
	PlanID      string    `json:"plan_id,omitempty"`
	State       TaskState `json:"state"`
	Title       string    `json:"title,omitempty"`
	Description string    `json:"description,omitempty"`
	Owner       string    `json:"owner,omitempty"`
	WorktreeID  string    `json:"worktree_id,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type TaskRecord = Task

type ToolRun struct {
	ID          string       `json:"id"`
	RunID       string       `json:"run_id"`
	TaskID      string       `json:"task_id,omitempty"`
	State       ToolRunState `json:"state"`
	ToolName    string       `json:"tool_name"`
	InputJSON   string       `json:"input_json,omitempty"`
	OutputJSON  string       `json:"output_json,omitempty"`
	Error       string       `json:"error,omitempty"`
	StartedAt   time.Time    `json:"started_at,omitempty"`
	CompletedAt time.Time    `json:"completed_at,omitempty"`
	CreatedAt   time.Time    `json:"created_at"`
	UpdatedAt   time.Time    `json:"updated_at"`
}

type Worktree struct {
	ID             string        `json:"id"`
	RunID          string        `json:"run_id"`
	TaskID         string        `json:"task_id,omitempty"`
	State          WorktreeState `json:"state"`
	WorktreePath   string        `json:"worktree_path"`
	WorktreeBranch string        `json:"worktree_branch,omitempty"`
	CreatedAt      time.Time     `json:"created_at"`
	UpdatedAt      time.Time     `json:"updated_at"`
}

type RuntimeGraph struct {
	Runs      []Run      `json:"runs"`
	Plans     []Plan     `json:"plans"`
	Tasks     []Task     `json:"tasks"`
	ToolRuns  []ToolRun  `json:"tool_runs"`
	Worktrees []Worktree `json:"worktrees"`
}
