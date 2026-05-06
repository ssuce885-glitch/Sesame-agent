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

	otherWorkspaceMemory := contracts.Memory{
		ID:            "memory-other-workspace",
		WorkspaceRoot: "/other-workspace",
		Kind:          "note",
		Content:       "other workspace context",
		Owner:         "workspace",
		Visibility:    "workspace",
		Confidence:    1,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := s.Memories().Create(ctx, otherWorkspaceMemory); err != nil {
		t.Fatalf("Create other workspace memory: %v", err)
	}
	_, err = tool.Execute(ctx, contracts.ToolCall{
		Args: map[string]any{"reference_id": otherWorkspaceMemory.ID},
	}, contracts.ExecContext{Store: s, WorkspaceRoot: "/workspace"})
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected cross-workspace memory to look not found, got %v", err)
	}
}

func TestMemoryToolsRespectRoleVisibility(t *testing.T) {
	ctx := context.Background()
	s, err := store.OpenInMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	now := time.Now().UTC()
	for _, memory := range []contracts.Memory{
		{
			ID:            "memory-reviewer",
			WorkspaceRoot: "/workspace",
			Kind:          "note",
			Content:       "reviewer scoped recall target",
			Owner:         "role:reviewer",
			Visibility:    "role_only",
			Confidence:    1,
			CreatedAt:     now,
			UpdatedAt:     now,
		},
		{
			ID:            "memory-other",
			WorkspaceRoot: "/workspace",
			Kind:          "note",
			Content:       "other scoped recall target",
			Owner:         "role:other",
			Visibility:    "role_only",
			Confidence:    1,
			CreatedAt:     now,
			UpdatedAt:     now,
		},
		{
			ID:            "memory-main",
			WorkspaceRoot: "/workspace",
			Kind:          "note",
			Content:       "main scoped recall target",
			Owner:         "main_session",
			Visibility:    "main_only",
			Confidence:    1,
			CreatedAt:     now,
			UpdatedAt:     now,
		},
	} {
		if err := s.Memories().Create(ctx, memory); err != nil {
			t.Fatalf("Create memory %s: %v", memory.ID, err)
		}
	}

	execCtx := contracts.ExecContext{
		WorkspaceRoot: "/workspace",
		Store:         s,
		RoleSpec:      &contracts.RoleSpec{ID: "reviewer"},
	}
	recallResult, err := NewRecallArchiveTool().Execute(ctx, contracts.ToolCall{
		Args: map[string]any{"query": "scoped recall target", "limit": 10},
	}, execCtx)
	if err != nil {
		t.Fatalf("recall Execute: %v", err)
	}
	if recallResult.IsError {
		t.Fatalf("recall returned error result: %+v", recallResult)
	}
	raw := recallResult.Output
	if !strings.Contains(raw, "memory-reviewer") {
		t.Fatalf("expected reviewer memory in recall output: %s", raw)
	}
	if strings.Contains(raw, "memory-other") || strings.Contains(raw, "memory-main") {
		t.Fatalf("recall leaked hidden memory: %s", raw)
	}

	_, err = NewLoadContextTool().Execute(ctx, contracts.ToolCall{
		Args: map[string]any{"reference_id": "memory-other"},
	}, execCtx)
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected hidden memory to look not found, got %v", err)
	}
}

func TestMemoryWriteDefaultsToRoleSharedForRoleTurns(t *testing.T) {
	ctx := context.Background()
	s, err := store.OpenInMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	result, err := NewMemoryWriteTool().Execute(ctx, contracts.ToolCall{
		Args: map[string]any{
			"kind":    "note",
			"content": "role generated memory",
		},
	}, contracts.ExecContext{
		WorkspaceRoot: "/workspace",
		Store:         s,
		RoleSpec:      &contracts.RoleSpec{ID: "reviewer"},
	})
	if err != nil {
		t.Fatalf("memory_write Execute: %v", err)
	}
	if result.IsError {
		t.Fatalf("memory_write returned error result: %+v", result)
	}
	memory, ok := result.Data.(contracts.Memory)
	if !ok {
		t.Fatalf("result data = %T, want contracts.Memory", result.Data)
	}
	if memory.Owner != "role:reviewer" || memory.Visibility != "role_shared" {
		t.Fatalf("memory scope = (%q, %q), want role owner and role_shared", memory.Owner, memory.Visibility)
	}
}
