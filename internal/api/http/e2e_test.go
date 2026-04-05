package httpapi

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	contextstate "go-agent/internal/context"
	"go-agent/internal/engine"
	"go-agent/internal/model"
	"go-agent/internal/permissions"
	"go-agent/internal/session"
	"go-agent/internal/store/sqlite"
	"go-agent/internal/stream"
	"go-agent/internal/tools"
	"go-agent/internal/types"
)

func TestCreateSessionThenStreamEvents(t *testing.T) {
	handler := NewRouter(NewTestDependencies(t))

	createReq := httptest.NewRequest(http.MethodPost, "/v1/sessions", strings.NewReader(`{"workspace_root":"D:/work/demo","system_prompt":"focus on greetings"}`))
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	handler.ServeHTTP(createRec, createReq)

	if createRec.Code != http.StatusCreated {
		t.Fatalf("create session status = %d, want %d", createRec.Code, http.StatusCreated)
	}
}

type e2eStoreAndBusSink struct {
	store *sqlite.Store
	bus   interface {
		Publish(types.Event)
	}
}

func (s e2eStoreAndBusSink) Emit(ctx context.Context, event types.Event) error {
	seq, err := s.store.AppendEvent(ctx, event)
	if err != nil {
		return err
	}
	event.Seq = seq
	s.bus.Publish(event)
	return nil
}

type signalingBus struct {
	inner      *stream.Bus
	subscribed chan struct{}
	once       sync.Once
}

func newSignalingBus() *signalingBus {
	return &signalingBus{
		inner:      stream.NewBus(),
		subscribed: make(chan struct{}),
	}
}

func (b *signalingBus) Publish(event types.Event) {
	b.inner.Publish(event)
}

func (b *signalingBus) Subscribe(sessionID string) (<-chan types.Event, func()) {
	ch, unsub := b.inner.Subscribe(sessionID)
	b.once.Do(func() {
		close(b.subscribed)
	})
	return ch, unsub
}

type e2eSessionRunner struct {
	engine *engine.Engine
	sink   engine.EventSink
}

func (r e2eSessionRunner) RunTurn(ctx context.Context, in session.RunInput) error {
	return r.engine.RunTurn(ctx, engine.Input{
		Session: in.Session,
		Turn: types.Turn{
			ID:          in.TurnID,
			SessionID:   in.Session.ID,
			UserMessage: in.Message,
		},
		Sink: r.sink,
	})
}

func readSSEUntil(body io.ReadCloser, needle string, cancel context.CancelFunc) (string, error) {
	defer body.Close()

	got := make(chan string, 1)
	errs := make(chan error, 1)

	go func() {
		reader := bufio.NewReader(body)
		var builder strings.Builder
		for {
			line, err := reader.ReadString('\n')
			if line != "" {
				builder.WriteString(line)
				if strings.Contains(builder.String(), needle) {
					got <- builder.String()
					return
				}
			}
			if err != nil {
				errs <- err
				return
			}
		}
	}()

	select {
	case output := <-got:
		cancel()
		return output, nil
	case err := <-errs:
		cancel()
		return "", err
	case <-time.After(2 * time.Second):
		cancel()
		return "", fmt.Errorf("timed out waiting for %q in SSE stream", needle)
	}
}

