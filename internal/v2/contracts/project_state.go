package contracts

import "time"

// ProjectState is a workspace-scoped state document used to keep long-running
// project context compact and current.
type ProjectState struct {
	WorkspaceRoot   string    `json:"workspace_root"`
	Summary         string    `json:"summary"`
	SourceSessionID string    `json:"source_session_id,omitempty"`
	SourceTurnID    string    `json:"source_turn_id,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}
