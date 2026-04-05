package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go-agent/internal/model"
	"go-agent/internal/session"
	"go-agent/internal/store/sqlite"
	"go-agent/internal/types"
)

type fakeStore struct {
	session           types.Session
	sessions          []types.Session
	turns             []types.Turn
	conversationItems []model.ConversationItem
	latestSeq         int64
	selectedSessionID string
	hasSelected       bool
	setSelectedCalls  int
	called            bool
}

func (s *fakeStore) InsertSession(ctx context.Context, session types.Session) error {
	s.called = true
	s.session = session
	s.sessions = append([]types.Session{session}, s.sessions...)
	return nil
}

func (s *fakeStore) ListSessions(context.Context) ([]types.Session, error) {
	return append([]types.Session(nil), s.sessions...), nil
}

func (s *fakeStore) UpdateSessionSystemPrompt(_ context.Context, sessionID, systemPrompt string) (types.Session, bool, error) {
	for i, session := range s.sessions {
		if session.ID != sessionID {
			continue
		}
		session.SystemPrompt = systemPrompt
		s.sessions[i] = session
		if s.session.ID == sessionID {
			s.session = session
		}
		return session, true, nil
	}
	return types.Session{}, false, nil
}

func (s *fakeStore) GetSelectedSessionID(context.Context) (string, bool, error) {
	return s.selectedSessionID, s.hasSelected, nil
}

func (s *fakeStore) SetSelectedSessionID(_ context.Context, sessionID string) error {
	s.selectedSessionID = sessionID
	s.hasSelected = true
	s.setSelectedCalls++
	return nil
}

func (s *fakeStore) InsertTurn(ctx context.Context, turn types.Turn) error {
	s.turns = append(s.turns, turn)
	return nil
}

func (s *fakeStore) DeleteTurn(ctx context.Context, turnID string) error {
	return nil
}

func (s *fakeStore) ListSessionEvents(ctx context.Context, sessionID string, afterSeq int64) ([]types.Event, error) {
	return nil, nil
}

func (s *fakeStore) GetSession(ctx context.Context, sessionID string) (types.Session, bool, error) {
	for _, session := range s.sessions {
		if session.ID == sessionID {
			return session, true, nil
		}
	}
	if s.session.ID == sessionID {
		return s.session, true, nil
	}
	return types.Session{}, false, nil
}

func (s *fakeStore) ListTurnsBySession(ctx context.Context, sessionID string) ([]types.Turn, error) {
	out := make([]types.Turn, 0, len(s.turns))
	for _, turn := range s.turns {
		if turn.SessionID == sessionID {
			out = append(out, turn)
		}
	}
	return out, nil
}

func (s *fakeStore) ListConversationItems(ctx context.Context, sessionID string) ([]model.ConversationItem, error) {
	return append([]model.ConversationItem(nil), s.conversationItems...), nil
}

func (s *fakeStore) LatestSessionEventSeq(ctx context.Context, sessionID string) (int64, error) {
	return s.latestSeq, nil
}

type fakeManager struct {
	session types.Session
	called  bool
	updated types.Session
}

func (m *fakeManager) RegisterSession(session types.Session) {
	m.called = true
	m.session = session
}

func (m *fakeManager) UpdateSession(session types.Session) bool {
	m.updated = session
	m.session = session
	return true
}

func (m *fakeManager) SubmitTurn(ctx context.Context, sessionID string, in session.SubmitTurnInput) (string, error) {
	return "", nil
}

func (m *fakeManager) Subscribe(sessionID string) <-chan types.Event {
	return nil
}

