package session

import (
	"context"
	"errors"
	"sync"

	"go-agent/internal/types"
)

var errSessionNotFound = errors.New("session not found")

type Runner interface {
	RunTurn(ctx context.Context, in RunInput) error
}

type SubmitTurnInput struct {
	TurnID       string
	ClientTurnID string
	Message      string
}

type RunInput struct {
	Session types.Session
	TurnID  string
	Message string
}

type Manager struct {
	mu       sync.Mutex
	runner   Runner
	sessions map[string]types.Session
	runtime  map[string]*RuntimeState
}

func NewManager(runner Runner) *Manager {
	return &Manager{
		runner:   runner,
		sessions: make(map[string]types.Session),
		runtime:  make(map[string]*RuntimeState),
	}
}

func (m *Manager) RegisterSession(session types.Session) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.sessions[session.ID] = session
	m.runtime[session.ID] = &RuntimeState{}
}

func (m *Manager) GetRuntimeState(sessionID string) (RuntimeState, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	state, ok := m.runtime[sessionID]
	if !ok {
		return RuntimeState{}, false
	}

	return *state, true
}

func (m *Manager) SubmitTurn(ctx context.Context, sessionID string, in SubmitTurnInput) (string, error) {
	m.mu.Lock()
	session, ok := m.sessions[sessionID]
	if !ok {
		m.mu.Unlock()
		return "", errSessionNotFound
	}

	state, ok := m.runtime[sessionID]
	if !ok {
		m.mu.Unlock()
		return "", errSessionNotFound
	}

	if state.cancel != nil {
		state.cancel()
	}

	runCtx, cancel := context.WithCancel(ctx)
	state.cancel = cancel
	state.ActiveTurnID = in.TurnID
	m.mu.Unlock()

	go func() {
		_ = m.runner.RunTurn(runCtx, RunInput{
			Session: session,
			TurnID:  in.TurnID,
			Message: in.Message,
		})
	}()

	return in.TurnID, nil
}
