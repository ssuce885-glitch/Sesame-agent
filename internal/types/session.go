package types

import "time"

type SessionState string
type TurnState string

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

type Session struct {
	ID            string       `json:"id"`
	WorkspaceRoot string       `json:"workspace_root"`
	SystemPrompt  string       `json:"system_prompt,omitempty"`
	State         SessionState `json:"state"`
	ActiveTurnID  string       `json:"active_turn_id,omitempty"`
	CreatedAt     time.Time    `json:"created_at"`
	UpdatedAt     time.Time    `json:"updated_at"`
}

type Turn struct {
	ID           string    `json:"id"`
	SessionID    string    `json:"session_id"`
	ClientTurnID string    `json:"client_turn_id,omitempty"`
	State        TurnState `json:"state"`
	UserMessage  string    `json:"user_message"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}
