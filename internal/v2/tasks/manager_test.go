package tasks

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"go-agent/internal/v2/contracts"
	"go-agent/internal/v2/store"
)

func TestManagerCreateAndStart(t *testing.T) {
	s, err := store.OpenInMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	workspaceRoot := t.TempDir()
	m := NewManager(s, t.TempDir())
	task := contracts.Task{
		ID:            "task_test001",
		WorkspaceRoot: workspaceRoot,
		SessionID:     "sess_1",
		Kind:          "shell",
		State:         "pending",
		Prompt:        "echo hello",
	}
	if err := m.Create(context.Background(), task); err != nil {
		t.Fatal(err)
	}

	got, err := s.Tasks().Get(context.Background(), task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != task.ID {
		t.Fatalf("expected %q, got %q", task.ID, got.ID)
	}

	if err := m.Start(context.Background(), task.ID); err != nil {
		t.Fatal(err)
	}
	waitCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	completed, err := m.Wait(waitCtx, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if completed.State != "completed" {
		t.Fatalf("expected completed, got %q", completed.State)
	}
	if completed.Outcome != "success" {
		t.Fatalf("expected success outcome, got %q", completed.Outcome)
	}
	data, err := os.ReadFile(completed.OutputPath)
	if err != nil {
		t.Fatal(err)
	}
	output := string(data)
	if !strings.Contains(output, "hello") {
		t.Fatalf("expected streamed output to contain hello, got %q", output)
	}
	if !strings.Contains(output, "[exit code: 0]") {
		t.Fatalf("expected output to contain exit code, got %q", output)
	}

	reports, err := s.Reports().ListBySession(context.Background(), task.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if len(reports) != 1 {
		t.Fatalf("expected 1 report, got %d", len(reports))
	}
	if reports[0].SourceID != task.ID || reports[0].Severity != "info" {
		t.Fatalf("unexpected report: %+v", reports[0])
	}
}

func TestManagerFinishRunPreservesRunnerFinalText(t *testing.T) {
	s, err := store.OpenInMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	m := NewManager(s, t.TempDir())
	m.RegisterRunner("agent_test", finalTextRunner{store: s})
	task := contracts.Task{
		ID:            "task_final_text",
		WorkspaceRoot: t.TempDir(),
		SessionID:     "sess_final_text",
		Kind:          "agent_test",
		State:         "pending",
		Prompt:        "answer",
	}
	if err := m.Create(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	if err := m.Start(context.Background(), task.ID); err != nil {
		t.Fatal(err)
	}

	waitCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	completed, err := m.Wait(waitCtx, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if completed.FinalText != "specialist answer" {
		t.Fatalf("expected runner final text, got %q", completed.FinalText)
	}

	reports, err := s.Reports().ListBySession(context.Background(), task.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if len(reports) != 1 || reports[0].Summary != "specialist answer" {
		t.Fatalf("unexpected report summary: %+v", reports)
	}
}

func TestRetryDatabaseBusyRetriesLockedDatabaseErrors(t *testing.T) {
	attempts := 0
	err := retryDatabaseBusy(context.Background(), func(ctx context.Context) error {
		attempts++
		if attempts < 3 {
			return errors.New("database is locked (5) (SQLITE_BUSY)")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("expected retry to succeed, got %v", err)
	}
	if attempts != 3 {
		t.Fatalf("expected 3 attempts, got %d", attempts)
	}
}

type finalTextRunner struct {
	store contracts.Store
}

func (r finalTextRunner) Run(ctx context.Context, task contracts.Task, sink OutputSink) error {
	task.FinalText = "specialist answer"
	task.UpdatedAt = time.Now().UTC()
	return r.store.Tasks().Update(ctx, task)
}
