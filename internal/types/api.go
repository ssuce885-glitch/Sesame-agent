package types

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
