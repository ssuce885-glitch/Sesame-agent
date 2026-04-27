package session

import (
	"context"

	"go-agent/internal/types"
)

type RuntimeState struct {
	ActiveTurnID        string
	ActiveTurnKind      types.TurnKind
	QueueDepth          int
	QueuedUserTurns     int
	QueuedReportBatches int

	queue  []queuedTurn
	cancel context.CancelFunc
}
