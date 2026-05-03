package session

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"go-agent/internal/v2/contracts"
)

var errSessionNotFound = errors.New("session not found")

type Manager struct {
	mu       sync.Mutex
	agent    contracts.Agent
	sessions map[string]contracts.Session
	runtime  map[string]*RuntimeState
}

type RuntimeState struct {
	ActiveTurnID        string
	ActiveTurnKind      string
	QueueDepth          int
	QueuedUserTurns     int
	QueuedReportBatches int

	queue  []queuedTurn
	cancel context.CancelFunc
}

type queuedTurn struct {
	ctx   context.Context
	input contracts.TurnInput
	turn  contracts.Turn
}

var _ contracts.SessionManager = (*Manager)(nil)

func NewManager(agent contracts.Agent) *Manager {
	return &Manager{
		agent:    agent,
		sessions: make(map[string]contracts.Session),
		runtime:  make(map[string]*RuntimeState),
	}
}

func (m *Manager) Register(session contracts.Session) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.sessions[session.ID] = session
	if _, ok := m.runtime[session.ID]; !ok {
		m.runtime[session.ID] = &RuntimeState{}
	}
}

func (m *Manager) SubmitTurn(ctx context.Context, sessionID string, input contracts.SubmitTurnInput) (string, error) {
	turn := input.Turn
	turn.Kind = normalizeTurnKind(turn.Kind)
	if strings.TrimSpace(turn.ID) == "" {
		turn.ID = newLocalID("turn")
	}
	if turn.CreatedAt.IsZero() {
		turn.CreatedAt = time.Now().UTC()
	}
	if turn.UpdatedAt.IsZero() {
		turn.UpdatedAt = turn.CreatedAt
	}

	runInput := contracts.TurnInput{
		SessionID: sessionID,
		TurnID:    turn.ID,
		TaskID:    input.TaskID,
		Messages:  turnMessages(turn),
		RoleSpec:  input.RoleSpec,
	}
	return m.startTurn(ctx, sessionID, turn, runInput)
}

func (m *Manager) CancelTurn(sessionID, turnID string) bool {
	m.mu.Lock()
	state, ok := m.runtime[sessionID]
	if !ok || strings.TrimSpace(turnID) == "" {
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
		if queued.input.TurnID != turnID {
			continue
		}
		state.queue = append(state.queue[:i], state.queue[i+1:]...)
		m.refreshQueueCountersLocked(state)
		m.mu.Unlock()
		return true
	}
	m.mu.Unlock()
	return false
}

func (m *Manager) QueuePayload(sessionID string) (contracts.QueuePayload, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	state, ok := m.runtime[sessionID]
	if !ok {
		return contracts.QueuePayload{}, false
	}
	return contracts.QueuePayload{
		ActiveTurnID:        state.ActiveTurnID,
		ActiveTurnKind:      state.ActiveTurnKind,
		QueueDepth:          state.QueueDepth,
		QueuedUserTurns:     state.QueuedUserTurns,
		QueuedReportBatches: state.QueuedReportBatches,
	}, true
}

