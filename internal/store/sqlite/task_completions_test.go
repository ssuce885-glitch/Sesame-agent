package sqlite

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"go-agent/internal/types"
)

func TestStoreClaimsPendingTaskCompletionsPerTurn(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "agentd.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	observedAt := time.Date(2026, 4, 10, 9, 0, 0, 0, time.UTC)
	if err := store.UpsertPendingTaskCompletion(context.Background(), types.PendingTaskCompletion{
		ID:            "task_child_1",
		SessionID:     "sess_child_completion",
		ParentTurnID:  "turn_parent",
		TaskID:        "task_child_1",
		TaskType:      "agent",
		ResultKind:    "assistant_text",
		ResultText:    "subtask final text",
		ResultPreview: "subtask final text",
		ObservedAt:    observedAt,
	}); err != nil {
		t.Fatalf("UpsertPendingTaskCompletion() error = %v", err)
	}

	pending, err := store.ListPendingTaskCompletions(context.Background(), "sess_child_completion")
	if err != nil {
		t.Fatalf("ListPendingTaskCompletions() error = %v", err)
	}
	if len(pending) != 1 {
		t.Fatalf("len(pending) = %d, want 1", len(pending))
	}
	if pending[0].InjectedTurnID != "" {
		t.Fatalf("InjectedTurnID = %q, want empty before claim", pending[0].InjectedTurnID)
	}

	claimed, err := store.ClaimPendingTaskCompletionsForTurn(context.Background(), "sess_child_completion", "turn_delivery")
	if err != nil {
		t.Fatalf("ClaimPendingTaskCompletionsForTurn() error = %v", err)
	}
	if len(claimed) != 1 {
		t.Fatalf("len(claimed) = %d, want 1", len(claimed))
	}
	if claimed[0].InjectedTurnID != "turn_delivery" {
		t.Fatalf("InjectedTurnID = %q, want turn_delivery", claimed[0].InjectedTurnID)
	}
	if claimed[0].InjectedAt.IsZero() {
		t.Fatal("InjectedAt = zero, want claim timestamp")
	}

	claimedAgain, err := store.ClaimPendingTaskCompletionsForTurn(context.Background(), "sess_child_completion", "turn_delivery")
	if err != nil {
		t.Fatalf("ClaimPendingTaskCompletionsForTurn(same turn) error = %v", err)
	}
	if len(claimedAgain) != 1 {
		t.Fatalf("len(claimedAgain) = %d, want 1", len(claimedAgain))
	}
	if claimedAgain[0].InjectedTurnID != "turn_delivery" {
		t.Fatalf("InjectedTurnID(same turn) = %q, want turn_delivery", claimedAgain[0].InjectedTurnID)
	}

	pendingAfter, err := store.ListPendingTaskCompletions(context.Background(), "sess_child_completion")
	if err != nil {
		t.Fatalf("ListPendingTaskCompletions(after claim) error = %v", err)
	}
	if len(pendingAfter) != 0 {
		t.Fatalf("len(pendingAfter) = %d, want 0 after claim", len(pendingAfter))
	}
}
