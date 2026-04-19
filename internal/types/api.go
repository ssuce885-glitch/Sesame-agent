package types

type EnsureSessionRequest struct {
	WorkspaceRoot    string `json:"workspace_root"`
	SessionRole      string `json:"session_role,omitempty"`
	SpecialistRoleID string `json:"specialist_role_id,omitempty"`
}

type LoadContextHistoryRequest struct {
	HeadID string `json:"head_id"`
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

type ListContextHistoryResponse struct {
	Entries       []HistoryEntry `json:"entries"`
	CurrentHeadID string         `json:"current_head_id,omitempty"`
}
