package types

import "time"

type PendingTaskCompletion struct {
	ID             string    `json:"id"`
	SessionID      string    `json:"session_id"`
	ParentTurnID   string    `json:"parent_turn_id,omitempty"`
	TaskID         string    `json:"task_id"`
	TaskType       string    `json:"task_type,omitempty"`
	Command        string    `json:"command,omitempty"`
	Description    string    `json:"description,omitempty"`
	ResultKind     string    `json:"result_kind,omitempty"`
	ResultText     string    `json:"result_text,omitempty"`
	ResultPreview  string    `json:"result_preview,omitempty"`
	ObservedAt     time.Time `json:"observed_at,omitempty"`
	InjectedTurnID string    `json:"injected_turn_id,omitempty"`
	InjectedAt     time.Time `json:"injected_at,omitempty"`
	CreatedAt      time.Time `json:"created_at,omitempty"`
	UpdatedAt      time.Time `json:"updated_at,omitempty"`
}
