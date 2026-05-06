package contracts

import "time"

// RoleRuntimeState is a workspace-scoped dashboard document for one role.
type RoleRuntimeState struct {
	WorkspaceRoot   string    `json:"workspace_root"`
	RoleID          string    `json:"role_id"`
	Summary         string    `json:"summary"`
	SourceSessionID string    `json:"source_session_id,omitempty"`
	SourceTurnID    string    `json:"source_turn_id,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}
