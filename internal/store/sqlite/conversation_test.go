package sqlite

import (
	"context"
	"path/filepath"
	"testing"

	"go-agent/internal/model"
)

func TestMaxConversationPositionUsesHighestStoredPosition(t *testing.T) {
	ctx := context.Background()
	store, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	if pos, ok, err := store.MaxConversationPosition(ctx, "session_sparse"); err != nil || ok || pos != 0 {
		t.Fatalf("MaxConversationPosition(empty) = (%d, %v, %v), want (0, false, nil)", pos, ok, err)
	}

	if err := store.InsertConversationItemWithContextHead(ctx, "session_sparse", "head_a", "turn_a", 50, model.UserMessageItem("fifty")); err != nil {
		t.Fatalf("InsertConversationItemWithContextHead(50) error = %v", err)
	}
	if err := store.InsertConversationItemWithContextHead(ctx, "session_sparse", "head_a", "turn_a", 99, model.UserMessageItem("ninety-nine")); err != nil {
		t.Fatalf("InsertConversationItemWithContextHead(99) error = %v", err)
	}
	if err := store.InsertConversationItemWithContextHead(ctx, "other_session", "head_a", "turn_a", 200, model.UserMessageItem("other")); err != nil {
		t.Fatalf("InsertConversationItemWithContextHead(other session) error = %v", err)
	}

	pos, ok, err := store.MaxConversationPosition(ctx, "session_sparse")
	if err != nil {
		t.Fatalf("MaxConversationPosition() error = %v", err)
	}
	if !ok || pos != 99 {
		t.Fatalf("MaxConversationPosition() = (%d, %v), want (99, true)", pos, ok)
	}
}
