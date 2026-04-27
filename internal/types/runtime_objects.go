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
	PlanFile     string    `json:"plan_file,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type Task struct {
	ID              string    `json:"id"`
	RunID           string    `json:"run_id"`
	PlanID          string    `json:"plan_id,omitempty"`
	ParentTaskID    string    `json:"parent_task_id,omitempty"`
	State           TaskState `json:"state"`
	Title           string    `json:"title,omitempty"`
	Description     string    `json:"description,omitempty"`
	Owner           string    `json:"owner,omitempty"`
	Kind            string    `json:"kind,omitempty"`
	ExecutionTaskID string    `json:"execution_task_id,omitempty"`
	WorktreeID      string    `json:"worktree_id,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type TaskRecord = Task

type ToolRun struct {
	ID           string       `json:"id"`
	RunID        string       `json:"run_id"`
	TaskID       string       `json:"task_id,omitempty"`
	State        ToolRunState `json:"state"`
	ToolName     string       `json:"tool_name"`
	ToolCallID   string       `json:"tool_call_id,omitempty"`
	InputJSON    string       `json:"input_json,omitempty"`
	OutputJSON   string       `json:"output_json,omitempty"`
	Error        string       `json:"error,omitempty"`
	ResourceKeys []string     `json:"resource_keys,omitempty"`
	LockWaitMs   int64        `json:"lock_wait_ms,omitempty"`
	StartedAt    time.Time    `json:"started_at,omitempty"`
	CompletedAt  time.Time    `json:"completed_at,omitempty"`
	CreatedAt    time.Time    `json:"created_at"`
	UpdatedAt    time.Time    `json:"updated_at"`
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

type RuntimeDiagnostic struct {
	ID         string    `json:"id"`
	SessionID  string    `json:"session_id"`
	TurnID     string    `json:"turn_id,omitempty"`
	EventType  string    `json:"event_type"`
	Category   string    `json:"category,omitempty"`
	Severity   string    `json:"severity,omitempty"`
	Reason     string    `json:"reason,omitempty"`
	Summary    string    `json:"summary,omitempty"`
	RepairHint string    `json:"repair_hint,omitempty"`
	AssetKind  string    `json:"asset_kind,omitempty"`
	AssetID    string    `json:"asset_id,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

type RuntimeGraph struct {
	Runs        []Run               `json:"runs"`
	Plans       []Plan              `json:"plans"`
	Tasks       []Task              `json:"tasks"`
	ToolRuns    []ToolRun           `json:"tool_runs"`
	Worktrees   []Worktree          `json:"worktrees"`
	Diagnostics []RuntimeDiagnostic `json:"diagnostics"`
}
