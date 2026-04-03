package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go-agent/internal/session"
	"go-agent/internal/types"
)

func TestCreateSessionThenStreamEvents(t *testing.T) {
	handler := NewRouter(NewTestDependencies(t))

	createReq := httptest.NewRequest(http.MethodPost, "/v1/sessions", strings.NewReader(`{"workspace_root":"D:/work/demo"}`))
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	handler.ServeHTTP(createRec, createReq)

	if createRec.Code != http.StatusCreated {
		t.Fatalf("create session status = %d, want %d", createRec.Code, http.StatusCreated)
	}
}

type turnSubmitStore struct {
	turns        map[string]types.Turn
	insertCalled bool
	deleteCalled bool
	deletedID    string
	deleteErr    error
}

func (s *turnSubmitStore) InsertSession(context.Context, types.Session) error { return nil }

func (s *turnSubmitStore) InsertTurn(_ context.Context, turn types.Turn) error {
	s.insertCalled = true
	if s.turns == nil {
		s.turns = make(map[string]types.Turn)
	}
	s.turns[turn.ID] = turn
	return nil
}

func (s *turnSubmitStore) DeleteTurn(ctx context.Context, turnID string) error {
	s.deleteCalled = true
	s.deletedID = turnID
	delete(s.turns, turnID)
	if err := ctx.Err(); err != nil {
		return err
	}
	if s.deleteErr != nil {
		return s.deleteErr
	}
	return nil
}

func (s *turnSubmitStore) ListSessionEvents(context.Context, string, int64) ([]types.Event, error) {
	return nil, nil
}

type turnSubmitManager struct {
	sessionID     string
	input         session.SubmitTurnInput
	called        bool
	err           error
	cancelRequest func()
}

func (m *turnSubmitManager) RegisterSession(types.Session) {}

func (m *turnSubmitManager) SubmitTurn(ctx context.Context, sessionID string, in session.SubmitTurnInput) (string, error) {
	m.called = true
	m.sessionID = sessionID
	m.input = in
	if m.cancelRequest != nil {
		m.cancelRequest()
	}
	if m.err != nil {
		return "", m.err
	}
	return "turn_test_id", nil
}

func (m *turnSubmitManager) Subscribe(string) <-chan types.Event { return nil }

func TestSubmitTurnUsesSessionScopedRoute(t *testing.T) {
	store := &turnSubmitStore{}
	manager := &turnSubmitManager{}
	handler := NewRouter(Dependencies{
		Store:   store,
		Manager: manager,
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/sessions/sess_123/turns", strings.NewReader(`{"client_turn_id":"client_1","message":"hello"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status code = %d, want %d", rec.Code, http.StatusAccepted)
	}
	if !store.insertCalled {
		t.Fatal("store.InsertTurn was not called")
	}
	if len(store.turns) != 1 {
		t.Fatalf("len(turns) = %d, want 1", len(store.turns))
	}
	var turn types.Turn
	for _, got := range store.turns {
		turn = got
	}
	if turn.SessionID != "sess_123" {
		t.Fatalf("turn session ID = %q, want %q", turn.SessionID, "sess_123")
	}
	if turn.ClientTurnID != "client_1" {
		t.Fatalf("turn client_turn_id = %q, want %q", turn.ClientTurnID, "client_1")
	}
	if turn.UserMessage != "hello" {
		t.Fatalf("turn user_message = %q, want %q", turn.UserMessage, "hello")
	}
	if turn.State != types.TurnStateCreated {
		t.Fatalf("turn state = %q, want %q", turn.State, types.TurnStateCreated)
	}
	if turn.ID == "" {
		t.Fatal("turn ID is empty")
	}
	if turn.CreatedAt.IsZero() || turn.UpdatedAt.IsZero() {
		t.Fatal("turn timestamps were not set")
	}
	if manager.sessionID != "sess_123" {
		t.Fatalf("manager session ID = %q, want %q", manager.sessionID, "sess_123")
	}
	if manager.input.Message != "hello" {
		t.Fatalf("manager input message = %q, want %q", manager.input.Message, "hello")
	}

	var got types.Turn
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal response error = %v", err)
	}
	if got.SessionID != "sess_123" {
		t.Fatalf("response session ID = %q, want %q", got.SessionID, "sess_123")
	}
	if got.UserMessage != "hello" {
		t.Fatalf("response user_message = %q, want %q", got.UserMessage, "hello")
	}
	if got.State != types.TurnStateCreated {
		t.Fatalf("response state = %q, want %q", got.State, types.TurnStateCreated)
	}
}

func TestSubmitTurnRemovesPersistedTurnWhenManagerRejectsSession(t *testing.T) {
	store := &turnSubmitStore{}
	manager := &turnSubmitManager{err: errors.New("session not found")}
	handler := NewRouter(Dependencies{
		Store:   store,
		Manager: manager,
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/sessions/sess_missing/turns", strings.NewReader(`{"client_turn_id":"client_1","message":"hello"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status code = %d, want %d", rec.Code, http.StatusNotFound)
	}
	if len(store.turns) != 0 {
		t.Fatalf("len(turns) = %d, want 0 after manager rejection", len(store.turns))
	}
	if !store.deleteCalled {
		t.Fatal("store.DeleteTurn was not called")
	}
	if store.deletedID == "" {
		t.Fatal("deleted turn ID is empty")
	}
	if !store.insertCalled {
		t.Fatal("store.InsertTurn was not called")
	}
	if !manager.called {
		t.Fatal("manager.SubmitTurn was not called")
	}
}

