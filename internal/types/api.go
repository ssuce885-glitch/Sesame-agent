package types

import "time"

type CreateSessionRequest struct {
	WorkspaceRoot string `json:"workspace_root"`
}

type SubmitTurnRequest struct {
	ClientTurnID string `json:"client_turn_id"`
	Message      string `json:"message"`
}

type PermissionDecisionRequest struct {
	Decision string `json:"decision"`
}

type SessionListItem struct {
	ID            string       `json:"id"`
	Title         string       `json:"title,omitempty"`
	LastPreview   string       `json:"last_preview,omitempty"`
	WorkspaceRoot string       `json:"workspace_root"`
	State         SessionState `json:"state"`
	ActiveTurnID  string       `json:"active_turn_id,omitempty"`
	CreatedAt     time.Time    `json:"created_at"`
	UpdatedAt     time.Time    `json:"updated_at"`
	IsSelected    bool         `json:"is_selected"`
}

type ListSessionsResponse struct {
	Sessions          []SessionListItem `json:"sessions"`
	SelectedSessionID string            `json:"selected_session_id,omitempty"`
}

type SelectSessionResponse struct {
	SelectedSessionID string `json:"selected_session_id"`
}
