package permissions

type PendingRequest struct {
	ID        string
	SessionID string
	TurnID    string
	ToolName  string
}

func NewPendingRequest(id, sessionID, turnID, toolName string) PendingRequest {
	return PendingRequest{ID: id, SessionID: sessionID, TurnID: turnID, ToolName: toolName}
}
