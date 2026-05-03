package store

import (
	"context"
	"testing"
	"time"

	"go-agent/internal/v2/contracts"
)

func TestMessageRepositoryCRUD(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	defer s.Close()

	newTime := time.Now().UTC()
	messages := []contracts.Message{
		{SessionID: "session-1", TurnID: "turn-1", Role: "user", Content: "one", Position: 1, CreatedAt: newTime},
		{SessionID: "session-1", TurnID: "turn-1", Role: "assistant", Content: "two", Position: 2, CreatedAt: newTime},
		{SessionID: "session-1", TurnID: "turn-2", Role: "user", Content: "three", Position: 3, CreatedAt: newTime},
	}
	if err := s.Messages().Append(ctx, messages); err != nil {
		t.Fatalf("Append: %v", err)
	}

	list, err := s.Messages().List(ctx, "session-1", contracts.MessageListOptions{})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 3 || list[0].Position != 1 || list[2].Position != 3 {
		t.Fatalf("unexpected messages: %+v", list)
	}

	maxPosition, err := s.Messages().MaxPosition(ctx, "session-1")
	if err != nil {
		t.Fatalf("MaxPosition: %v", err)
	}
	if maxPosition != 3 {
		t.Fatalf("expected max position 3, got %d", maxPosition)
	}
	emptyMaxPosition, err := s.Messages().MaxPosition(ctx, "session-empty")
	if err != nil {
		t.Fatalf("MaxPosition empty: %v", err)
	}
	if emptyMaxPosition != 0 {
		t.Fatalf("expected empty max position 0, got %d", emptyMaxPosition)
	}

	limited, err := s.Messages().List(ctx, "session-1", contracts.MessageListOptions{Limit: 2})
	if err != nil {
		t.Fatalf("List limit: %v", err)
	}
	if len(limited) != 2 || limited[1].Position != 2 {
		t.Fatalf("unexpected limited messages: %+v", limited)
	}

	snapshotID, err := s.Messages().SaveSnapshot(ctx, "session-1", "first two", 1, 2, "summary")
	if err != nil {
		t.Fatalf("SaveSnapshot: %v", err)
	}
	snapshot, err := s.Messages().LoadSnapshot(ctx, snapshotID)
	if err != nil {
		t.Fatalf("LoadSnapshot: %v", err)
	}
	if len(snapshot) != 2 || snapshot[0].Content != "one" || snapshot[1].Content != "two" {
		t.Fatalf("unexpected snapshot messages: %+v", snapshot)
	}

}
