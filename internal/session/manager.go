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
	Turn types.Turn
}

type ResumeTurnInput struct {
	Turn   types.Turn
	Resume *types.TurnResume
}

type RunInput struct {
	Session types.Session
	Turn    types.Turn
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

func (m *Manager) QueuePayload(sessionID string) (types.SessionQueuePayload, bool) {
	state, ok := m.GetRuntimeState(sessionID)
	if !ok {
		return types.SessionQueuePayload{}, false
	}
	return types.SessionQueuePayload{
		ActiveTurnID:             state.ActiveTurnID,
		ActiveTurnKind:           state.ActiveTurnKind,
		QueueDepth:               state.QueueDepth,
		QueuedUserTurns:          state.QueuedUserTurns,
		QueuedChildReportBatches: state.QueuedChildReportBatches,
	}, true
}

func (m *Manager) SubmitTurn(ctx context.Context, sessionID string, in SubmitTurnInput) (string, error) {
	return m.startTurn(ctx, sessionID, RunInput{
		Turn: in.Turn,
	})
}

func (m *Manager) ResumeTurn(ctx context.Context, sessionID string, in ResumeTurnInput) (string, error) {
	return m.startTurn(ctx, sessionID, RunInput{
		Turn:   in.Turn,
		Resume: in.Resume,
	})
}

func (m *Manager) InterruptTurn(sessionID, turnID string) bool {
	m.mu.Lock()
	state, ok := m.runtime[sessionID]
	if !ok || turnID == "" {
		m.mu.Unlock()
		return false
	}
	if state.ActiveTurnID != turnID {
		m.mu.Unlock()
		return false
	}

	if state.cancel != nil {
		state.cancel()
	}
	m.clearActiveTurnLocked(state)

	next, runCtx, ok := m.dequeueAndActivateNextLocked(state)
	m.mu.Unlock()

	if ok {
		m.runTurn(sessionID, runCtx, next)
	}
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

	in.Session = session
	in.Turn.Kind = normalizeTurnKind(in.Turn.Kind)

	if in.Resume != nil && in.Resume.DecisionScope == types.PermissionDecisionAllowSession && in.Resume.EffectivePermissionProfile != "" {
		session.PermissionProfile = in.Resume.EffectivePermissionProfile
		m.sessions[sessionID] = session
		in.Session = session
	}

	if state.ActiveTurnID != "" {
		if state.ActiveTurnKind == types.TurnKindChildReportBatch && in.Turn.Kind == types.TurnKindUserMessage {
			if state.cancel != nil {
				state.cancel()
			}
			m.clearActiveTurnLocked(state)

			runCtx := m.activateTurnLocked(state, in, ctx)
			m.mu.Unlock()
			m.runTurn(sessionID, runCtx, in)
			return in.Turn.ID, nil
		}

		item := queuedTurn{
			ctx:  ctx,
			in:   in,
			kind: queueItemKindForTurn(in.Turn),
		}
		var turnID string
		state.queue, turnID, _ = enqueueQueuedTurn(state.queue, item)
		m.refreshQueueCountersLocked(state)
		m.mu.Unlock()
		return turnID, nil
	}

	runCtx := m.activateTurnLocked(state, in, ctx)
	m.mu.Unlock()
	m.runTurn(sessionID, runCtx, in)

	return in.Turn.ID, nil
}

func (m *Manager) finishTurn(sessionID, turnID string) {
	m.mu.Lock()
	state, ok := m.runtime[sessionID]
	if !ok {
		m.mu.Unlock()
		return
	}
	if state.ActiveTurnID != turnID {
		m.mu.Unlock()
		return
	}

	m.clearActiveTurnLocked(state)
	next, runCtx, ok := m.dequeueAndActivateNextLocked(state)
	m.mu.Unlock()

	if ok {
		m.runTurn(sessionID, runCtx, next)
	}
}

func (m *Manager) runTurn(sessionID string, runCtx context.Context, in RunInput) {
	go func() {
		runErr := m.runner.RunTurn(runCtx, in)
		if m.turnResultSink != nil {
			m.turnResultSink.HandleTurnResult(context.WithoutCancel(runCtx), in.Session, in.Turn.ID, runErr)
		}
		m.finishTurn(sessionID, in.Turn.ID)
	}()
}

func (m *Manager) activateTurnLocked(state *RuntimeState, in RunInput, submitCtx context.Context) context.Context {
	in.Turn.Kind = normalizeTurnKind(in.Turn.Kind)

	runCtx, cancel := context.WithCancel(context.WithoutCancel(submitCtx))
	state.cancel = cancel
	state.ActiveTurnID = in.Turn.ID
	state.ActiveTurnKind = in.Turn.Kind
	state.RunPermissions = nil
	if in.Resume != nil && in.Resume.RunID != "" && in.Resume.EffectivePermissionProfile != "" {
		if in.Resume.DecisionScope == types.PermissionDecisionAllowRun || in.Resume.DecisionScope == types.PermissionDecisionAllowOnce {
			state.RunPermissions = map[string]string{in.Resume.RunID: in.Resume.EffectivePermissionProfile}
		}
	}
	m.refreshQueueCountersLocked(state)
	return runCtx
}

func (m *Manager) dequeueAndActivateNextLocked(state *RuntimeState) (RunInput, context.Context, bool) {
	next, remaining, ok := dequeueQueuedTurn(state.queue)
	if !ok {
		m.refreshQueueCountersLocked(state)
		return RunInput{}, nil, false
	}
	state.queue = remaining
	if refreshed, exists := m.sessions[next.in.Session.ID]; exists {
		next.in.Session = refreshed
	}
	runCtx := m.activateTurnLocked(state, next.in, next.ctx)
	return next.in, runCtx, true
}

func (m *Manager) clearActiveTurnLocked(state *RuntimeState) {
	state.ActiveTurnID = ""
	state.ActiveTurnKind = ""
	state.cancel = nil
	state.RunPermissions = nil
}

func (m *Manager) refreshQueueCountersLocked(state *RuntimeState) {
	state.QueueDepth, state.QueuedUserTurns, state.QueuedChildReportBatches = queueCounters(state.queue)
}
