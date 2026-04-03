package session

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"go-agent/internal/types"
)

type fakeRunner struct {
	started atomic.Int32
	done    chan struct{}
}

func (r *fakeRunner) RunTurn(ctx context.Context, in RunInput) error {
	r.started.Add(1)
	select {
	case r.done <- struct{}{}:
	default:
	}
	<-ctx.Done()

	return ctx.Err()
}

func TestManagerInterruptsActiveTurnBeforeStartingNext(t *testing.T) {
	runner := &fakeRunner{done: make(chan struct{}, 1)}
	manager := NewManager(runner)

	session := types.Session{
		ID:            "sess_test",
		WorkspaceRoot: "D:/work/demo",
		State:         types.SessionStateIdle,
		CreatedAt:     time.Now().UTC(),
		UpdatedAt:     time.Now().UTC(),
	}
	manager.RegisterSession(session)

	if _, err := manager.SubmitTurn(context.Background(), session.ID, SubmitTurnInput{
		TurnID:       "turn_1",
		ClientTurnID: "client_1",
		Message:      "first",
	}); err != nil {
		t.Fatalf("SubmitTurn(first) error = %v", err)
	}
	<-runner.done

	if _, err := manager.SubmitTurn(context.Background(), session.ID, SubmitTurnInput{
		TurnID:       "turn_2",
		ClientTurnID: "client_2",
		Message:      "second",
	}); err != nil {
		t.Fatalf("SubmitTurn(second) error = %v", err)
	}

	state, ok := manager.GetRuntimeState(session.ID)
	if !ok {
		t.Fatal("session runtime state not found")
	}
	if state.ActiveTurnID != "turn_2" {
		t.Fatalf("ActiveTurnID = %q, want %q", state.ActiveTurnID, "turn_2")
	}
}
