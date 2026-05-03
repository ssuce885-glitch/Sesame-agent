package contracts

import "time"

type Task struct {
	ID              string    `json:"id"`
	WorkspaceRoot   string    `json:"workspace_root"`
	SessionID       string    `json:"session_id"`
	RoleID          string    `json:"role_id,omitempty"` // specialist role ID for agent tasks
	TurnID          string    `json:"turn_id,omitempty"`
	ParentSessionID string    `json:"parent_session_id,omitempty"`
	ParentTurnID    string    `json:"parent_turn_id,omitempty"`
	ReportSessionID string    `json:"report_session_id,omitempty"`
	Kind            string    `json:"kind"`  // "shell", "agent", "remote"
	State           string    `json:"state"` // "pending", "running", "completed", "failed", "cancelled"
	Prompt          string    `json:"prompt"`
	OutputPath      string    `json:"output_path,omitempty"`
	FinalText       string    `json:"final_text,omitempty"`
	Outcome         string    `json:"outcome,omitempty"` // "success", "failure"
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}