func TestRouterExposesStatusEndpoint(t *testing.T) {
	handler := NewRouter(Dependencies{})

	req := httptest.NewRequest(http.MethodGet, "/v1/status", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), "\"status\":\"ok\"") {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

func TestStatusEndpointIncludesRuntimeMetadata(t *testing.T) {
	handler := NewRouter(Dependencies{
		Status: StatusPayload{
			Provider:             "openai_compatible",
			PermissionProfile:    "trusted_local",
			Model:                "glm-4-7-251222",
			ProviderCacheProfile: "ark_responses",
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/status", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d", rec.Code, http.StatusOK)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "\"provider\":\"openai_compatible\"") {
		t.Fatalf("body = %q, want provider metadata", body)
	}
	if !strings.Contains(body, "\"permission_profile\":\"trusted_local\"") {
		t.Fatalf("body = %q, want permission profile metadata", body)
	}
	if !strings.Contains(body, "\"provider_cache_profile\":\"ark_responses\"") {
		t.Fatalf("body = %q, want provider cache profile metadata", body)
	}
	if strings.Contains(body, "OPENAI_API_KEY") {
		t.Fatalf("body = %q, should not leak secrets", body)
	}
}

func TestCreateSessionPersistsAndReturnsSession(t *testing.T) {
	store := &fakeStore{}
	manager := &fakeManager{}
	handler := NewRouter(Dependencies{
		Store:   store,
		Manager: manager,
	})

	reqBody := `{"workspace_root":"D:/work/demo","system_prompt":"focus on internal/model"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/sessions", strings.NewReader(reqBody))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status code = %d, want %d", rec.Code, http.StatusCreated)
	}
	if !store.called {
		t.Fatal("store.InsertSession was not called")
	}
	if store.session.WorkspaceRoot != "D:/work/demo" {
		t.Fatalf("workspace root = %q, want %q", store.session.WorkspaceRoot, "D:/work/demo")
	}
	if store.session.SystemPrompt != "focus on internal/model" {
		t.Fatalf("system prompt = %q, want %q", store.session.SystemPrompt, "focus on internal/model")
	}
	if !manager.called {
		t.Fatal("manager.RegisterSession was not called")
	}
	if manager.session.ID == "" {
		t.Fatal("registered session ID is empty")
	}
	if manager.session.WorkspaceRoot != "D:/work/demo" {
		t.Fatalf("registered workspace root = %q, want %q", manager.session.WorkspaceRoot, "D:/work/demo")
	}
	if manager.session.SystemPrompt != "focus on internal/model" {
		t.Fatalf("registered system prompt = %q, want %q", manager.session.SystemPrompt, "focus on internal/model")
	}
	if manager.session.State != types.SessionStateIdle {
		t.Fatalf("registered state = %q, want %q", manager.session.State, types.SessionStateIdle)
	}

	var got types.Session
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal response error = %v", err)
	}
	if got.WorkspaceRoot != "D:/work/demo" {
		t.Fatalf("response workspace_root = %q, want %q", got.WorkspaceRoot, "D:/work/demo")
	}
	if got.SystemPrompt != "focus on internal/model" {
		t.Fatalf("response system_prompt = %q, want %q", got.SystemPrompt, "focus on internal/model")
	}
	if !strings.Contains(rec.Body.String(), `"workspace_root":"D:/work/demo"`) {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

func TestPatchSessionUpdatesSystemPrompt(t *testing.T) {
	store := &fakeStore{
		sessions: []types.Session{{
			ID:            "sess_1",
			WorkspaceRoot: "D:/work/demo",
			SystemPrompt:  "old prompt",
			State:         types.SessionStateIdle,
		}},
	}
	manager := &fakeManager{}
	handler := NewRouter(Dependencies{
		Store:   store,
		Manager: manager,
	})

	req := httptest.NewRequest(http.MethodPatch, "/v1/sessions/sess_1", strings.NewReader(`{"system_prompt":"new prompt"}`))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := store.sessions[0].SystemPrompt; got != "new prompt" {
		t.Fatalf("store system prompt = %q, want %q", got, "new prompt")
	}
	if got := manager.updated.SystemPrompt; got != "new prompt" {
		t.Fatalf("manager updated system prompt = %q, want %q", got, "new prompt")
	}

	var session types.Session
	if err := json.Unmarshal(rec.Body.Bytes(), &session); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if session.SystemPrompt != "new prompt" {
		t.Fatalf("response system prompt = %q, want %q", session.SystemPrompt, "new prompt")
	}
}

func TestListSessionsReturnsSelectedSessionID(t *testing.T) {
	store := &fakeStore{
		sessions: []types.Session{
			{ID: "sess_2", WorkspaceRoot: "D:/work/two"},
			{ID: "sess_1", WorkspaceRoot: "D:/work/one"},
		},
		selectedSessionID: "sess_1",
		hasSelected:       true,
	}
	handler := NewRouter(Dependencies{
		Store:   store,
		Manager: &fakeManager{},
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/sessions", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d", rec.Code, http.StatusOK)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "\"selected_session_id\":\"sess_1\"") {
		t.Fatalf("body = %q, want selected session id", body)
	}
	if !strings.Contains(body, "\"is_selected\":true") {
		t.Fatalf("body = %q, want selected marker", body)
	}
}

func TestCreateSessionDoesNotStealSelectedFocus(t *testing.T) {
	store := &fakeStore{
		selectedSessionID: "sess_existing",
		hasSelected:       true,
	}
	handler := NewRouter(Dependencies{
		Store:   store,
		Manager: &fakeManager{},
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/sessions", strings.NewReader(`{"workspace_root":"D:/work/demo"}`))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status code = %d, want %d", rec.Code, http.StatusCreated)
	}
	if store.setSelectedCalls != 0 {
		t.Fatalf("SetSelectedSessionID calls = %d, want 0", store.setSelectedCalls)
	}
	if store.selectedSessionID != "sess_existing" {
		t.Fatalf("selected session = %q, want %q", store.selectedSessionID, "sess_existing")
	}
}

func TestSelectSessionPersistsExplicitFocus(t *testing.T) {
	store := &fakeStore{
		sessions: []types.Session{
			{ID: "sess_1", WorkspaceRoot: "D:/work/one"},
			{ID: "sess_2", WorkspaceRoot: "D:/work/two"},
		},
	}
	handler := NewRouter(Dependencies{
		Store:   store,
		Manager: &fakeManager{},
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/sessions/sess_2/select", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d", rec.Code, http.StatusOK)
	}
	if store.selectedSessionID != "sess_2" || store.setSelectedCalls != 1 {
		t.Fatalf("selected session = %q, calls = %d, want sess_2 and 1", store.selectedSessionID, store.setSelectedCalls)
	}
}

func TestListSessionsIncludesDerivedTitleAndPreview(t *testing.T) {
	deps := NewTestDependencies(t)
	store := deps.Store.(*sqlite.Store)
	now := time.Now().UTC()
	session := types.Session{
		ID:            "sess_derive",
		WorkspaceRoot: "E:/project/go-agent",
		State:         types.SessionStateIdle,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := store.InsertSession(context.Background(), session); err != nil {
		t.Fatalf("InsertSession() error = %v", err)
	}
	if err := store.InsertTurn(context.Background(), types.Turn{
		ID:          "turn_1",
		SessionID:   session.ID,
		State:       types.TurnStateCompleted,
		UserMessage: "Inspect README",
		CreatedAt:   now,
		UpdatedAt:   now,
	}); err != nil {
		t.Fatalf("InsertTurn(turn_1) error = %v", err)
	}
	if err := store.InsertTurn(context.Background(), types.Turn{
		ID:          "turn_2",
		SessionID:   session.ID,
		State:       types.TurnStateCompleted,
		UserMessage: "Check shell tool",
		CreatedAt:   now.Add(time.Second),
		UpdatedAt:   now.Add(time.Second),
	}); err != nil {
		t.Fatalf("InsertTurn(turn_2) error = %v", err)
	}

	handler := NewRouter(deps)
	req := httptest.NewRequest(http.MethodGet, "/v1/sessions", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d", rec.Code, http.StatusOK)
	}

	var got types.ListSessionsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if len(got.Sessions) != 1 {
		t.Fatalf("len(got.Sessions) = %d, want 1", len(got.Sessions))
	}
	if got.Sessions[0].Title != "Inspect README" {
		t.Fatalf("Title = %q, want %q", got.Sessions[0].Title, "Inspect README")
	}
	if got.Sessions[0].LastPreview != "Check shell tool" {
		t.Fatalf("LastPreview = %q, want %q", got.Sessions[0].LastPreview, "Check shell tool")
	}
}

func TestTimelineEndpointReturnsNormalizedBlocksAndLatestSeq(t *testing.T) {
	deps := NewTestDependencies(t)
	store := deps.Store.(*sqlite.Store)
	now := time.Now().UTC()
	if err := store.InsertSession(context.Background(), types.Session{
		ID:            "sess_timeline",
		WorkspaceRoot: "E:/project/go-agent",
		State:         types.SessionStateIdle,
		CreatedAt:     now,
		UpdatedAt:     now,
	}); err != nil {
		t.Fatalf("InsertSession() error = %v", err)
	}
	if err := store.InsertConversationItem(context.Background(), "sess_timeline", "turn_1", 1, model.UserMessageItem("hello")); err != nil {
		t.Fatalf("InsertConversationItem(user) error = %v", err)
	}
	if err := store.InsertConversationItem(context.Background(), "sess_timeline", "turn_1", 2, model.ConversationItem{
		Kind: model.ConversationItemToolCall,
		ToolCall: model.ToolCallChunk{
			ID:    "call_1",
			Name:  "file_read",
			Input: map[string]any{"path": "README.md"},
		},
	}); err != nil {
		t.Fatalf("InsertConversationItem(tool_call) error = %v", err)
	}
	if err := store.InsertConversationItem(context.Background(), "sess_timeline", "turn_1", 3, model.ToolResultItem(model.ToolResult{
		ToolCallID: "call_1",
		ToolName:   "file_read",
		Content:    "readme content",
	})); err != nil {
		t.Fatalf("InsertConversationItem(tool_result) error = %v", err)
	}
	event, err := types.NewEvent("sess_timeline", "turn_1", types.EventTurnCompleted, struct{}{})
	if err != nil {
		t.Fatalf("NewEvent() error = %v", err)
	}
	if _, err := store.AppendEvent(context.Background(), event); err != nil {
		t.Fatalf("AppendEvent() error = %v", err)
	}

	handler := NewRouter(deps)
	req := httptest.NewRequest(http.MethodGet, "/v1/sessions/sess_timeline/timeline", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d", rec.Code, http.StatusOK)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "\"kind\":\"user_message\"") {
		t.Fatalf("body = %s, want user_message block", body)
	}
	if !strings.Contains(body, "\"kind\":\"assistant_message\"") {
		t.Fatalf("body = %s, want assistant_message block", body)
	}
	if !strings.Contains(body, "\"latest_seq\":1") {
		t.Fatalf("body = %s, want latest_seq 1", body)
	}
	if strings.Contains(body, "\"kind\":\"tool_result\"") {
		t.Fatalf("body = %s, want tool_result blocks removed", body)
	}
	if !strings.Contains(body, "\"tool_call_id\":\"call_1\"") {
		t.Fatalf("body = %s, want tool_call_id on assistant message content", body)
	}
	if !strings.Contains(body, "\"result_preview\":\"readme content\"") {
		t.Fatalf("body = %s, want result_preview backfilled onto tool_call content", body)
	}
}

func TestTimelineEndpointGroupsAssistantItemsIntoAssistantMessages(t *testing.T) {
	deps := NewTestDependencies(t)
	store := deps.Store.(*sqlite.Store)
	now := time.Now().UTC()
	if err := store.InsertSession(context.Background(), types.Session{
		ID:            "sess_assistant_message",
		WorkspaceRoot: "E:/project/go-agent",
		State:         types.SessionStateIdle,
		CreatedAt:     now,
		UpdatedAt:     now,
	}); err != nil {
		t.Fatalf("InsertSession() error = %v", err)
	}
	if err := store.InsertConversationItem(context.Background(), "sess_assistant_message", "turn_1", 1, model.UserMessageItem("hello")); err != nil {
		t.Fatalf("InsertConversationItem(user) error = %v", err)
	}
	if err := store.InsertConversationItem(context.Background(), "sess_assistant_message", "turn_1", 2, model.ConversationItem{
		Kind: model.ConversationItemAssistantText,
		Text: "Before tool. ",
	}); err != nil {
		t.Fatalf("InsertConversationItem(assistant_text_1) error = %v", err)
	}
	if err := store.InsertConversationItem(context.Background(), "sess_assistant_message", "turn_1", 3, model.ConversationItem{
		Kind: model.ConversationItemToolCall,
		ToolCall: model.ToolCallChunk{
			ID:    "call_1",
			Name:  "file_read",
			Input: map[string]any{"path": "README.md"},
		},
	}); err != nil {
		t.Fatalf("InsertConversationItem(tool_call) error = %v", err)
	}
	if err := store.InsertConversationItem(context.Background(), "sess_assistant_message", "turn_1", 4, model.ConversationItem{
		Kind: model.ConversationItemAssistantText,
		Text: "After tool.",
	}); err != nil {
		t.Fatalf("InsertConversationItem(assistant_text_2) error = %v", err)
	}
	if err := store.InsertConversationItem(context.Background(), "sess_assistant_message", "turn_1", 5, model.ToolResultItem(model.ToolResult{
		ToolCallID: "call_1",
		ToolName:   "file_read",
		Content:    "readme content",
	})); err != nil {
		t.Fatalf("InsertConversationItem(tool_result) error = %v", err)
	}

	handler := NewRouter(deps)
	req := httptest.NewRequest(http.MethodGet, "/v1/sessions/sess_assistant_message/timeline", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d", rec.Code, http.StatusOK)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "\"kind\":\"assistant_message\"") {
		t.Fatalf("body = %s, want assistant_message block", body)
	}
	if !strings.Contains(body, "\"content\":[{\"type\":\"text\",\"text\":\"Before tool. \"},{\"type\":\"tool_call\"") {
		t.Fatalf("body = %s, want assistant content blocks in order", body)
	}
	if !strings.Contains(body, "\"text\":\"After tool.\"") {
		t.Fatalf("body = %s, want trailing assistant text inside message content", body)
	}
	if strings.Contains(body, "\"kind\":\"tool_result\"") {
		t.Fatalf("body = %s, want tool_result blocks removed", body)
	}
	if !strings.Contains(body, "\"result_preview\":\"readme content\"") {
		t.Fatalf("body = %s, want result_preview on tool_call content", body)
	}
}

func TestWorkspaceEndpointReturnsWorkspaceAndRuntimeMetadata(t *testing.T) {
	deps := NewTestDependencies(t)
	deps.Status = StatusPayload{
		Provider:             "openai_compatible",
		Model:                "glm-4-7-251222",
		PermissionProfile:    "trusted_local",
		ProviderCacheProfile: "ark_responses",
	}
	store := deps.Store.(*sqlite.Store)
	now := time.Now().UTC()
	if err := store.InsertSession(context.Background(), types.Session{
		ID:            "sess_workspace",
		WorkspaceRoot: "E:/project/go-agent",
		State:         types.SessionStateIdle,
		CreatedAt:     now,
		UpdatedAt:     now,
	}); err != nil {
		t.Fatalf("InsertSession() error = %v", err)
	}

	handler := NewRouter(deps)
	req := httptest.NewRequest(http.MethodGet, "/v1/sessions/sess_workspace/workspace", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d", rec.Code, http.StatusOK)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "\"workspace_root\":\"E:/project/go-agent\"") {
		t.Fatalf("body = %s, want workspace_root", body)
	}
	if !strings.Contains(body, "\"model\":\"glm-4-7-251222\"") {
		t.Fatalf("body = %s, want model metadata", body)
	}
	if !strings.Contains(body, "\"permission_profile\":\"trusted_local\"") {
		t.Fatalf("body = %s, want permission profile metadata", body)
	}
}

func TestMetricsOverviewEndpointReturnsAggregates(t *testing.T) {
	deps := NewTestDependencies(t)
	store := deps.Store.(*sqlite.Store)
	seedMetricsFixture(t, store)

	handler := NewRouter(deps)
	req := httptest.NewRequest(http.MethodGet, "/v1/metrics/overview?session_id=sess_metrics_1", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d", rec.Code, http.StatusOK)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "\"input_tokens\":200") {
		t.Fatalf("body = %s, want input token aggregate", body)
	}
	if !strings.Contains(body, "\"output_tokens\":50") {
		t.Fatalf("body = %s, want output token aggregate", body)
	}
	if !strings.Contains(body, "\"cached_tokens\":40") {
		t.Fatalf("body = %s, want cached token aggregate", body)
	}
	if !strings.Contains(body, "\"cache_hit_rate\":0.2") {
		t.Fatalf("body = %s, want cache hit rate", body)
	}
}

func TestMetricsTimeseriesEndpointReturnsBuckets(t *testing.T) {
	deps := NewTestDependencies(t)
	store := deps.Store.(*sqlite.Store)
	seedMetricsFixture(t, store)

	handler := NewRouter(deps)
	req := httptest.NewRequest(http.MethodGet, "/v1/metrics/timeseries?bucket=day", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d", rec.Code, http.StatusOK)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "\"bucket\":\"day\"") {
		t.Fatalf("body = %s, want bucket marker", body)
	}
	if !strings.Contains(body, "\"input_tokens\":200") || !strings.Contains(body, "\"input_tokens\":30") {
		t.Fatalf("body = %s, want grouped input tokens", body)
	}
}

func TestMetricsTurnsEndpointReturnsPaginatedRows(t *testing.T) {
	deps := NewTestDependencies(t)
	store := deps.Store.(*sqlite.Store)
	seedMetricsFixture(t, store)

	handler := NewRouter(deps)
	req := httptest.NewRequest(http.MethodGet, "/v1/metrics/turns?page=1&page_size=1", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d", rec.Code, http.StatusOK)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "\"total_count\":2") {
		t.Fatalf("body = %s, want total_count 2", body)
	}
	if !strings.Contains(body, "\"page\":1") || !strings.Contains(body, "\"page_size\":1") {
		t.Fatalf("body = %s, want pagination metadata", body)
	}
	if !strings.Contains(body, "\"session_title\":\"检查 shell 输出限制\"") {
		t.Fatalf("body = %s, want derived session title", body)
	}
	if !strings.Contains(body, "\"turn_id\":\"turn_metrics_2\"") {
		t.Fatalf("body = %s, want newest turn row", body)
	}
}

func TestConsoleRouteServesIndexWhenConfigured(t *testing.T) {
	deps := NewTestDependencies(t)
	consoleDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(consoleDir, "index.html"), []byte("<!doctype html><title>console</title>"), 0o644); err != nil {
		t.Fatalf("WriteFile(index.html) error = %v", err)
	}
	deps.ConsoleRoot = consoleDir

	handler := NewRouter(deps)
	req := httptest.NewRequest(http.MethodGet, "/chat", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), "<title>console</title>") {
		t.Fatalf("body = %s, want embedded console html", rec.Body.String())
	}
}

func seedMetricsFixture(t *testing.T, store *sqlite.Store) {
	t.Helper()

	firstTime := time.Date(2026, 4, 4, 8, 0, 0, 0, time.UTC)
	secondTime := firstTime.Add(24 * time.Hour)
	if err := store.InsertSession(context.Background(), types.Session{
		ID:            "sess_metrics_1",
		WorkspaceRoot: "E:/project/go-agent",
		State:         types.SessionStateIdle,
		CreatedAt:     firstTime,
		UpdatedAt:     firstTime,
	}); err != nil {
		t.Fatalf("InsertSession(sess_metrics_1) error = %v", err)
	}
	if err := store.InsertSession(context.Background(), types.Session{
		ID:            "sess_metrics_2",
		WorkspaceRoot: "E:/project/go-agent",
		State:         types.SessionStateIdle,
		CreatedAt:     secondTime,
		UpdatedAt:     secondTime,
	}); err != nil {
		t.Fatalf("InsertSession(sess_metrics_2) error = %v", err)
	}
	if err := store.InsertTurn(context.Background(), types.Turn{
		ID:          "turn_metrics_1",
		SessionID:   "sess_metrics_1",
		State:       types.TurnStateCompleted,
		UserMessage: "查看 README 结构",
		CreatedAt:   firstTime,
		UpdatedAt:   firstTime,
	}); err != nil {
		t.Fatalf("InsertTurn(turn_metrics_1) error = %v", err)
	}
	if err := store.InsertTurn(context.Background(), types.Turn{
		ID:          "turn_metrics_2",
		SessionID:   "sess_metrics_2",
		State:       types.TurnStateCompleted,
		UserMessage: "检查 shell 输出限制",
		CreatedAt:   secondTime,
		UpdatedAt:   secondTime,
	}); err != nil {
		t.Fatalf("InsertTurn(turn_metrics_2) error = %v", err)
	}
	if err := store.UpsertTurnUsage(context.Background(), types.TurnUsage{
		TurnID:       "turn_metrics_1",
		SessionID:    "sess_metrics_1",
		Provider:     "openai_compatible",
		Model:        "glm-4-7-251222",
		InputTokens:  200,
		OutputTokens: 50,
		CachedTokens: 40,
		CacheHitRate: 0.2,
		CreatedAt:    firstTime,
		UpdatedAt:    firstTime,
	}); err != nil {
		t.Fatalf("UpsertTurnUsage(turn_metrics_1) error = %v", err)
	}
	if err := store.UpsertTurnUsage(context.Background(), types.TurnUsage{
		TurnID:       "turn_metrics_2",
		SessionID:    "sess_metrics_2",
		Provider:     "openai_compatible",
		Model:        "glm-4-7-251222",
		InputTokens:  30,
		OutputTokens: 12,
		CachedTokens: 6,
		CacheHitRate: 0.2,
		CreatedAt:    secondTime,
		UpdatedAt:    secondTime,
	}); err != nil {
		t.Fatalf("UpsertTurnUsage(turn_metrics_2) error = %v", err)
	}
}
