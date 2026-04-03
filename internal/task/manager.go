package task

import "go-agent/internal/types"

type Manager struct{}

func NewManager() *Manager {
	return &Manager{}
}

func (m *Manager) Create(parentSessionID, workspaceRoot string) Session {
	return Session{
		ID:              types.NewID("task"),
		ParentSessionID: parentSessionID,
		WorkspaceRoot:   workspaceRoot,
	}
}
