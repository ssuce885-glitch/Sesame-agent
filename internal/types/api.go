package types

type EnsureSessionRequest struct {
	WorkspaceRoot string `json:"workspace_root"`
	SessionRole   string `json:"session_role,omitempty"`
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

type ListContextHistoryResponse struct {
	Entries       []HistoryEntry `json:"entries"`
	CurrentHeadID string         `json:"current_head_id,omitempty"`
}
