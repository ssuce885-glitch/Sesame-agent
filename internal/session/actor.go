package session

import "go-agent/internal/types"

type Actor struct {
	SessionID     string
	WorkspaceRoot string
	CurrentTurnID string
	LastTurnState types.TurnState
}

func NewActor(session types.Session) *Actor {
	return &Actor{
		SessionID:     session.ID,
		WorkspaceRoot: session.WorkspaceRoot,
		LastTurnState: types.TurnStateCreated,
	}
}
