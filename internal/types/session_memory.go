package types

import "time"

type SessionMemory struct {
	SessionID      string    `json:"session_id"`
	WorkspaceRoot  string    `json:"workspace_root,omitempty"`
	SourceTurnID   string    `json:"source_turn_id,omitempty"`
	UpToPosition   int       `json:"up_to_position"`
	ItemCount      int       `json:"item_count"`
	SummaryPayload string    `json:"summary_payload"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}
