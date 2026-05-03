package store

import (
	"context"
	"testing"
	"time"

	"go-agent/internal/v2/contracts"
)

func TestEventRepositoryCRUD(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	defer s.Close()

	now := time.Now().UTC()
	events := []contracts.Event{
		{ID: "event-1", SessionID: "session-1", TurnID: "turn-1", Type: "turn_started", Time: now, Payload: `{"state":"running"}`},
		{ID: "event-2", SessionID: "session-1", TurnID: "turn-1", Type: "turn_completed", Time: now, Payload: `{"state":"completed"}`},
	}
	if err := s.Events().Append(ctx, events); err != nil {
		t.Fatalf("Append: %v", err)
	}

	list, err := s.Events().List(ctx, "session-1", 0, 0)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 2 || list[0].Seq == 0 || list[0].ID != "event-1" {
		t.Fatalf("unexpected events: %+v", list)
	}

	after, err := s.Events().List(ctx, "session-1", list[0].Seq, 1)
	if err != nil {
		t.Fatalf("List after seq: %v", err)
	}
	if len(after) != 1 || after[0].ID != "event-2" {
		t.Fatalf("unexpected events after seq: %+v", after)
	}
}

func openTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := OpenInMemory()
	if err != nil {
		t.Fatalf("OpenInMemory: %v", err)
	}
	return s
}
