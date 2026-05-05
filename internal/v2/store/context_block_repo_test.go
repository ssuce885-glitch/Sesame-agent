package store

import (
	"context"
	"testing"
	"time"

	"go-agent/internal/v2/contracts"
)

func TestContextBlockRepositoryCRUDAndFilters(t *testing.T) {
	s, err := OpenInMemory()
	if err != nil {
		t.Fatalf("OpenInMemory: %v", err)
	}
	defer s.Close()

	ctx := context.Background()
	now := time.Date(2026, 5, 3, 2, 0, 0, 0, time.UTC)
	expiresAt := now.Add(24 * time.Hour)
	block := contracts.ContextBlock{
		ID:              "ctx-1",
		WorkspaceRoot:   "/workspace",
		Type:            "decision",
		Owner:           "workspace",
		Visibility:      "global",
		SourceRef:       "message:1",
		Title:           "Context decision",
		Summary:         "Keep context blocks as an index layer.",
		Evidence:        "User asked for context governance.",
		Confidence:      0.8,
		ImportanceScore: 0.7,
		ExpiryPolicy:    "manual",
		ExpiresAt:       &expiresAt,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if err := s.ContextBlocks().Create(ctx, block); err != nil {
		t.Fatalf("create context block: %v", err)
	}

	got, err := s.ContextBlocks().Get(ctx, block.ID)
	if err != nil {
		t.Fatalf("get context block: %v", err)
	}
	if got.Title != block.Title || got.ExpiresAt == nil || !got.ExpiresAt.Equal(expiresAt) {
		t.Fatalf("got context block = %+v", got)
	}

	block.Summary = "Updated summary."
	block.ImportanceScore = 0.9
	block.UpdatedAt = now.Add(time.Hour)
	if err := s.ContextBlocks().Update(ctx, block); err != nil {
		t.Fatalf("update context block: %v", err)
	}

	list, err := s.ContextBlocks().ListByWorkspace(ctx, "/workspace", contracts.ContextBlockListOptions{
		Owner:      "workspace",
		Visibility: "global",
		Type:       "decision",
	})
	if err != nil {
		t.Fatalf("list context blocks: %v", err)
	}
	if len(list) != 1 || list[0].Summary != "Updated summary." {
		t.Fatalf("list = %+v", list)
	}

	if err := s.ContextBlocks().Delete(ctx, block.ID); err != nil {
		t.Fatalf("delete context block: %v", err)
	}
	list, err = s.ContextBlocks().ListByWorkspace(ctx, "/workspace", contracts.ContextBlockListOptions{})
	if err != nil {
		t.Fatalf("list after delete: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("list after delete = %+v", list)
	}
}