func TestCreateSessionSubmitTurnAndStreamEvents(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "agentd.db")
	store, err := sqlite.Open(dbPath)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	bus := newSignalingBus()
	modelClient := model.NewFakeStreaming([][]model.StreamEvent{
		{
			{Kind: model.StreamEventTextDelta, TextDelta: "hello"},
			{Kind: model.StreamEventMessageEnd},
		},
	})
	runner := engine.New(
		modelClient,
		tools.NewRegistry(),
		permissions.NewEngine(),
		store,
		contextstate.NewManager(contextstate.Config{
			MaxRecentItems:      8,
			MaxEstimatedTokens:  6000,
			CompactionThreshold: 16,
		}),
		nil,
		8,
	)
	manager := session.NewManager(e2eSessionRunner{
		engine: runner,
		sink: e2eStoreAndBusSink{
			store: store,
			bus:   bus,
		},
	})

	server := httptest.NewServer(NewRouter(Dependencies{
		Store:   store,
		Bus:     bus,
		Manager: manager,
	}))
	defer server.Close()

	createReq, err := http.NewRequest(http.MethodPost, server.URL+"/v1/sessions", strings.NewReader(`{"workspace_root":"D:/work/demo","system_prompt":"focus on greetings"}`))
	if err != nil {
		t.Fatalf("NewRequest(create) error = %v", err)
	}
	createReq.Header.Set("Content-Type", "application/json")

	createResp, err := http.DefaultClient.Do(createReq)
	if err != nil {
		t.Fatalf("Do(create) error = %v", err)
	}
	defer createResp.Body.Close()

	if createResp.StatusCode != http.StatusCreated {
		t.Fatalf("create session status = %d, want %d", createResp.StatusCode, http.StatusCreated)
	}

	var created types.Session
	if err := json.NewDecoder(createResp.Body).Decode(&created); err != nil {
		t.Fatalf("Decode(create session) error = %v", err)
	}
	if created.ID == "" {
		t.Fatal("created session ID is empty")
	}
	if created.SystemPrompt != "focus on greetings" {
		t.Fatalf("created.SystemPrompt = %q, want %q", created.SystemPrompt, "focus on greetings")
	}

	streamCtx, cancel := context.WithCancel(context.Background())
	streamBody := make(chan string, 1)
	streamErrs := make(chan error, 1)
	go func() {
		streamReq, err := http.NewRequestWithContext(streamCtx, http.MethodGet, server.URL+"/v1/sessions/"+created.ID+"/events?after=0", nil)
		if err != nil {
			streamErrs <- fmt.Errorf("NewRequest(stream): %w", err)
			return
		}

		streamResp, err := http.DefaultClient.Do(streamReq)
		if err != nil {
			streamErrs <- fmt.Errorf("Do(stream): %w", err)
			return
		}
		if streamResp.StatusCode != http.StatusOK {
			defer streamResp.Body.Close()
			streamErrs <- fmt.Errorf("stream events status = %d, want %d", streamResp.StatusCode, http.StatusOK)
			return
		}

		body, err := readSSEUntil(streamResp.Body, "event: turn.completed", cancel)
		if err != nil {
			streamErrs <- err
			return
		}
		streamBody <- body
	}()

	select {
	case <-bus.subscribed:
	case err := <-streamErrs:
		t.Fatal(err)
	case <-time.After(2 * time.Second):
		cancel()
		t.Fatal("timed out waiting for SSE subscription")
	}

	submitReq, err := http.NewRequest(http.MethodPost, server.URL+"/v1/sessions/"+created.ID+"/turns", strings.NewReader(`{"client_turn_id":"turn-1","message":"say hello"}`))
	if err != nil {
		t.Fatalf("NewRequest(submit) error = %v", err)
	}
	submitReq.Header.Set("Content-Type", "application/json")

	submitResp, err := http.DefaultClient.Do(submitReq)
	if err != nil {
		t.Fatalf("Do(submit) error = %v", err)
	}
	defer submitResp.Body.Close()

	if submitResp.StatusCode != http.StatusAccepted {
		t.Fatalf("submit turn status = %d, want %d", submitResp.StatusCode, http.StatusAccepted)
	}

	var submitted types.Turn
	if err := json.NewDecoder(submitResp.Body).Decode(&submitted); err != nil {
		t.Fatalf("Decode(submit turn) error = %v", err)
	}
	if submitted.ID == "" {
		t.Fatal("submitted turn ID is empty")
	}

	var body string
	select {
	case body = <-streamBody:
	case err := <-streamErrs:
		t.Fatal(err)
	case <-time.After(2 * time.Second):
		cancel()
		t.Fatal("timed out waiting for SSE body")
	}

	if !strings.Contains(body, "event: turn.started") {
		t.Fatalf("SSE body = %q, want turn.started", body)
	}
	if !strings.Contains(body, "event: assistant.delta") {
		t.Fatalf("SSE body = %q, want assistant.delta", body)
	}
	if !strings.Contains(body, "event: assistant.completed") {
		t.Fatalf("SSE body = %q, want assistant.completed", body)
	}
	if !strings.Contains(body, "event: turn.completed") {
		t.Fatalf("SSE body = %q, want turn.completed", body)
	}
	if !strings.Contains(body, "\"text\":\"hello\"") {
		t.Fatalf("SSE body = %q, want assistant text payload", body)
	}
}