func (m *Manager) startTurn(ctx context.Context, sessionID string, turn contracts.Turn, input contracts.TurnInput) (string, error) {
	m.mu.Lock()
	if _, ok := m.sessions[sessionID]; !ok {
		m.mu.Unlock()
		return "", errSessionNotFound
	}
	state, ok := m.runtime[sessionID]
	if !ok {
		m.mu.Unlock()
		return "", errSessionNotFound
	}

	if state.ActiveTurnID != "" {
		if state.ActiveTurnKind == "report_batch" && turn.Kind == "user_message" {
			if state.cancel != nil {
				state.cancel()
			}
			m.clearActiveTurnLocked(state)
			runCtx := m.activateTurnLocked(state, turn, ctx)
			m.mu.Unlock()
			m.runTurn(sessionID, runCtx, queuedTurn{ctx: ctx, input: input, turn: turn})
			return turn.ID, nil
		}

		item := queuedTurn{ctx: ctx, input: input, turn: turn}
		var turnID string
		state.queue, turnID, _ = enqueueQueuedTurn(state.queue, item)
		m.refreshQueueCountersLocked(state)
		m.mu.Unlock()
		return turnID, nil
	}

	runCtx := m.activateTurnLocked(state, turn, ctx)
	m.mu.Unlock()
	m.runTurn(sessionID, runCtx, queuedTurn{ctx: ctx, input: input, turn: turn})
	return turn.ID, nil
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

func (m *Manager) runTurn(sessionID string, runCtx context.Context, queued queuedTurn) {
	go func() {
		_ = m.agent.RunTurn(runCtx, queued.input)
		m.finishTurn(sessionID, queued.input.TurnID)
	}()
}

func (m *Manager) activateTurnLocked(state *RuntimeState, turn contracts.Turn, submitCtx context.Context) context.Context {
	runCtx, cancel := context.WithCancel(context.WithoutCancel(submitCtx))
	state.cancel = cancel
	state.ActiveTurnID = turn.ID
	state.ActiveTurnKind = normalizeTurnKind(turn.Kind)
	m.refreshQueueCountersLocked(state)
	return runCtx
}

func (m *Manager) dequeueAndActivateNextLocked(state *RuntimeState) (queuedTurn, context.Context, bool) {
	next, remaining, ok := dequeueQueuedTurn(state.queue)
	if !ok {
		m.refreshQueueCountersLocked(state)
		return queuedTurn{}, nil, false
	}
	state.queue = remaining
	runCtx := m.activateTurnLocked(state, next.turn, next.ctx)
	return next, runCtx, true
}

func (m *Manager) clearActiveTurnLocked(state *RuntimeState) {
	state.ActiveTurnID = ""
	state.ActiveTurnKind = ""
	state.cancel = nil
	m.refreshQueueCountersLocked(state)
}

func (m *Manager) refreshQueueCountersLocked(state *RuntimeState) {
	state.QueueDepth, state.QueuedUserTurns, state.QueuedReportBatches = queueCounters(state.queue)
}

func normalizeTurnKind(kind string) string {
	if kind == "report_batch" {
		return "report_batch"
	}
	return "user_message"
}

func turnMessages(turn contracts.Turn) []contracts.Message {
	if strings.TrimSpace(turn.UserMessage) == "" {
		return nil
	}
	return []contracts.Message{{
		SessionID: turn.SessionID,
		TurnID:    turn.ID,
		Role:      "user",
		Content:   turn.UserMessage,
		CreatedAt: time.Now().UTC(),
	}}
}

func enqueueQueuedTurn(queue []queuedTurn, item queuedTurn) ([]queuedTurn, string, bool) {
	if normalizeTurnKind(item.turn.Kind) == "report_batch" {
		for _, queued := range queue {
			if normalizeTurnKind(queued.turn.Kind) == "report_batch" {
				return queue, queued.turn.ID, false
			}
		}
	}
	return append(queue, item), item.turn.ID, true
}

func dequeueQueuedTurn(queue []queuedTurn) (queuedTurn, []queuedTurn, bool) {
	if len(queue) == 0 {
		return queuedTurn{}, queue, false
	}
	return queue[0], queue[1:], true
}

func queueCounters(queue []queuedTurn) (depth, queuedUsers, queuedReportBatches int) {
	depth = len(queue)
	for _, item := range queue {
		if normalizeTurnKind(item.turn.Kind) == "report_batch" {
			queuedReportBatches++
		} else {
			queuedUsers++
		}
	}
	return depth, queuedUsers, queuedReportBatches
}

func newLocalID(prefix string) string {
	return prefix + "-" + strings.ReplaceAll(time.Now().UTC().Format("20060102150405.000000000"), ".", "")
}
