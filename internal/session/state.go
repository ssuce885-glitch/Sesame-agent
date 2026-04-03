package session

import "context"

type RuntimeState struct {
	ActiveTurnID string
	cancel       context.CancelFunc
}
