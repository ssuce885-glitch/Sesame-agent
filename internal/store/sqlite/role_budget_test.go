package sqlite

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"go-agent/internal/types"
)

func TestTurnCostRoundTrip(t *testing.T) {
	ctx := context.Background()
	store, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	createdAt := time.Date(2026, time.April, 28, 12, 0, 0, 0, time.UTC)
	cost := types.TurnCost{
		TurnID:       "turn_1",
		SessionID:    "session_1",
		OwnerRoleID:  "analyst",
		InputTokens:  100,
		OutputTokens: 20,
		CreatedAt:    createdAt,
	}
	if err := store.UpsertTurnCost(ctx, cost); err != nil {
		t.Fatalf("UpsertTurnCost() error = %v", err)
	}

	got, ok, err := store.GetTurnCost(ctx, "turn_1")
	if err != nil {
		t.Fatalf("GetTurnCost() error = %v", err)
	}
	if !ok {
		t.Fatal("GetTurnCost() found no row")
	}
	if got.ID != "turn_cost_turn_1" || got.OwnerRoleID != "analyst" || got.InputTokens != 100 || got.OutputTokens != 20 || !got.CreatedAt.Equal(createdAt) {
		t.Fatalf("TurnCost = %#v", got)
	}

	items, err := store.ListTurnCostsBySession(ctx, "session_1")
	if err != nil {
		t.Fatalf("ListTurnCostsBySession() error = %v", err)
	}
	if len(items) != 1 || items[0].TurnID != "turn_1" {
		t.Fatalf("items = %#v", items)
	}
}
