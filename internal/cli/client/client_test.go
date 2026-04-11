package client

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go-agent/internal/types"
)

func TestFindOrCreateWorkspaceSessionPrefersSelectedWorkspaceSession(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/sessions":
			_ = json.NewEncoder(w).Encode(types.ListSessionsResponse{
				SelectedSessionID: "sess_selected",
				Sessions: []types.SessionListItem{
					{ID: "sess_selected", WorkspaceRoot: "E:/project/go-agent"},
					{ID: "sess_other", WorkspaceRoot: "E:/project/other"},
				},
			})
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
	}))
	defer server.Close()

	c := New(server.URL, server.Client())
	sessionID, created, err := c.FindOrCreateWorkspaceSession(context.Background(), "E:/project/go-agent")
	if err != nil {
		t.Fatalf("FindOrCreateWorkspaceSession() error = %v", err)
	}
	if created {
		t.Fatal("created = true, want false")
	}
	if sessionID != "sess_selected" {
		t.Fatalf("sessionID = %q, want %q", sessionID, "sess_selected")
	}
}

func TestFindOrCreateWorkspaceSessionCreatesMissingWorkspaceSession(t *testing.T) {
	var created bool
	var selected string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/sessions":
			_ = json.NewEncoder(w).Encode(types.ListSessionsResponse{
				SelectedSessionID: "sess_other",
				Sessions: []types.SessionListItem{
					{ID: "sess_other", WorkspaceRoot: "E:/project/other"},
				},
			})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/sessions":
			created = true
			var req types.CreateSessionRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("Decode() error = %v", err)
			}
			if req.WorkspaceRoot != "E:/project/go-agent" {
				t.Fatalf("WorkspaceRoot = %q, want %q", req.WorkspaceRoot, "E:/project/go-agent")
			}
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(types.Session{ID: "sess_new", WorkspaceRoot: req.WorkspaceRoot})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/sessions/sess_new/select":
			selected = "sess_new"
			_ = json.NewEncoder(w).Encode(types.SelectSessionResponse{SelectedSessionID: "sess_new"})
		default:
			t.Fatalf("unexpected %s %q", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	c := New(server.URL, server.Client())
	sessionID, wasCreated, err := c.FindOrCreateWorkspaceSession(context.Background(), "E:/project/go-agent")
	if err != nil {
		t.Fatalf("FindOrCreateWorkspaceSession() error = %v", err)
	}
	if !wasCreated {
		t.Fatal("wasCreated = false, want true")
	}
	if !created {
		t.Fatal("create session endpoint was not called")
	}
	if selected != "sess_new" {
		t.Fatalf("selected = %q, want %q", selected, "sess_new")
	}
	if sessionID != "sess_new" {
		t.Fatalf("sessionID = %q, want %q", sessionID, "sess_new")
	}
}

func TestStreamEventsParsesSSEFrames(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "id: 7\nevent: assistant.delta\ndata: {\"id\":\"evt_1\",\"seq\":7,\"session_id\":\"sess_123\",\"type\":\"assistant.delta\",\"time\":\"2026-04-06T00:00:00Z\",\"payload\":{\"text\":\"hello\"}}\n\n")
	}))
	defer server.Close()

	c := New(server.URL, server.Client())
	events, err := c.StreamEvents(context.Background(), "sess_123", 0)
	if err != nil {
		t.Fatalf("StreamEvents() error = %v", err)
	}

	event, ok := <-events
	if !ok {
		t.Fatal("event channel closed before first event")
	}
	if event.Seq != 7 {
		t.Fatalf("Seq = %d, want 7", event.Seq)
	}
	if event.Type != types.EventAssistantDelta {
		t.Fatalf("Type = %q, want %q", event.Type, types.EventAssistantDelta)
	}
	if !strings.Contains(string(event.Payload), "\"hello\"") {
		t.Fatalf("Payload = %s, want hello text", string(event.Payload))
	}
}

func TestStreamEventsParsesLargeSSEFrames(t *testing.T) {
	largeText := strings.Repeat("a", 70*1024)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(
			w,
			"id: 9\nevent: assistant.delta\ndata: {\"id\":\"evt_large\",\"seq\":9,\"session_id\":\"sess_123\",\"type\":\"assistant.delta\",\"time\":\"2026-04-06T00:00:00Z\",\"payload\":{\"text\":\""+largeText+"\"}}\n\n",
		)
	}))
	defer server.Close()

	c := New(server.URL, server.Client())
	events, err := c.StreamEvents(context.Background(), "sess_123", 0)
	if err != nil {
		t.Fatalf("StreamEvents() error = %v", err)
	}

	event, ok := <-events
	if !ok {
		t.Fatal("event channel closed before first large event")
	}
	if event.Seq != 9 {
		t.Fatalf("Seq = %d, want 9", event.Seq)
	}
	if !strings.Contains(string(event.Payload), largeText[:256]) {
		t.Fatal("Payload missing expected large text content")
	}
}

func TestInterruptTurnPostsSessionInterruptEndpoint(t *testing.T) {
	var called bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/v1/sessions/sess_123/interrupt" {
			t.Fatalf("path = %q, want interrupt endpoint", r.URL.Path)
		}
		called = true
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()

	c := New(server.URL, server.Client())
	if err := c.InterruptTurn(context.Background(), "sess_123"); err != nil {
		t.Fatalf("InterruptTurn() error = %v", err)
	}
	if !called {
		t.Fatal("interrupt endpoint was not called")
	}
}

func TestDecidePermissionPostsPermissionDecisionEndpoint(t *testing.T) {
	var called bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/v1/permissions/decide" {
			t.Fatalf("path = %q, want permission decision endpoint", r.URL.Path)
		}
		var req types.PermissionDecisionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("Decode() error = %v", err)
		}
		if req.RequestID != "perm_123" {
			t.Fatalf("RequestID = %q, want perm_123", req.RequestID)
		}
		if req.Decision != types.PermissionDecisionAllowOnce {
			t.Fatalf("Decision = %q, want %q", req.Decision, types.PermissionDecisionAllowOnce)
		}
		called = true
		_ = json.NewEncoder(w).Encode(types.PermissionDecisionResponse{
			Request: types.PermissionRequest{
				ID:               "perm_123",
				RequestedProfile: "trusted_local",
				Status:           types.PermissionRequestStatusApproved,
				Decision:         types.PermissionDecisionAllowOnce,
			},
			TurnID:  "turn_1",
			Resumed: true,
		})
	}))
	defer server.Close()

	c := New(server.URL, server.Client())
	resp, err := c.DecidePermission(context.Background(), types.PermissionDecisionRequest{
		RequestID: "perm_123",
		Decision:  types.PermissionDecisionAllowOnce,
	})
	if err != nil {
		t.Fatalf("DecidePermission() error = %v", err)
	}
	if !called {
		t.Fatal("permission decision endpoint was not called")
	}
	if !resp.Resumed {
		t.Fatal("Resumed = false, want true")
	}
}
