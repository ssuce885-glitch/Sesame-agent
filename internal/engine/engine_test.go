package engine

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"go-agent/internal/model"
	"go-agent/internal/permissions"
	"go-agent/internal/tools"
	"go-agent/internal/types"
)

func TestRunTurnLoopsThroughToolCallAndFinishes(t *testing.T) {
	root := t.TempDir()
	readme := filepath.Join(root, "README.md")
	if err := os.WriteFile(readme, []byte("project readme"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	fakeModel := model.NewFake([]model.Response{
		{
			AssistantText: "Reading file",
			ToolCalls: []model.ToolCall{
				{Name: "file_read", Input: map[string]any{"path": readme}},
			},
		},
		{
			AssistantText: "Finished after reading file",
		},
	})

	runner := New(fakeModel, tools.NewRegistry(), permissions.NewEngine())
	events, err := runner.RunTurn(context.Background(), Input{
		Session: types.Session{ID: "sess_1", WorkspaceRoot: root},
		Turn:    types.Turn{ID: "turn_1", UserMessage: "inspect the readme"},
	})
	if err != nil {
		t.Fatalf("RunTurn() error = %v", err)
	}
	if len(events) == 0 {
		t.Fatal("expected non-empty event list")
	}
	if events[len(events)-1].Type != types.EventTurnCompleted {
		t.Fatalf("last event type = %q, want %q", events[len(events)-1].Type, types.EventTurnCompleted)
	}
}
