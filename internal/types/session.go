package types

import "time"

type SessionState string
type TurnState string
type TurnExecutionMode string

const (
	SessionStateIdle               SessionState = "idle"
	SessionStateRunning            SessionState = "running"
	SessionStateAwaitingPermission SessionState = "awaiting_permission"
	SessionStateClosed             SessionState = "closed"
)

const (
	TurnStateCreated            TurnState = "created"
	TurnStateBuildingContext    TurnState = "building_context"
	TurnStateModelStreaming     TurnState = "model_streaming"
	TurnStateToolDispatching    TurnState = "tool_dispatching"
	TurnStateAwaitingPermission TurnState = "awaiting_permission"
	TurnStateToolRunning        TurnState = "tool_running"
	TurnStateLoopContinue       TurnState = "loop_continue"
	TurnStateCompleted          TurnState = "completed"
	TurnStateInterrupted        TurnState = "interrupted"
	TurnStateFailed             TurnState = "failed"
)

const (
	TurnExecutionModeForegroundAttached TurnExecutionMode = "foreground_attached"
	TurnExecutionModeDetached           TurnExecutionMode = "detached"
)

type Session struct {
	ID                string       `json:"id"`
	WorkspaceRoot     string       `json:"workspace_root"`
	SystemPrompt      string       `json:"system_prompt,omitempty"`
	PermissionProfile string       `json:"permission_profile,omitempty"`
	State             SessionState `json:"state"`
	ActiveTurnID      string       `json:"active_turn_id,omitempty"`
	CreatedAt         time.Time    `json:"created_at"`
	UpdatedAt         time.Time    `json:"updated_at"`
}

type Turn struct {
	ID                       string            `json:"id"`
	SessionID                string            `json:"session_id"`
	ContextHeadID            string            `json:"context_head_id,omitempty"`
	ClientTurnID             string            `json:"client_turn_id,omitempty"`
	State                    TurnState         `json:"state"`
	ExecutionMode            TurnExecutionMode `json:"execution_mode,omitempty"`
	ForegroundLeaseID        string            `json:"foreground_lease_id,omitempty"`
	ForegroundLeaseExpiresAt time.Time         `json:"foreground_lease_expires_at,omitempty"`
	UserMessage              string            `json:"user_message"`
	CreatedAt                time.Time         `json:"created_at"`
	UpdatedAt                time.Time         `json:"updated_at"`
}

type TurnContinuationState string

const (
	TurnContinuationStatePending  TurnContinuationState = "pending"
	TurnContinuationStateResumed  TurnContinuationState = "resumed"
	TurnContinuationStateCanceled TurnContinuationState = "canceled"
)

type TurnContinuation struct {
	ID                  string                `json:"id"`
	SessionID           string                `json:"session_id"`
	TurnID              string                `json:"turn_id"`
	RunID               string                `json:"run_id,omitempty"`
	TaskID              string                `json:"task_id,omitempty"`
	PermissionRequestID string                `json:"permission_request_id,omitempty"`
	ToolRunID           string                `json:"tool_run_id,omitempty"`
	ToolCallID          string                `json:"tool_call_id,omitempty"`
	ToolName            string                `json:"tool_name,omitempty"`
	RequestedProfile    string                `json:"requested_profile,omitempty"`
	Reason              string                `json:"reason,omitempty"`
	State               TurnContinuationState `json:"state"`
	Decision            string                `json:"decision,omitempty"`
	DecisionScope       string                `json:"decision_scope,omitempty"`
	CreatedAt           time.Time             `json:"created_at"`
	UpdatedAt           time.Time             `json:"updated_at"`
}

type TurnResume struct {
	ContinuationID             string `json:"continuation_id"`
	PermissionRequestID        string `json:"permission_request_id,omitempty"`
	ToolRunID                  string `json:"tool_run_id,omitempty"`
	ToolCallID                 string `json:"tool_call_id,omitempty"`
	ToolName                   string `json:"tool_name,omitempty"`
	RequestedProfile           string `json:"requested_profile,omitempty"`
	Reason                     string `json:"reason,omitempty"`
	Decision                   string `json:"decision,omitempty"`
	DecisionScope              string `json:"decision_scope,omitempty"`
	EffectivePermissionProfile string `json:"effective_permission_profile,omitempty"`
	RunID                      string `json:"run_id,omitempty"`
	TaskID                     string `json:"task_id,omitempty"`
}
