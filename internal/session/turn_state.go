package session

import "go-agent/internal/types"

func InterruptedTurnState() types.TurnState {
	return types.TurnStateInterrupted
}
