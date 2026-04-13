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

type TurnResultSink interface {
	HandleTurnResult(ctx context.Context, session types.Session, turnID string, err error)
}

type SubmitTurnInput struct {
	TurnID       string
	ClientTurnID string
	Message      string
}

type ResumeTurnInput struct {
	TurnID  string
	Message string
	Resume  *types.TurnResume
}

type RunInput struct {
	Session types.Session
	TurnID  string
	Message string
	Resume  *types.TurnResume
}

type Manager struct {
	mu             sync.Mutex
	runner         Runner
	turnResultSink TurnResultSink
	sessions       map[string]types.Session
	runtime        map[string]*RuntimeState
}

func NewManager(runner Runner, sink ...TurnResultSink) *Manager {
	var turnResultSink TurnResultSink
	if len(sink) > 0 {
		turnResultSink = sink[0]
	}
	return &Manager{
		runner:         runner,
		turnResultSink: turnResultSink,
		sessions:       make(map[string]types.Session),
		runtime:        make(map[string]*RuntimeState),
	}
}

func (m *Manager) RegisterSession(session types.Session) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.sessions[session.ID] = session
	m.runtime[session.ID] = &RuntimeState{}
}

func (m *Manager) UpdateSession(session types.Session) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.sessions[session.ID]; !ok {
		return false
	}
	m.sessions[session.ID] = session
	return true
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
	return m.startTurn(ctx, sessionID, RunInput{
		TurnID:  in.TurnID,
		Message: in.Message,
	})
}

func (m *Manager) ResumeTurn(ctx context.Context, sessionID string, in ResumeTurnInput) (string, error) {
	return m.startTurn(ctx, sessionID, RunInput{
		TurnID:  in.TurnID,
		Message: in.Message,
		Resume:  in.Resume,
	})
}

func (m *Manager) InterruptTurn(sessionID, turnID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	state, ok := m.runtime[sessionID]
	if !ok || turnID == "" {
		return false
	}
	if state.ActiveTurnID != turnID {
		return false
	}

	if state.cancel != nil {
		state.cancel()
		state.cancel = nil
	}
	state.ActiveTurnID = ""
	state.RunPermissions = nil
	return true
}

func (m *Manager) startTurn(ctx context.Context, sessionID string, in RunInput) (string, error) {
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

	if in.Resume != nil && in.Resume.DecisionScope == types.PermissionDecisionAllowSession && in.Resume.EffectivePermissionProfile != "" {
		session.PermissionProfile = in.Resume.EffectivePermissionProfile
		m.sessions[sessionID] = session
	}

	if state.cancel != nil {
		state.cancel()
	}
	if state.RunPermissions == nil {
		state.RunPermissions = make(map[string]string)
	}
	if in.Resume != nil && in.Resume.RunID != "" && in.Resume.EffectivePermissionProfile != "" {
		if in.Resume.DecisionScope == types.PermissionDecisionAllowRun || in.Resume.DecisionScope == types.PermissionDecisionAllowOnce {
			state.RunPermissions[in.Resume.RunID] = in.Resume.EffectivePermissionProfile
		}
	}

	// Background turn execution should not be tied to the lifetime of the
	// submitting HTTP request, but it should still have its own cancel handle.
	runCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	state.cancel = cancel
	state.ActiveTurnID = in.TurnID
	m.mu.Unlock()

	go func() {
		runErr := m.runner.RunTurn(runCtx, RunInput{
			Session: session,
			TurnID:  in.TurnID,
			Message: in.Message,
			Resume:  in.Resume,
		})
		if m.turnResultSink != nil {
			m.turnResultSink.HandleTurnResult(runCtx, session, in.TurnID, runErr)
		}
		m.finishTurn(sessionID, in.TurnID)
	}()

	return in.TurnID, nil
}

func (m *Manager) finishTurn(sessionID, turnID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	state, ok := m.runtime[sessionID]
	if !ok {
		return
	}
	if state.ActiveTurnID != turnID {
		return
	}

	state.ActiveTurnID = ""
	state.cancel = nil
	state.RunPermissions = nil
}
