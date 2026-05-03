package store

import (
	"context"
	"testing"
	"time"

	"go-agent/internal/v2/contracts"
)

func TestProjectStateRepoRoundTrip(t *testing.T) {
	s, err := OpenInMemory()
	if err != nil {
		t.Fatalf("OpenInMemory: %v", err)
	}
	defer s.Close()

	ctx := context.Background()
	createdAt := time.Date(2026, 5, 2, 10, 0, 0, 0, time.UTC)
	state := contracts.ProjectState{
		WorkspaceRoot:   "/workspace",
		Summary:         "# Current Goal\nBuild V2 context.",
		SourceSessionID: "session-1",
		SourceTurnID:    "turn-1",
		CreatedAt:       createdAt,
		UpdatedAt:       createdAt,
	}
	if err := s.ProjectStates().Upsert(ctx, state); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	got, ok, err := s.ProjectStates().Get(ctx, "/workspace")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !ok {
		t.Fatal("Get ok = false, want true")
	}
	if got.Summary != state.Summary {
		t.Fatalf("Summary = %q, want %q", got.Summary, state.Summary)
	}
	if got.SourceSessionID != "session-1" || got.SourceTurnID != "turn-1" {
		t.Fatalf("source = %q/%q, want session-1/turn-1", got.SourceSessionID, got.SourceTurnID)
	}

	state.Summary = "# Current Goal\nKeep V2 simple."
	state.SourceTurnID = "turn-2"
	state.UpdatedAt = createdAt.Add(time.Hour)
	if err := s.ProjectStates().Upsert(ctx, state); err != nil {
		t.Fatalf("second Upsert: %v", err)
	}
	got, ok, err = s.ProjectStates().Get(ctx, "/workspace")
	if err != nil {
		t.Fatalf("second Get: %v", err)
	}
	if !ok {
		t.Fatal("second Get ok = false, want true")
	}
	if got.Summary != state.Summary {
		t.Fatalf("updated Summary = %q, want %q", got.Summary, state.Summary)
	}
	if got.SourceTurnID != "turn-2" {
		t.Fatalf("SourceTurnID = %q, want turn-2", got.SourceTurnID)
	}
}
