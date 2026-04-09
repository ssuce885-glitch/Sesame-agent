package session

import "context"

type RuntimeState struct {
	ActiveTurnID      string
	RunPermissions    map[string]string
	cancel            context.CancelFunc
}
