package session

import (
	"context"
	"testing"
	"time"

	"go-agent/internal/types"
)

type blockingRunner struct {
	started chan types.Turn
	release chan struct{}
}

func (r *blockingRunner) RunTurn(ctx context.Context, in RunInput) error {
	r.started <- in.Turn
	if in.Turn.ID == "turn_user" {
		select {
		case <-r.release:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

func TestManagerRunsQueuedReportAfterActiveTurnCompletes(t *testing.T) {
	runner := &blockingRunner{
		started: make(chan types.Turn, 2),
		release: make(chan struct{}),
	}
	manager := NewManager(runner)
	session := types.Session{ID: "session_1"}
	manager.RegisterSession(session)

	if _, err := manager.SubmitTurn(context.Background(), session.ID, SubmitTurnInput{Turn: types.Turn{
		ID:        "turn_user",
		SessionID: session.ID,
		Kind:      types.TurnKindUserMessage,
	}}); err != nil {
		t.Fatalf("submit user turn: %v", err)
	}
	first := waitStartedTurn(t, runner.started)
	if first.ID != "turn_user" {
		t.Fatalf("first started turn = %q, want turn_user", first.ID)
	}

	if _, err := manager.SubmitTurn(context.Background(), session.ID, SubmitTurnInput{Turn: types.Turn{
		ID:        "turn_report",
		SessionID: session.ID,
		Kind:      types.TurnKindReportBatch,
	}}); err != nil {
		t.Fatalf("submit child report turn: %v", err)
	}
	state, ok := manager.GetRuntimeState(session.ID)
	if !ok {
		t.Fatalf("runtime state missing")
	}
	if state.ActiveTurnID != "turn_user" || state.QueuedReportBatches != 1 {
		t.Fatalf("state = %#v, want active user with one queued child report", state)
	}

	close(runner.release)
	second := waitStartedTurn(t, runner.started)
	if second.ID != "turn_report" {
		t.Fatalf("second started turn = %q, want turn_report", second.ID)
	}
}

func waitStartedTurn(t *testing.T, started <-chan types.Turn) types.Turn {
	t.Helper()
	select {
	case turn := <-started:
		return turn
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for turn start")
		return types.Turn{}
	}
}
