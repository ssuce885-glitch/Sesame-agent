package types

import "time"

type ContextHeadSummary struct {
	SessionID      string    `json:"session_id"`
	ContextHeadID  string    `json:"context_head_id"`
	WorkspaceRoot  string    `json:"workspace_root,omitempty"`
	SourceTurnID   string    `json:"source_turn_id,omitempty"`
	UpToItemID     int64     `json:"up_to_item_id"`
	ItemCount      int       `json:"item_count"`
	SummaryPayload string    `json:"summary_payload"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}
