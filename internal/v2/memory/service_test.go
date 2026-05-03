package memory

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"go-agent/internal/v2/contracts"
	"go-agent/internal/v2/store"
)

func TestServiceRememberRecallForget(t *testing.T) {
	ctx := context.Background()
	s, err := store.OpenInMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	service := NewService(s)
	m := contracts.Memory{
		ID:            "memory-1",
		WorkspaceRoot: "/workspace",
		Kind:          "decision",
		Content:       "Use LIKE search for v2 memory.",
		Source:        "test",
	}
	if err := service.Remember(ctx, m); err != nil {
		t.Fatalf("Remember: %v", err)
	}

	results, err := service.Recall(ctx, "/workspace", "LIKE", 10)
	if err != nil {
		t.Fatalf("Recall content: %v", err)
	}
	if len(results) != 1 || results[0].ID != m.ID {
		t.Fatalf("unexpected recall results: %+v", results)
	}

	results, err = service.Recall(ctx, "/workspace", "decision", 10)
	if err != nil {
		t.Fatalf("Recall kind: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected kind recall to match, got %+v", results)
	}

	m.Content = "Updated durable note."
	if err := service.Remember(ctx, m); err != nil {
		t.Fatalf("Remember update: %v", err)
	}
	got, err := s.Memories().Get(ctx, m.ID)
	if err != nil {
		t.Fatalf("Get updated memory: %v", err)
	}
	if got.Content != "Updated durable note." {
		t.Fatalf("memory was not updated: %+v", got)
	}

	if err := service.Forget(ctx, m.ID); err != nil {
		t.Fatalf("Forget: %v", err)
	}
	if _, err := s.Memories().Get(ctx, m.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected sql.ErrNoRows after forget, got %v", err)
	}
}

func TestServiceRecallSortsByScoreAndCleanup(t *testing.T) {
	ctx := context.Background()
	s, err := store.OpenInMemory()
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	service := NewService(s)
	now := time.Now().UTC()
	memories := []contracts.Memory{
		{
			ID:            "memory-high-old",
			WorkspaceRoot: "/workspace",
			Kind:          "note",
			Content:       "ranked cleanup target",
			Confidence:    1,
			CreatedAt:     now.Add(-15 * 24 * time.Hour),
			UpdatedAt:     now.Add(-15 * 24 * time.Hour),
		},
		{
			ID:            "memory-low-new",
			WorkspaceRoot: "/workspace",
			Kind:          "note",
			Content:       "ranked cleanup target",
			Confidence:    0.1,
			CreatedAt:     now,
			UpdatedAt:     now,
		},
		{
			ID:            "memory-low-old",
			WorkspaceRoot: "/workspace",
			Kind:          "note",
			Content:       "ranked cleanup target",
			Confidence:    0.1,
			CreatedAt:     now.Add(-60 * 24 * time.Hour),
			UpdatedAt:     now.Add(-60 * 24 * time.Hour),
		},
		{
			ID:            "memory-mid",
			WorkspaceRoot: "/workspace",
			Kind:          "note",
			Content:       "ranked cleanup target",
			Confidence:    0.6,
			CreatedAt:     now,
			UpdatedAt:     now,
		},
	}
	for _, memory := range memories {
		if err := s.Memories().Create(ctx, memory); err != nil {
			t.Fatalf("Create: %v", err)
		}
	}

	results, err := service.Recall(ctx, "/workspace", "ranked cleanup", 10)
	if err != nil {
		t.Fatalf("Recall: %v", err)
	}
	if len(results) != 4 {
		t.Fatalf("expected 4 results, got %+v", results)
	}
	if results[0].ID != "memory-high-old" {
		t.Fatalf("expected highest scored memory first, got %+v", results)
	}

	deleted, err := service.Cleanup(ctx, "/workspace", 3, 2)
	if err != nil {
		t.Fatalf("Cleanup: %v", err)
	}
	if deleted != 2 {
		t.Fatalf("expected 2 deleted, got %d", deleted)
	}
	count, err := s.Memories().Count(ctx, "/workspace")
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected 2 memories after cleanup, got %d", count)
	}
	for _, id := range []string{"memory-high-old", "memory-mid"} {
		if _, err := s.Memories().Get(ctx, id); err != nil {
			t.Fatalf("expected %s to be kept: %v", id, err)
		}
	}
}
