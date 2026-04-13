package types

import "time"

type CreateSessionRequest struct {
	WorkspaceRoot string `json:"workspace_root"`
	SystemPrompt  string `json:"system_prompt"`
}

type PatchSessionRequest struct {
	SystemPrompt *string `json:"system_prompt"`
}

type SubmitTurnRequest struct {
	ClientTurnID string `json:"client_turn_id"`
	Message      string `json:"message"`
}

type PermissionDecisionRequest struct {
	RequestID string `json:"request_id"`
	Decision  string `json:"decision"`
}

type PermissionDecisionResponse struct {
	Request PermissionRequest `json:"request"`
	TurnID  string            `json:"turn_id"`
	Resumed bool              `json:"resumed"`
}

type ListPendingAutomationPermissionsResponse struct {
	Pending []PendingAutomationPermission `json:"pending"`
}

type SessionListItem struct {
	ID            string       `json:"id"`
	Title         string       `json:"title,omitempty"`
	LastPreview   string       `json:"last_preview,omitempty"`
	WorkspaceRoot string       `json:"workspace_root"`
	SystemPrompt  string       `json:"system_prompt,omitempty"`
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

type DeleteSessionResponse struct {
	DeletedSessionID  string `json:"deleted_session_id"`
	SelectedSessionID string `json:"selected_session_id,omitempty"`
}
