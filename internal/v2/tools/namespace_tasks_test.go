package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"go-agent/internal/v2/contracts"
	"go-agent/internal/v2/store"
)

func TestTaskTraceToolReturnsRunningRoleSummary(t *testing.T) {
	s, err := store.OpenInMemory()
	if err != nil {
		t.Fatalf("OpenInMemory: %v", err)
	}
	defer s.Close()

	now := time.Now().UTC()
	workspace := t.TempDir()
	mainSession := contracts.Session{
		ID:            "main_session",
		WorkspaceRoot: workspace,
		State:         "idle",
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	roleSession := contracts.Session{
		ID:            "role_session",
		WorkspaceRoot: workspace,
		State:         "running",
		ActiveTurnID:  "role_turn",
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := s.Sessions().Create(context.Background(), mainSession); err != nil {
		t.Fatal(err)
	}
	if err := s.Sessions().Create(context.Background(), roleSession); err != nil {
		t.Fatal(err)
	}
	if err := s.Turns().Create(context.Background(), contracts.Turn{
		ID:          "role_turn",
		SessionID:   roleSession.ID,
		Kind:        "user_message",
		State:       "running",
		UserMessage: "inspect this",
		CreatedAt:   now,
		UpdatedAt:   now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.Events().Append(context.Background(), []contracts.Event{{
		ID:        "event_1",
		SessionID: roleSession.ID,
		TurnID:    "role_turn",
		Type:      "tool_call",
		Time:      now,
		Payload:   `{"name":"shell"}`,
	}}); err != nil {
		t.Fatal(err)
	}
	if err := s.Tasks().Create(context.Background(), contracts.Task{
		ID:              "task_running",
		WorkspaceRoot:   workspace,
		SessionID:       roleSession.ID,
		RoleID:          "reviewer",
		TurnID:          "role_turn",
		ParentSessionID: mainSession.ID,
		ParentTurnID:    "main_turn",
		ReportSessionID: mainSession.ID,
		Kind:            "agent",
		State:           "running",
		Prompt:          "inspect this",
		CreatedAt:       now,
		UpdatedAt:       now,
	}); err != nil {
		t.Fatal(err)
	}

	result, err := NewTaskTraceTool().Execute(context.Background(), contracts.ToolCall{
		Name: "task_trace",
		Args: map[string]any{"task_id": "task_running"},
	}, contracts.ExecContext{Store: s})
	if err != nil {
		t.Fatalf("task_trace returned error: %v", err)
	}
	if result.IsError {
		t.Fatalf("task_trace failed: %s", result.Output)
	}
	if !strings.Contains(result.Output, `"task_id":"task_running"`) || !strings.Contains(result.Output, `"task":"running"`) {
		t.Fatalf("unexpected summary output: %s", result.Output)
	}
	var trace contractsTrace
	raw, err := json.Marshal(result.Data)
	if err != nil {
		t.Fatalf("marshal data: %v", err)
	}
	if err := json.Unmarshal(raw, &trace); err != nil {
		t.Fatalf("decode trace data: %v", err)
	}
	if trace.Parent.SessionID != mainSession.ID || trace.Role.SessionID != roleSession.ID || len(trace.Events) != 1 {
		t.Fatalf("unexpected trace data: %+v", trace)
	}
}

func TestTaskTraceToolRejectsRoleMismatch(t *testing.T) {
	s, err := store.OpenInMemory()
	if err != nil {
		t.Fatalf("OpenInMemory: %v", err)
	}
	defer s.Close()

	now := time.Now().UTC()
	workspace := t.TempDir()
	if err := s.Tasks().Create(context.Background(), contracts.Task{
		ID:            "task_other_role",
		WorkspaceRoot: workspace,
		RoleID:        "other_role",
		Kind:          "agent",
		State:         "running",
		Prompt:        "private work",
		CreatedAt:     now,
		UpdatedAt:     now,
	}); err != nil {
		t.Fatal(err)
	}

	result, err := NewTaskTraceTool().Execute(context.Background(), contracts.ToolCall{
		Name: "task_trace",
		Args: map[string]any{"task_id": "task_other_role"},
	}, contracts.ExecContext{
		WorkspaceRoot: workspace,
		Store:         s,
		RoleSpec:      &contracts.RoleSpec{ID: "reviewer"},
	})
	if err != nil {
		t.Fatalf("task_trace returned error: %v", err)
	}
	if !result.IsError || !strings.Contains(result.Output, "not found") {
		t.Fatalf("expected role mismatch to be hidden as not found, got %+v", result)
	}

	result, err = NewTaskTraceTool().Execute(context.Background(), contracts.ToolCall{
		Name: "task_trace",
		Args: map[string]any{"task_id": "task_other_role"},
	}, contracts.ExecContext{
		Store:    s,
		RoleSpec: &contracts.RoleSpec{ID: "other_role"},
	})
	if err != nil {
		t.Fatalf("task_trace without workspace returned error: %v", err)
	}
	if !result.IsError || !strings.Contains(result.Output, "not found") {
		t.Fatalf("expected missing workspace to be hidden as not found, got %+v", result)
	}
}

type contractsTrace struct {
	Parent struct {
		SessionID string `json:"session_id"`
	} `json:"parent"`
	Role struct {
		SessionID string `json:"session_id"`
	} `json:"role"`
	Events []contracts.Event `json:"events"`
}
