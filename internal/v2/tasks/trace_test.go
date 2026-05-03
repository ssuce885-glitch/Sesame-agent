package tasks

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"go-agent/internal/v2/contracts"
	"go-agent/internal/v2/store"
)

func TestBuildTraceIncludesRunningRoleEventsMessagesReportsAndLog(t *testing.T) {
	s, err := store.OpenInMemory()
	if err != nil {
		t.Fatal(err)
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
		ID:            "specialist_reviewer",
		WorkspaceRoot: workspace,
		State:         "running",
		ActiveTurnID:  "turn_role",
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := s.Sessions().Create(context.Background(), mainSession); err != nil {
		t.Fatal(err)
	}
	if err := s.Sessions().Create(context.Background(), roleSession); err != nil {
		t.Fatal(err)
	}
	turn := contracts.Turn{
		ID:          "turn_role",
		SessionID:   roleSession.ID,
		Kind:        "user_message",
		State:       "running",
		UserMessage: "inspect this",
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := s.Turns().Create(context.Background(), turn); err != nil {
		t.Fatal(err)
	}
	if err := s.Messages().Append(context.Background(), []contracts.Message{{
		SessionID: roleSession.ID,
		TurnID:    turn.ID,
		Role:      "user",
		Content:   "inspect this",
		Position:  1,
		CreatedAt: now,
	}}); err != nil {
		t.Fatal(err)
	}
	if err := s.Events().Append(context.Background(), []contracts.Event{{
		ID:        "event_1",
		SessionID: roleSession.ID,
		TurnID:    turn.ID,
		Type:      "tool_call",
		Time:      now,
		Payload:   `{"name":"shell","args":{"cmd":"pwd"}}`,
	}}); err != nil {
		t.Fatal(err)
	}
	if err := s.Reports().Create(context.Background(), contracts.Report{
		ID:         "report_1",
		SessionID:  mainSession.ID,
		SourceKind: "task_result",
		SourceID:   "task_trace",
		Status:     "running",
		Severity:   "info",
		Title:      "Task result: agent",
		Summary:    "partial",
		CreatedAt:  now,
	}); err != nil {
		t.Fatal(err)
	}

	logPath := filepath.Join(t.TempDir(), "task.log")
	if err := os.WriteFile(logPath, []byte("line 1\nline 2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	task := contracts.Task{
		ID:              "task_trace",
		WorkspaceRoot:   workspace,
		SessionID:       roleSession.ID,
		RoleID:          "reviewer",
		TurnID:          turn.ID,
		ParentSessionID: mainSession.ID,
		ParentTurnID:    "turn_main",
		ReportSessionID: mainSession.ID,
		Kind:            "agent",
		State:           "running",
		Prompt:          "inspect this",
		OutputPath:      logPath,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if err := s.Tasks().Create(context.Background(), task); err != nil {
		t.Fatal(err)
	}

	trace, err := BuildTrace(context.Background(), s, task, TraceOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if trace.State.Task != "running" || trace.State.Turn != "running" || trace.State.Session != "running" {
		t.Fatalf("unexpected state: %+v", trace.State)
	}
	if trace.Parent.SessionID != mainSession.ID || trace.Role.SessionID != roleSession.ID {
		t.Fatalf("unexpected linkage: parent=%+v role=%+v", trace.Parent, trace.Role)
	}
	if len(trace.Events) != 1 || trace.Events[0].Type != "tool_call" {
		t.Fatalf("unexpected events: %+v", trace.Events)
	}
	if len(trace.Messages) != 1 || trace.Messages[0].Role != "user" {
		t.Fatalf("unexpected messages: %+v", trace.Messages)
	}
	if len(trace.Reports) != 1 || trace.Reports[0].SessionID != mainSession.ID {
		t.Fatalf("unexpected reports: %+v", trace.Reports)
	}
	if trace.LogPreview != "line 1\nline 2\n" {
		t.Fatalf("unexpected log preview: %q", trace.LogPreview)
	}
}
