package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go-agent/internal/session"
	"go-agent/internal/types"
)

type fakeStore struct {
	session           types.Session
	sessions          []types.Session
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
	return nil
}

func (s *fakeStore) DeleteTurn(ctx context.Context, turnID string) error {
	return nil
}

func (s *fakeStore) ListSessionEvents(ctx context.Context, sessionID string, afterSeq int64) ([]types.Event, error) {
	return nil, nil
}

type fakeManager struct {
	session types.Session
	called  bool
}

func (m *fakeManager) RegisterSession(session types.Session) {
	m.called = true
	m.session = session
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

	reqBody := `{"workspace_root":"D:/work/demo"}`
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
	if !manager.called {
		t.Fatal("manager.RegisterSession was not called")
	}
	if manager.session.ID == "" {
		t.Fatal("registered session ID is empty")
	}
	if manager.session.WorkspaceRoot != "D:/work/demo" {
		t.Fatalf("registered workspace root = %q, want %q", manager.session.WorkspaceRoot, "D:/work/demo")
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
	if !strings.Contains(rec.Body.String(), `"workspace_root":"D:/work/demo"`) {
		t.Fatalf("body = %s", rec.Body.String())
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
