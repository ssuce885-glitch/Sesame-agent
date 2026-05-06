package contracts

import "context"

// SessionManager handles session lifecycle and turn queuing.
// Ported from v1's well-designed session.Manager.
type SessionManager interface {
	Register(session Session)
	SubmitTurn(ctx context.Context, sessionID string, input SubmitTurnInput) (string, error)
	CancelTurn(sessionID, turnID string) bool
	QueuePayload(sessionID string) (QueuePayload, bool)
}

type SubmitTurnInput struct {
	Turn                 Turn                  `json:"turn"`
	TaskID               string                `json:"task_id,omitempty"`
	InstructionConflicts []InstructionConflict `json:"instruction_conflicts,omitempty"`
	TaskObserver         TaskObserver          `json:"-"`
	ActivatedSkillNames  []string              `json:"activated_skill_names,omitempty"`
	RoleSpec             *RoleSpec             `json:"role_spec,omitempty"`
}

type TaskObserver interface {
	AppendLog([]byte) error
	SetFinalText(string) error
	SetOutcome(outcome, summary string) error
}

type QueuePayload struct {
	ActiveTurnID        string `json:"active_turn_id"`
	ActiveTurnKind      string `json:"active_turn_kind"`
	QueueDepth          int    `json:"queue_depth"`
	QueuedUserTurns     int    `json:"queued_user_turns"`
	QueuedReportBatches int    `json:"queued_report_batches"`
}
