package store

import (
	"context"
	"testing"
	"time"

	"go-agent/internal/v2/contracts"
)

func TestRoleRuntimeStateRepoRoundTrip(t *testing.T) {
	s, err := OpenInMemory()
	if err != nil {
		t.Fatalf("OpenInMemory: %v", err)
	}
	defer s.Close()

	ctx := context.Background()
	createdAt := time.Date(2026, 5, 6, 10, 0, 0, 0, time.UTC)
	reviewer := contracts.RoleRuntimeState{
		WorkspaceRoot:   "/workspace",
		RoleID:          "reviewer",
		Summary:         "# Role Runtime State: reviewer\n\n## Active Work\n- Review runtime chain.",
		SourceSessionID: "session-1",
		SourceTurnID:    "turn-1",
		CreatedAt:       createdAt,
		UpdatedAt:       createdAt,
	}
	writer := contracts.RoleRuntimeState{
		WorkspaceRoot:   "/workspace",
		RoleID:          "writer",
		Summary:         "# Role Runtime State: writer\n\n## Active Work\n- Draft role note.",
		SourceSessionID: "session-2",
		SourceTurnID:    "turn-2",
		CreatedAt:       createdAt,
		UpdatedAt:       createdAt,
	}
	if err := s.RoleRuntimeStates().Upsert(ctx, reviewer); err != nil {
		t.Fatalf("Upsert reviewer: %v", err)
	}
	if err := s.RoleRuntimeStates().Upsert(ctx, writer); err != nil {
		t.Fatalf("Upsert writer: %v", err)
	}

	gotReviewer, ok, err := s.RoleRuntimeStates().Get(ctx, "/workspace", "reviewer")
	if err != nil {
		t.Fatalf("Get reviewer: %v", err)
	}
	if !ok {
		t.Fatal("Get reviewer ok = false, want true")
	}
	if gotReviewer.Summary != reviewer.Summary {
		t.Fatalf("reviewer Summary = %q, want %q", gotReviewer.Summary, reviewer.Summary)
	}

	gotWriter, ok, err := s.RoleRuntimeStates().Get(ctx, "/workspace", "writer")
	if err != nil {
		t.Fatalf("Get writer: %v", err)
	}
	if !ok {
		t.Fatal("Get writer ok = false, want true")
	}
	if gotWriter.SourceTurnID != "turn-2" {
		t.Fatalf("writer SourceTurnID = %q, want turn-2", gotWriter.SourceTurnID)
	}

	reviewer.Summary = "# Role Runtime State: reviewer\n\n## Active Work\n- Add tests."
	reviewer.SourceTurnID = "turn-3"
	reviewer.UpdatedAt = createdAt.Add(time.Hour)
	if err := s.RoleRuntimeStates().Upsert(ctx, reviewer); err != nil {
		t.Fatalf("second Upsert reviewer: %v", err)
	}
	gotReviewer, ok, err = s.RoleRuntimeStates().Get(ctx, "/workspace", "reviewer")
	if err != nil {
		t.Fatalf("second Get reviewer: %v", err)
	}
	if !ok {
		t.Fatal("second Get reviewer ok = false, want true")
	}
	if gotReviewer.Summary != reviewer.Summary {
		t.Fatalf("updated reviewer Summary = %q, want %q", gotReviewer.Summary, reviewer.Summary)
	}
	if gotReviewer.SourceTurnID != "turn-3" {
		t.Fatalf("reviewer SourceTurnID = %q, want turn-3", gotReviewer.SourceTurnID)
	}

	if err := s.RoleRuntimeStates().Delete(ctx, "/workspace", "reviewer"); err != nil {
		t.Fatalf("Delete reviewer: %v", err)
	}
	if _, ok, err := s.RoleRuntimeStates().Get(ctx, "/workspace", "reviewer"); err != nil {
		t.Fatalf("Get deleted reviewer: %v", err)
	} else if ok {
		t.Fatal("Get deleted reviewer ok = true, want false")
	}
}
