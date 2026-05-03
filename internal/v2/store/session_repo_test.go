package store

import (
	"context"
	"testing"
	"time"

	"go-agent/internal/v2/contracts"
)

func TestSessionRepositoryCRUD(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	defer s.Close()

	now := time.Now().UTC()
	session := contracts.Session{
		ID:                "session-1",
		WorkspaceRoot:     "/workspace",
		SystemPrompt:      "system",
		PermissionProfile: "trusted",
		State:             "idle",
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	if err := s.Sessions().Create(ctx, session); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := s.Sessions().Get(ctx, session.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ID != session.ID || got.WorkspaceRoot != session.WorkspaceRoot || got.State != "idle" {
		t.Fatalf("unexpected session: %+v", got)
	}

	if err := s.Sessions().UpdateState(ctx, session.ID, "closed"); err != nil {
		t.Fatalf("UpdateState: %v", err)
	}
	got, err = s.Sessions().Get(ctx, session.ID)
	if err != nil {
		t.Fatalf("Get after UpdateState: %v", err)
	}
	if got.State != "closed" {
		t.Fatalf("expected closed state, got %q", got.State)
	}

	if err := s.Sessions().SetActiveTurn(ctx, session.ID, "turn-1"); err != nil {
		t.Fatalf("SetActiveTurn: %v", err)
	}
	got, err = s.Sessions().Get(ctx, session.ID)
	if err != nil {
		t.Fatalf("Get after SetActiveTurn: %v", err)
	}
	if got.ActiveTurnID != "turn-1" || got.State != "running" {
		t.Fatalf("unexpected active turn state: %+v", got)
	}

	list, err := s.Sessions().ListByWorkspace(ctx, "/workspace")
	if err != nil {
		t.Fatalf("ListByWorkspace: %v", err)
	}
	if len(list) != 1 || list[0].ID != session.ID {
		t.Fatalf("unexpected workspace sessions: %+v", list)
	}
}
