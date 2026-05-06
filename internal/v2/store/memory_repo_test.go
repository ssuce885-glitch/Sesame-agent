package store

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"go-agent/internal/v2/contracts"
)

func TestMemoryRepositoryCRUD(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	defer s.Close()

	now := time.Now().UTC()
	memory := contracts.Memory{
		ID:              "memory-1",
		WorkspaceRoot:   "/workspace",
		Kind:            "decision",
		Content:         "Use LIKE search.",
		Source:          "test",
		Owner:           "role:researcher",
		Visibility:      "role_shared",
		Confidence:      0.9,
		ImportanceScore: 0.7,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if err := s.Memories().Create(ctx, memory); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := s.Memories().Get(ctx, memory.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ID != memory.ID || got.Kind != "decision" || got.Owner != "role:researcher" || got.Visibility != "role_shared" || got.ImportanceScore != 0.7 {
		t.Fatalf("unexpected memory: %+v", got)
	}

	memory.Content = "Updated LIKE memory."
	memory.UpdatedAt = now.Add(time.Minute)
	if err := s.Memories().Create(ctx, memory); err != nil {
		t.Fatalf("Create update: %v", err)
	}
	got, err = s.Memories().Get(ctx, memory.ID)
	if err != nil {
		t.Fatalf("Get updated: %v", err)
	}
	if got.Content != memory.Content {
		t.Fatalf("expected updated content, got %+v", got)
	}

	results, err := s.Memories().Search(ctx, "/workspace", "decision", 10)
	if err != nil {
		t.Fatalf("Search kind: %v", err)
	}
	if len(results) != 1 || results[0].ID != memory.ID {
		t.Fatalf("unexpected search results: %+v", results)
	}

	if err := s.Memories().Delete(ctx, memory.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := s.Memories().Get(ctx, memory.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected sql.ErrNoRows, got %v", err)
	}
}

func TestMemoryRepositoryFTSSearchListAndCount(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	defer s.Close()

	now := time.Now().UTC()
	memories := []contracts.Memory{
		{
			ID:            "memory-1",
			WorkspaceRoot: "/workspace",
			Kind:          "note",
			Content:       "durable indexed search",
			Source:        "alpha",
			Confidence:    1,
			CreatedAt:     now,
			UpdatedAt:     now,
		},
		{
			ID:            "memory-2",
			WorkspaceRoot: "/workspace",
			Kind:          "note",
			Content:       "durable unrelated note",
			Source:        "beta",
			Confidence:    1,
			CreatedAt:     now,
			UpdatedAt:     now.Add(time.Minute),
		},
	}
	for _, memory := range memories {
		if err := s.Memories().Create(ctx, memory); err != nil {
			t.Fatalf("Create: %v", err)
		}
	}

	results, err := s.Memories().Search(ctx, "/workspace", "durable search", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 1 || results[0].ID != "memory-1" {
		t.Fatalf("unexpected FTS results: %+v", results)
	}

	count, err := s.Memories().Count(ctx, "/workspace")
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected count 2, got %d", count)
	}

	listed, err := s.Memories().ListByWorkspace(ctx, "/workspace", 1)
	if err != nil {
		t.Fatalf("ListByWorkspace: %v", err)
	}
	if len(listed) != 1 || listed[0].ID != "memory-2" {
		t.Fatalf("unexpected list results: %+v", listed)
	}
}
