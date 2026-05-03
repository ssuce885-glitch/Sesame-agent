package store

import (
	"context"
	"testing"
	"time"

	"go-agent/internal/v2/contracts"
)

func TestTurnRepositoryCRUD(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	defer s.Close()

	now := time.Now().UTC()
	turn := contracts.Turn{
		ID:          "turn-1",
		SessionID:   "session-1",
		Kind:        "user_message",
		State:       "created",
		UserMessage: "hello",
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := s.Turns().Create(ctx, turn); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := s.Turns().Get(ctx, turn.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ID != turn.ID || got.UserMessage != turn.UserMessage || got.State != "created" {
		t.Fatalf("unexpected turn: %+v", got)
	}

	if err := s.Turns().UpdateState(ctx, turn.ID, "running"); err != nil {
		t.Fatalf("UpdateState: %v", err)
	}
	got, err = s.Turns().Get(ctx, turn.ID)
	if err != nil {
		t.Fatalf("Get after UpdateState: %v", err)
	}
	if got.State != "running" {
		t.Fatalf("expected running state, got %q", got.State)
	}

	bySession, err := s.Turns().ListBySession(ctx, turn.SessionID)
	if err != nil {
		t.Fatalf("ListBySession: %v", err)
	}
	if len(bySession) != 1 || bySession[0].ID != turn.ID {
		t.Fatalf("unexpected session turns: %+v", bySession)
	}

	running, err := s.Turns().ListRunning(ctx)
	if err != nil {
		t.Fatalf("ListRunning: %v", err)
	}
	if len(running) != 1 || running[0].ID != turn.ID {
		t.Fatalf("unexpected running turns: %+v", running)
	}
}
