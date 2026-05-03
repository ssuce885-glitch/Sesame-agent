package tools

import (
	"context"
	"strings"
	"testing"
	"time"

	"go-agent/internal/v2/contracts"
	"go-agent/internal/v2/store"
)

func TestLoadContextTool(t *testing.T) {
	ctx := context.Background()
	s, err := store.OpenInMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	now := time.Now().UTC()
	memory := contracts.Memory{
		ID:            "memory-1",
		WorkspaceRoot: "/workspace",
		Kind:          "note",
		Content:       "full conversation context",
		Confidence:    1,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := s.Memories().Create(ctx, memory); err != nil {
		t.Fatalf("Create: %v", err)
	}

	tool := NewLoadContextTool()
	result, err := tool.Execute(ctx, contracts.ToolCall{
		Args: map[string]any{"reference_id": memory.ID},
	}, contracts.ExecContext{Store: s, WorkspaceRoot: "/workspace"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Output != memory.Content {
		t.Fatalf("unexpected output: %q", result.Output)
	}
	if got, ok := result.Data.(contracts.Memory); !ok || got.ID != memory.ID {
		t.Fatalf("unexpected data: %+v", result.Data)
	}

	_, err = tool.Execute(ctx, contracts.ToolCall{
		Args: map[string]any{"reference_id": "missing"},
	}, contracts.ExecContext{Store: s, WorkspaceRoot: "/workspace"})
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected not found error, got %v", err)
	}
}
