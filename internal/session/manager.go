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
	Run  RunMetadata
}

type RunMetadata struct {
	TaskID                  string
	TaskObserver            TaskObserver
	ActivatedSkillNames     []string
	CancelWithSubmitContext bool
	Done                    chan error
}

type RunInput struct {
	Session                 types.Session
	Turn                    types.Turn
	TaskID                  string
	TaskObserver            TaskObserver
	ActivatedSkillNames     []string
	CancelWithSubmitContext bool
	Done                    chan error
}

type TaskObserver interface {
	AppendLog([]byte) error
	SetFinalText(string) error
	SetOutcome(types.ChildAgentOutcome, string) error
	SetRunContext(string, string) error
}

type Manager struct {
	mu             sync.Mutex
	runner         Runner
	turnResultSink TurnResultSink
	idleNotifier   func(string)
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

func (m *Manager) SetIdleNotifier(notifier func(string)) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.idleNotifier = notifier
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
		ActiveTurnID:        state.ActiveTurnID,
		ActiveTurnKind:      state.ActiveTurnKind,
		QueueDepth:          state.QueueDepth,
		QueuedUserTurns:     state.QueuedUserTurns,
		QueuedReportBatches: state.QueuedReportBatches,
	}, true
}

func (m *Manager) SubmitTurn(ctx context.Context, sessionID string, in SubmitTurnInput) (string, error) {
	return m.startTurn(ctx, sessionID, RunInput{
		Turn:                    in.Turn,
		TaskID:                  in.Run.TaskID,
		TaskObserver:            in.Run.TaskObserver,
		ActivatedSkillNames:     append([]string(nil), in.Run.ActivatedSkillNames...),
		CancelWithSubmitContext: in.Run.CancelWithSubmitContext,
		Done:                    in.Run.Done,
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

func (m *Manager) CancelTurn(sessionID, turnID string) bool {
	m.mu.Lock()
	state, ok := m.runtime[sessionID]
	if !ok || turnID == "" {
		m.mu.Unlock()
		return false
	}
	if state.ActiveTurnID == turnID {
		if state.cancel != nil {
			state.cancel()
		}
		m.clearActiveTurnLocked(state)

		next, runCtx, hasNext := m.dequeueAndActivateNextLocked(state)
		m.mu.Unlock()

		if hasNext {
			m.runTurn(sessionID, runCtx, next)
		}
		return true
	}

	for i, queued := range state.queue {
		if queued.in.Turn.ID != turnID {
			continue
		}
		done := queued.in.Done
		state.queue = append(state.queue[:i], state.queue[i+1:]...)
		m.refreshQueueCountersLocked(state)
		m.mu.Unlock()
		if done != nil {
			done <- context.Canceled
		}
		return true
	}
	m.mu.Unlock()
	return false
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

	if state.ActiveTurnID != "" {
		if state.ActiveTurnKind == types.TurnKindReportBatch && in.Turn.Kind == types.TurnKindUserMessage {
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
	idleNotifier := m.idleNotifier
	m.mu.Unlock()

	if ok {
		m.runTurn(sessionID, runCtx, next)
		return
	}
	if idleNotifier != nil {
		idleNotifier(sessionID)
	}
}

func (m *Manager) runTurn(sessionID string, runCtx context.Context, in RunInput) {
	go func() {
		runErr := m.runner.RunTurn(runCtx, in)
		if m.turnResultSink != nil {
			m.turnResultSink.HandleTurnResult(context.WithoutCancel(runCtx), in.Session, in.Turn.ID, runErr)
		}
		if in.Done != nil {
			in.Done <- runErr
		}
		m.finishTurn(sessionID, in.Turn.ID)
	}()
}

func (m *Manager) activateTurnLocked(state *RuntimeState, in RunInput, submitCtx context.Context) context.Context {
	in.Turn.Kind = normalizeTurnKind(in.Turn.Kind)

	baseCtx := context.WithoutCancel(submitCtx)
	if in.CancelWithSubmitContext {
		baseCtx = submitCtx
	}
	runCtx, cancel := context.WithCancel(baseCtx)
	state.cancel = cancel
	state.ActiveTurnID = in.Turn.ID
	state.ActiveTurnKind = in.Turn.Kind
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
}

func (m *Manager) refreshQueueCountersLocked(state *RuntimeState) {
	state.QueueDepth, state.QueuedUserTurns, state.QueuedReportBatches = queueCounters(state.queue)
}
