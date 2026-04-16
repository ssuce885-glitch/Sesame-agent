package session

import (
	"context"

	"go-agent/internal/types"
)

type RuntimeState struct {
	ActiveTurnID             string
	ActiveTurnKind           types.TurnKind
	QueueDepth               int
	QueuedUserTurns          int
	QueuedChildReportBatches int
	RunPermissions           map[string]string

	queue  []queuedTurn
	cancel context.CancelFunc
}
