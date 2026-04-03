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
	session types.Session
	called  bool
}

func (s *fakeStore) InsertSession(ctx context.Context, session types.Session) error {
	s.called = true
	s.session = session
	return nil
}

func (s *fakeStore) InsertTurn(ctx context.Context, turn types.Turn) error {
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
