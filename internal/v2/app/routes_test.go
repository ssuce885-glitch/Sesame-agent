package app

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"go-agent/internal/config"
	"go-agent/internal/v2/contracts"
	"go-agent/internal/v2/roles"
	v2store "go-agent/internal/v2/store"
)

func TestRoutesStatusAndRoleCRUD(t *testing.T) {
	ctx := context.Background()
	s, err := v2store.OpenInMemory()
	if err != nil {
		t.Fatalf("OpenInMemory: %v", err)
	}
	defer s.Close()

	workspaceRoot := t.TempDir()
	now := time.Now().UTC()
	session := contracts.Session{
		ID:                "session-1",
		WorkspaceRoot:     workspaceRoot,
		SystemPrompt:      "You are Sesame.",
		PermissionProfile: "workspace",
		State:             "idle",
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	if err := s.Sessions().Create(ctx, session); err != nil {
		t.Fatalf("create session: %v", err)
	}
	sessionMgr := &testSessionManager{}
	sessionMgr.Register(session)

	handler := (&routes{
		cfg: config.Config{
			Addr:              "127.0.0.1:8421",
			Model:             "test-model",
			PermissionProfile: "workspace",
			Paths: config.Paths{
				WorkspaceRoot: workspaceRoot,
				DataDir:       filepath.Join(workspaceRoot, ".sesame"),
			},
		},
		store:            s,
		sessionMgr:       sessionMgr,
		roleService:      roles.NewService(workspaceRoot),
		defaultSessionID: session.ID,
	}).handler()

	status := decodeJSON[map[string]any](t, handler, http.MethodGet, "/v2/status", nil, http.StatusOK)
	if status["default_session_id"] != session.ID {
		t.Fatalf("default_session_id = %v, want %s", status["default_session_id"], session.ID)
	}

	rolePayload := map[string]any{
		"id":                 "smoke_role",
		"name":               "Smoke Role",
		"system_prompt":      "You are a smoke role.",
		"permission_profile": "workspace",
		"model":              "test-role-model",
		"max_tool_calls":     3,
		"max_runtime":        60,
		"skill_names":        []string{"email"},
	}
	created := decodeJSON[roles.RoleSpec](t, handler, http.MethodPost, "/v2/roles", rolePayload, http.StatusCreated)
	if created.ID != "smoke_role" || created.Name != "Smoke Role" || created.Model != "test-role-model" {
		t.Fatalf("created role = %+v", created)
	}

	fetched := decodeJSON[roles.RoleSpec](t, handler, http.MethodGet, "/v2/roles/smoke_role", nil, http.StatusOK)
	if fetched.ID != created.ID || fetched.SystemPrompt != "You are a smoke role." {
		t.Fatalf("fetched role = %+v", fetched)
	}

	rolePayload["name"] = "Smoke Role Updated"
	rolePayload["system_prompt"] = "Updated role prompt."
	updated := decodeJSON[roles.RoleSpec](t, handler, http.MethodPut, "/v2/roles/smoke_role", rolePayload, http.StatusOK)
	if updated.Name != "Smoke Role Updated" || updated.SystemPrompt != "Updated role prompt." || updated.Version != 2 {
		t.Fatalf("updated role = %+v", updated)
	}
}

func decodeJSON[T any](t *testing.T, handler http.Handler, method, path string, body any, wantStatus int) T {
	t.Helper()
	var reader *bytes.Reader
	if body == nil {
		reader = bytes.NewReader(nil)
	} else {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		reader = bytes.NewReader(raw)
	}
	req := httptest.NewRequest(method, path, reader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != wantStatus {
		t.Fatalf("%s %s status = %d, want %d, body %s", method, path, rec.Code, wantStatus, rec.Body.String())
	}
	var out T
	if err := json.NewDecoder(rec.Body).Decode(&out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return out
}

type testSessionManager struct {
	sessions map[string]contracts.Session
}

func (m *testSessionManager) Register(session contracts.Session) {
	if m.sessions == nil {
		m.sessions = map[string]contracts.Session{}
	}
	m.sessions[session.ID] = session
}

func (m *testSessionManager) SubmitTurn(context.Context, string, contracts.SubmitTurnInput) (string, error) {
	return "", nil
}

func (m *testSessionManager) CancelTurn(string, string) bool { return false }

func (m *testSessionManager) QueuePayload(sessionID string) (contracts.QueuePayload, bool) {
	if _, ok := m.sessions[sessionID]; !ok {
		return contracts.QueuePayload{}, false
	}
	return contracts.QueuePayload{}, true
}
