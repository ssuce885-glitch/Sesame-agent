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

type observingRunner struct {
	started chan context.Context
}

func (r *observingRunner) RunTurn(ctx context.Context, in RunInput) error {
	select {
	case r.started <- ctx:
	default:
	}
	<-ctx.Done()
	return ctx.Err()
}

func TestSubmitTurnRunContextOutlivesRequestContext(t *testing.T) {
	runner := &observingRunner{started: make(chan context.Context, 1)}
	manager := NewManager(runner)

	session := types.Session{
		ID:            "sess_test",
		WorkspaceRoot: "D:/work/demo",
		State:         types.SessionStateIdle,
		CreatedAt:     time.Now().UTC(),
		UpdatedAt:     time.Now().UTC(),
	}
	manager.RegisterSession(session)

	reqCtx, cancelReq := context.WithCancel(context.Background())
	defer cancelReq()

	if _, err := manager.SubmitTurn(reqCtx, session.ID, SubmitTurnInput{
		TurnID:       "turn_1",
		ClientTurnID: "client_1",
		Message:      "first",
	}); err != nil {
		t.Fatalf("SubmitTurn() error = %v", err)
	}

	var runCtx context.Context
	select {
	case runCtx = <-runner.started:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timed out waiting for runner start")
	}

	cancelReq()

	select {
	case <-runCtx.Done():
		t.Fatal("run context canceled with request context")
	case <-time.After(50 * time.Millisecond):
	}

	manager.runtime[session.ID].cancel()

	select {
	case <-runCtx.Done():
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timed out waiting for runtime cancellation")
	}
}

type immediateRunner struct {
	done chan struct{}
}

func (r *immediateRunner) RunTurn(ctx context.Context, in RunInput) error {
	select {
	case r.done <- struct{}{}:
	default:
	}
	return nil
}

func TestManagerInterruptTurnCancelsMatchingActiveTurn(t *testing.T) {
	runner := &observingRunner{started: make(chan context.Context, 1)}
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
		TurnID:  "turn_1",
		Message: "first",
	}); err != nil {
		t.Fatalf("SubmitTurn() error = %v", err)
	}

	var runCtx context.Context
	select {
	case runCtx = <-runner.started:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timed out waiting for runner start")
	}

	if ok := manager.InterruptTurn(session.ID, "turn_1"); !ok {
		t.Fatal("InterruptTurn() = false, want true")
	}

	select {
	case <-runCtx.Done():
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timed out waiting for interrupted run context")
	}

	state, ok := manager.GetRuntimeState(session.ID)
	if !ok {
		t.Fatal("session runtime state not found")
	}
	if state.ActiveTurnID != "" {
		t.Fatalf("ActiveTurnID = %q, want empty", state.ActiveTurnID)
	}
}

func TestManagerClearsRuntimeStateWhenTurnFinishes(t *testing.T) {
	runner := &immediateRunner{done: make(chan struct{}, 1)}
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
		TurnID:  "turn_1",
		Message: "first",
	}); err != nil {
		t.Fatalf("SubmitTurn() error = %v", err)
	}

	select {
	case <-runner.done:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timed out waiting for runner completion")
	}

	deadline := time.Now().Add(100 * time.Millisecond)
	for time.Now().Before(deadline) {
		state, ok := manager.GetRuntimeState(session.ID)
		if !ok {
			t.Fatal("session runtime state not found")
		}
		if state.ActiveTurnID == "" {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}

	state, _ := manager.GetRuntimeState(session.ID)
	t.Fatalf("ActiveTurnID = %q, want empty after completion", state.ActiveTurnID)
}

func TestUpdateSessionReplacesPromptWithoutResettingRuntimeState(t *testing.T) {
	manager := NewManager(&fakeRunner{done: make(chan struct{}, 1)})
	now := time.Now().UTC()
	manager.RegisterSession(types.Session{
		ID:            "sess_test",
		WorkspaceRoot: "D:/work/demo",
		SystemPrompt:  "first prompt",
		State:         types.SessionStateIdle,
		CreatedAt:     now,
		UpdatedAt:     now,
	})

	manager.runtime["sess_test"].ActiveTurnID = "turn_live"

	ok := manager.UpdateSession(types.Session{
		ID:            "sess_test",
		WorkspaceRoot: "D:/work/demo",
		SystemPrompt:  "second prompt",
		State:         types.SessionStateIdle,
		CreatedAt:     now,
		UpdatedAt:     now.Add(time.Minute),
	})
	if !ok {
		t.Fatal("UpdateSession() ok = false, want true")
	}

	state, ok := manager.GetRuntimeState("sess_test")
	if !ok {
		t.Fatal("GetRuntimeState() ok = false, want true")
	}
	if state.ActiveTurnID != "turn_live" {
		t.Fatalf("ActiveTurnID = %q, want %q", state.ActiveTurnID, "turn_live")
	}
	if got := manager.sessions["sess_test"].SystemPrompt; got != "second prompt" {
		t.Fatalf("SystemPrompt = %q, want %q", got, "second prompt")
	}
}