func TestSubmitTurnDeletesPersistedTurnEvenWhenRequestContextIsCancelled(t *testing.T) {
	store := &turnSubmitStore{}
	manager := &turnSubmitManager{
		err: errors.New("session not found"),
	}
	manager.cancelRequest = func() {}
	handler := NewRouter(Dependencies{
		Store:   store,
		Manager: manager,
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/sessions/sess_missing/turns", strings.NewReader(`{"client_turn_id":"client_1","message":"hello"}`))
	req.Header.Set("Content-Type", "application/json")
	ctx, cancel := context.WithCancel(req.Context())
	req = req.WithContext(ctx)
	manager.cancelRequest = cancel

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status code = %d, want %d", rec.Code, http.StatusNotFound)
	}
	if len(store.turns) != 0 {
		t.Fatalf("len(turns) = %d, want 0 after request cancellation", len(store.turns))
	}
	if !store.deleteCalled {
		t.Fatal("store.DeleteTurn was not called")
	}
	if store.deleteErr != nil {
		t.Fatalf("deleteErr was not expected in setup: %v", store.deleteErr)
	}
}

type replayStore struct {
	listCalled bool
	afterSeq   int64
	events     []types.Event
	liveBus    *handoffBus
}

func (s *replayStore) InsertSession(context.Context, types.Session) error { return nil }

func (s *replayStore) InsertTurn(context.Context, types.Turn) error { return nil }

func (s *replayStore) DeleteTurn(context.Context, string) error { return nil }

func (s *replayStore) ListSessionEvents(ctx context.Context, sessionID string, afterSeq int64) ([]types.Event, error) {
	s.listCalled = true
	s.afterSeq = afterSeq
	if s.liveBus != nil {
		_ = s.liveBus.Publish(types.Event{
			Seq:       afterSeq + 2,
			ID:        "evt_live_8",
			SessionID: sessionID,
			Type:      types.EventAssistantDelta,
			Time:      time.Date(2026, 4, 3, 1, 2, 4, 0, time.UTC),
			Payload:   json.RawMessage(`{"text":"live"}`),
		})
	}
	return s.events, nil
}

type replayBus struct {
	store           *replayStore
	subscribeCalled bool
}

func (b *replayBus) Subscribe(sessionID string) <-chan types.Event {
	b.subscribeCalled = true
	ch := make(chan types.Event)
	close(ch)
	return ch
}

func TestEventStreamReplaysHistoryBeforeSubscribing(t *testing.T) {
	history := types.Event{
		Seq:       7,
		ID:        "evt_test_7",
		SessionID: "sess_123",
		Type:      types.EventTurnStarted,
		Time:      time.Date(2026, 4, 3, 1, 2, 3, 0, time.UTC),
		Payload:   json.RawMessage(`{"workspace_root":"D:/work/demo"}`),
	}
	store := &replayStore{events: []types.Event{history}}
	bus := &replayBus{store: store}
	handler := NewRouter(Dependencies{
		Store: store,
		Bus:   bus,
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/sessions/sess_123/events?after=6", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d", rec.Code, http.StatusOK)
	}
	if !store.listCalled {
		t.Fatal("store.ListSessionEvents was not called")
	}
	if store.afterSeq != 6 {
		t.Fatalf("after seq = %d, want %d", store.afterSeq, 6)
	}
	if !bus.subscribeCalled {
		t.Fatal("bus.Subscribe was not called")
	}
	body := rec.Body.String()
	if !strings.Contains(body, "id: 7") {
		t.Fatalf("body = %q, want SSE history event", body)
	}
	if !strings.Contains(body, "event: turn.started") {
		t.Fatalf("body = %q, want SSE event type", body)
	}
	if !strings.Contains(body, "workspace_root") {
		t.Fatalf("body = %q, want SSE payload", body)
	}
}

type handoffBus struct {
	ch         chan types.Event
	published  bool
	subscribed bool
}

func (b *handoffBus) Subscribe(sessionID string) <-chan types.Event {
	b.subscribed = true
	if b.published && b.ch == nil {
		ch := make(chan types.Event)
		close(ch)
		return ch
	}
	if b.ch == nil {
		b.ch = make(chan types.Event, 1)
	}
	return b.ch
}

func (b *handoffBus) Publish(event types.Event) error {
	b.published = true
	if b.ch == nil {
		return nil
	}
	b.ch <- event
	close(b.ch)
	return nil
}

func TestEventStreamDeliversLiveEventPublishedDuringHistoryReplay(t *testing.T) {
	bus := &handoffBus{}
	store := &replayStore{
		events: []types.Event{
			{
				Seq:       7,
				ID:        "evt_hist_7",
				SessionID: "sess_123",
				Type:      types.EventTurnStarted,
				Time:      time.Date(2026, 4, 3, 1, 2, 3, 0, time.UTC),
				Payload:   json.RawMessage(`{"workspace_root":"D:/work/demo"}`),
			},
		},
		liveBus: bus,
	}
	handler := NewRouter(Dependencies{
		Store: store,
		Bus:   bus,
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/sessions/sess_123/events?after=6", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d", rec.Code, http.StatusOK)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "id: 7") {
		t.Fatalf("body = %q, want replayed history event", body)
	}
	if !strings.Contains(body, "id: 8") {
		t.Fatalf("body = %q, want live event published during replay", body)
	}
	if !strings.Contains(body, "event: assistant.delta") {
		t.Fatalf("body = %q, want live SSE event type", body)
	}
}
