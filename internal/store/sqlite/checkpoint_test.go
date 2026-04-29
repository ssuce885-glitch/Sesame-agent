package sqlite

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"go-agent/internal/types"
)

func TestTurnCheckpointRoundTripAndCleanup(t *testing.T) {
	ctx := context.Background()
	store, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	createdAt := time.Date(2026, time.April, 28, 12, 0, 0, 0, time.UTC)
	pre := types.TurnCheckpoint{
		ID:            "chkpt_pre",
		TurnID:        "turn_1",
		SessionID:     "sess_1",
		Sequence:      0,
		State:         types.TurnCheckpointStatePreToolBatch,
		ToolCallIDs:   []string{"call_1", "call_2"},
		ToolCallNames: []string{"tool_a", "tool_b"},
		NextPosition:  4,
		CreatedAt:     createdAt,
	}
	post := types.TurnCheckpoint{
		ID:                 "chkpt_post",
		TurnID:             "turn_1",
		SessionID:          "sess_1",
		Sequence:           1,
		State:              types.TurnCheckpointStatePostToolBatch,
		ToolCallIDs:        []string{"call_2"},
		ToolCallNames:      []string{"tool_b"},
		NextPosition:       6,
		CompletedToolIDs:   []string{"call_1", "call_2"},
		ToolResultsJSON:    `[{"ToolCallID":"call_1"}]`,
		AssistantItemsJSON: `[{"Kind":"tool_call"}]`,
		CreatedAt:          createdAt.Add(time.Second),
	}
	if err := store.InsertTurnCheckpoint(ctx, pre); err != nil {
		t.Fatalf("InsertTurnCheckpoint(pre) error = %v", err)
	}
	if err := store.InsertTurnCheckpoint(ctx, post); err != nil {
		t.Fatalf("InsertTurnCheckpoint(post) error = %v", err)
	}

	got, ok, err := store.GetLatestTurnCheckpoint(ctx, "turn_1")
	if err != nil {
		t.Fatalf("GetLatestTurnCheckpoint() error = %v", err)
	}
	if !ok {
		t.Fatal("GetLatestTurnCheckpoint() found no checkpoint")
	}
	if got.ID != post.ID || got.State != post.State || got.NextPosition != post.NextPosition {
		t.Fatalf("latest checkpoint = %#v, want post checkpoint", got)
	}
	if !reflect.DeepEqual(got.CompletedToolIDs, post.CompletedToolIDs) {
		t.Fatalf("CompletedToolIDs = %#v, want %#v", got.CompletedToolIDs, post.CompletedToolIDs)
	}

	deleted, err := store.DeleteTurnCheckpoints(ctx, "turn_1")
	if err != nil {
		t.Fatalf("DeleteTurnCheckpoints() error = %v", err)
	}
	if deleted != 2 {
		t.Fatalf("deleted checkpoints = %d, want 2", deleted)
	}
	if _, ok, err := store.GetLatestTurnCheckpoint(ctx, "turn_1"); err != nil || ok {
		t.Fatalf("GetLatestTurnCheckpoint(after delete) = (_, %v, %v), want (_, false, nil)", ok, err)
	}
}

func TestListInterruptedTurnsWithCheckpointsOnlyReturnsRunningTurns(t *testing.T) {
	ctx := context.Background()
	store, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	now := time.Date(2026, time.April, 28, 12, 0, 0, 0, time.UTC)
	if err := store.InsertSession(ctx, types.Session{
		ID:            "sess_1",
		WorkspaceRoot: t.TempDir(),
		State:         types.SessionStateRunning,
		ActiveTurnID:  "turn_running",
		CreatedAt:     now,
		UpdatedAt:     now,
	}); err != nil {
		t.Fatalf("InsertSession() error = %v", err)
	}
	for _, turn := range []types.Turn{
		{ID: "turn_running", SessionID: "sess_1", State: types.TurnStateToolRunning, UserMessage: "run", CreatedAt: now, UpdatedAt: now},
		{ID: "turn_done", SessionID: "sess_1", State: types.TurnStateCompleted, UserMessage: "done", CreatedAt: now, UpdatedAt: now},
	} {
		if err := store.InsertTurn(ctx, turn); err != nil {
			t.Fatalf("InsertTurn(%s) error = %v", turn.ID, err)
		}
		if err := store.InsertTurnCheckpoint(ctx, types.TurnCheckpoint{
			ID:        "chkpt_" + turn.ID,
			TurnID:    turn.ID,
			SessionID: turn.SessionID,
			Sequence:  0,
			State:     types.TurnCheckpointStatePostToolBatch,
			CreatedAt: now,
		}); err != nil {
			t.Fatalf("InsertTurnCheckpoint(%s) error = %v", turn.ID, err)
		}
	}

	got, err := store.ListInterruptedTurnsWithCheckpoints(ctx)
	if err != nil {
		t.Fatalf("ListInterruptedTurnsWithCheckpoints() error = %v", err)
	}
	want := []string{"turn_running"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("turns = %#v, want %#v", got, want)
	}
}