func TestResponsesProviderToolCallFlowOverHTTP(t *testing.T) {
	server, baseURL := startResponsesProviderStub(t, [][]string{
		{
			"event: response.output_item.added\ndata: {\"item\":{\"id\":\"tool_item_1\",\"type\":\"function_call\",\"call_id\":\"tool_1\",\"name\":\"glob\"}}\n\n",
			"event: response.function_call_arguments.done\ndata: {\"item_id\":\"tool_item_1\",\"name\":\"glob\",\"arguments\":\"{\\\"pattern\\\":\\\"*.go\\\"}\"}\n\n",
			"event: response.completed\ndata: {\"status\":\"completed\"}\n\n",
		},
		{
			"event: response.output_text.delta\ndata: {\"delta\":\"Found files\"}\n\n",
			"event: response.completed\ndata: {\"status\":\"completed\"}\n\n",
		},
	})
	defer server.Close()

	daemon := newHTTPRuntimeForTest(t, baseURL)
	sessionID := createSession(t, daemon.URL, t.TempDir())
	body := subscribeAndSubmit(t, daemon.URL, sessionID, "list Go files")

	if !strings.Contains(body, "event: tool.completed") {
		t.Fatalf("body = %q, want tool.completed", body)
	}
	if !strings.Contains(body, "Found files") {
		t.Fatalf("body = %q, want final assistant text", body)
	}
}

func startResponsesProviderStub(t *testing.T, responses [][]string) (*httptest.Server, string) {
	t.Helper()

	var mu sync.Mutex
	requestIndex := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/responses" {
			t.Fatalf("path = %s, want /v1/responses", r.URL.Path)
		}

		mu.Lock()
		if requestIndex >= len(responses) {
			mu.Unlock()
			t.Fatalf("unexpected extra responses request %d", requestIndex+1)
		}
		frames := responses[requestIndex]
		requestIndex++
		mu.Unlock()

		w.Header().Set("Content-Type", "text/event-stream")
		for _, frame := range frames {
			if _, err := io.WriteString(w, frame); err != nil {
				t.Fatalf("WriteString() error = %v", err)
			}
		}
	}))

	return server, server.URL
}

