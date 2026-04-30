package types

import (
	"crypto/rand"
	"encoding/hex"
	"time"
)

type SessionState string
type SessionRole string
type TurnState string
type TurnExecutionMode string
type TurnKind string

const (
	SessionRoleMainParent SessionRole = "main_parent"
)

const (
	SessionStateIdle    SessionState = "idle"
	SessionStateRunning SessionState = "running"
	SessionStateClosed  SessionState = "closed"
)

const (
	TurnStateCreated         TurnState = "created"
	TurnStateBuildingContext TurnState = "building_context"
	TurnStateModelStreaming  TurnState = "model_streaming"
	TurnStateToolDispatching TurnState = "tool_dispatching"
	TurnStateToolRunning     TurnState = "tool_running"
	TurnStateLoopContinue    TurnState = "loop_continue"
	TurnStateCompleted       TurnState = "completed"
	TurnStateInterrupted     TurnState = "interrupted"
	TurnStateFailed          TurnState = "failed"
)

const (
	TurnExecutionModeForegroundAttached TurnExecutionMode = "foreground_attached"
	TurnExecutionModeDetached           TurnExecutionMode = "detached"
)

const (
	TurnKindUserMessage TurnKind = "user_message"
	TurnKindReportBatch TurnKind = "report_batch"
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
	Kind                     TurnKind          `json:"kind,omitempty"`
	State                    TurnState         `json:"state"`
	ExecutionMode            TurnExecutionMode `json:"execution_mode,omitempty"`
	ForegroundLeaseID        string            `json:"foreground_lease_id,omitempty"`
	ForegroundLeaseExpiresAt time.Time         `json:"foreground_lease_expires_at,omitempty"`
	UserMessage              string            `json:"user_message"`
	CreatedAt                time.Time         `json:"created_at"`
	UpdatedAt                time.Time         `json:"updated_at"`
}

func NewID(prefix string) string {
	var buf [8]byte
	rand.Read(buf[:])

	return prefix + "_" + hex.EncodeToString(buf[:])
}