func newHTTPRuntimeForTest(t *testing.T, baseURL string) *httptest.Server {
	t.Helper()

	store, err := sqlite.Open(filepath.Join(t.TempDir(), "agentd.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	bus := stream.NewBus()
	provider, err := model.NewOpenAICompatibleProvider(model.Config{
		APIKey:  "test-key",
		Model:   "provider-model",
		BaseURL: baseURL,
	})
	if err != nil {
		t.Fatalf("NewOpenAICompatibleProvider() error = %v", err)
	}

	ctxManager := contextstate.NewManager(contextstate.Config{
		MaxRecentItems:      8,
		MaxEstimatedTokens:  6000,
		CompactionThreshold: 16,
	})
	runner := engine.New(provider, tools.NewRegistry(), permissions.NewEngine("trusted_local"), store, ctxManager, nil, 8)
	manager := session.NewManager(e2eSessionRunner{
		engine: runner,
		sink: e2eStoreAndBusSink{
			store: store,
			bus:   bus,
		},
	})

	server := httptest.NewServer(NewRouter(Dependencies{
		Store:   store,
		Bus:     bus,
		Manager: manager,
		Status: StatusPayload{
			Provider:          "openai_compatible",
			Model:             "provider-model",
			PermissionProfile: "trusted_local",
		},
	}))
	t.Cleanup(server.Close)
	return server
}

func createSession(t *testing.T, baseURL string, workspaceRoot string) string {
	t.Helper()

	req, err := http.NewRequest(http.MethodPost, baseURL+"/v1/sessions", strings.NewReader(fmt.Sprintf(`{"workspace_root":%q}`, workspaceRoot)))
	if err != nil {
		t.Fatalf("NewRequest(create) error = %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do(create) error = %v", err)
	}
	defer resp.Body.Close()

	var created types.Session
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatalf("Decode(create) error = %v", err)
	}
	return created.ID
}

func subscribeAndSubmit(t *testing.T, baseURL string, sessionID string, message string) string {
	t.Helper()

	streamCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	streamReq, err := http.NewRequestWithContext(streamCtx, http.MethodGet, baseURL+"/v1/sessions/"+sessionID+"/events?after=0", nil)
	if err != nil {
		t.Fatalf("NewRequest(stream) error = %v", err)
	}
	streamResp, err := http.DefaultClient.Do(streamReq)
	if err != nil {
		t.Fatalf("Do(stream) error = %v", err)
	}

	bodyCh := make(chan string, 1)
	errCh := make(chan error, 1)
	go func() {
		body, err := readSSEUntil(streamResp.Body, "event: turn.completed", cancel)
		if err != nil {
			errCh <- err
			return
		}
		bodyCh <- body
	}()

	submitReq, err := http.NewRequest(http.MethodPost, baseURL+"/v1/sessions/"+sessionID+"/turns", strings.NewReader(fmt.Sprintf(`{"client_turn_id":"turn-1","message":%q}`, message)))
	if err != nil {
		t.Fatalf("NewRequest(submit) error = %v", err)
	}
	submitReq.Header.Set("Content-Type", "application/json")
	submitResp, err := http.DefaultClient.Do(submitReq)
	if err != nil {
		t.Fatalf("Do(submit) error = %v", err)
	}
	defer submitResp.Body.Close()

	select {
	case body := <-bodyCh:
		return body
	case err := <-errCh:
		t.Fatalf("stream error = %v", err)
		return ""
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for HTTP tool-call flow")
		return ""
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

func (s *turnSubmitStore) ListSessions(context.Context) ([]types.Session, error) { return nil, nil }

func (s *turnSubmitStore) GetSession(context.Context, string) (types.Session, bool, error) {
	return types.Session{}, false, nil
}

func (s *turnSubmitStore) UpdateSessionSystemPrompt(context.Context, string, string) (types.Session, bool, error) {
	return types.Session{}, false, nil
}

func (s *turnSubmitStore) GetSelectedSessionID(context.Context) (string, bool, error) {
	return "", false, nil
}

func (s *turnSubmitStore) SetSelectedSessionID(context.Context, string) error { return nil }

func (s *turnSubmitStore) DeleteSession(context.Context, string) (string, bool, error) {
	return "", false, nil
}

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

func (s *turnSubmitStore) ListTurnsBySession(context.Context, string) ([]types.Turn, error) {
	return nil, nil
}

func (s *turnSubmitStore) ListConversationItems(context.Context, string) ([]model.ConversationItem, error) {
	return nil, nil
}

func (s *turnSubmitStore) ListSessionEvents(context.Context, string, int64) ([]types.Event, error) {
	return nil, nil
}

func (s *turnSubmitStore) LatestSessionEventSeq(context.Context, string) (int64, error) {
	return 0, nil
}

type turnSubmitManager struct {
	sessionID     string
	input         session.SubmitTurnInput
	called        bool
	err           error
	cancelRequest func()
}

func (m *turnSubmitManager) RegisterSession(types.Session) {}

func (m *turnSubmitManager) UpdateSession(types.Session) bool { return true }

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

func (m *turnSubmitManager) Subscribe(string) (<-chan types.Event, func()) {
	return nil, func() {}
}

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

func (s *replayStore) ListSessions(context.Context) ([]types.Session, error) { return nil, nil }

func (s *replayStore) GetSession(context.Context, string) (types.Session, bool, error) {
	return types.Session{}, false, nil
}

func (s *replayStore) UpdateSessionSystemPrompt(context.Context, string, string) (types.Session, bool, error) {
	return types.Session{}, false, nil
}

func (s *replayStore) GetSelectedSessionID(context.Context) (string, bool, error) {
	return "", false, nil
}

func (s *replayStore) SetSelectedSessionID(context.Context, string) error { return nil }

func (s *replayStore) DeleteSession(context.Context, string) (string, bool, error) {
	return "", false, nil
}

func (s *replayStore) InsertTurn(context.Context, types.Turn) error { return nil }

func (s *replayStore) DeleteTurn(context.Context, string) error { return nil }

func (s *replayStore) ListTurnsBySession(context.Context, string) ([]types.Turn, error) {
	return nil, nil
}

func (s *replayStore) ListConversationItems(context.Context, string) ([]model.ConversationItem, error) {
	return nil, nil
}

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

func (s *replayStore) LatestSessionEventSeq(context.Context, string) (int64, error) {
	if len(s.events) > 0 {
		return s.events[len(s.events)-1].Seq, nil
	}
	return 0, nil
}

type replayBus struct {
	store           *replayStore
	subscribeCalled bool
}

func (b *replayBus) Subscribe(sessionID string) (<-chan types.Event, func()) {
	b.subscribeCalled = true
	ch := make(chan types.Event)
	close(ch)
	return ch, func() {}
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
	if !strings.Contains(body, "\"type\":\"turn.started\"") {
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

func (b *handoffBus) Subscribe(sessionID string) (<-chan types.Event, func()) {
	b.subscribed = true
	if b.published && b.ch == nil {
		ch := make(chan types.Event)
		close(ch)
		return ch, func() {}
	}
	if b.ch == nil {
		b.ch = make(chan types.Event, 1)
	}
	return b.ch, func() {}
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
