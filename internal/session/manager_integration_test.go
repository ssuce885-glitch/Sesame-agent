package session_test

import (
	"context"
	"testing"
	"time"

	contextstate "go-agent/internal/context"
	"go-agent/internal/engine"
	"go-agent/internal/model"
	"go-agent/internal/permissions"
	session "go-agent/internal/session"
	"go-agent/internal/store/sqlite"
	"go-agent/internal/tools"
	"go-agent/internal/types"
)

type engineRunner struct{ eng *engine.Engine }

func (r engineRunner) RunTurn(ctx context.Context, in session.RunInput) error {
	return r.eng.RunTurn(ctx, engine.Input{
		Session:             in.Session,
		SessionRole:         types.SessionRoleMainParent,
		Turn:                in.Turn,
		TaskID:              in.TaskID,
		Sink:                noopSink{},
		ActivatedSkillNames: in.ActivatedSkillNames,
	})
}

type noopSink struct{}

func (noopSink) Emit(_ context.Context, _ types.Event) error { return nil }

func setupManagerIntegrationTest(t *testing.T) (*session.Manager, *sqlite.Store, *model.FakeStreaming) {
	t.Helper()
	store, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("sqlite.Open: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	fakeModel := model.NewFakeStreaming([][]model.StreamEvent{
		{
			{Kind: model.StreamEventTextDelta, TextDelta: "Hello"},
			{Kind: model.StreamEventMessageEnd},
		},
	})
	ctxMgr := contextstate.NewManager(contextstate.Config{
		MaxRecentItems:          100,
		MaxEstimatedTokens:      200000,
		ModelContextWindow:      200000,
		CompactionThreshold:     150000,
		MaxCompactionBatchItems: 500,
	})
	eng := engine.NewWithRuntime(
		fakeModel,
		tools.NewRegistry(),
		permissions.NewEngine("trusted_local"),
		store,
		ctxMgr,
		nil,
		nil,
		engine.RuntimeMetadata{Provider: "fake", Model: "fake"},
		10,
	)

	mgr := session.NewManager(engineRunner{eng})
	return mgr, store, fakeModel
}

func createSessionAndTurn(t *testing.T, store *sqlite.Store, workspaceRoot string) (types.Session, types.Turn) {
	t.Helper()
	ctx := context.Background()
	session, head, _, err := store.EnsureRoleSession(ctx, workspaceRoot, types.SessionRoleMainParent)
	if err != nil {
		t.Fatalf("EnsureRoleSession: %v", err)
	}
	now := time.Now().UTC()
	turn := types.Turn{
		ID:            types.NewID("turn"),
		SessionID:     session.ID,
		ContextHeadID: head.ID,
		Kind:          types.TurnKindUserMessage,
		State:         types.TurnStateCreated,
		UserMessage:   "Hello",
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if err := store.InsertTurn(ctx, turn); err != nil {
		t.Fatalf("InsertTurn: %v", err)
	}
	return session, turn
}

type blockingRunner struct {
	started chan session.RunInput
	release chan struct{}
	runErr  error
}

func (r *blockingRunner) RunTurn(ctx context.Context, in session.RunInput) error {
	r.started <- in
	select {
	case <-r.release:
		return r.runErr
	case <-ctx.Done():
		return ctx.Err()
	}
}

func TestIntegrationManagerSubmitAndCompleteTurn(t *testing.T) {
	mgr, store, fakeModel := setupManagerIntegrationTest(t)
	sess, turn := createSessionAndTurn(t, store, t.TempDir())
	mgr.RegisterSession(sess)

	done := make(chan error, 1)
	turnID, err := mgr.SubmitTurn(context.Background(), sess.ID, session.SubmitTurnInput{
		Turn: turn,
		Run:  session.RunMetadata{Done: done},
	})
	if err != nil {
		t.Fatalf("SubmitTurn: %v", err)
	}
	if turnID != turn.ID {
		t.Fatalf("turnID = %q, want %q", turnID, turn.ID)
	}

	select {
	case runErr := <-done:
		if runErr != nil {
			t.Errorf("RunTurn error: %v", runErr)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("turn did not complete in time")
	}

	if got := fakeModel.RequestCount(); got != 1 {
		t.Fatalf("RequestCount = %d, want 1", got)
	}

	state, ok := mgr.GetRuntimeState(sess.ID)
	if !ok {
		t.Fatalf("runtime state missing")
	}
	if state.ActiveTurnID != "" || state.ActiveTurnKind != "" {
		t.Fatalf("state = %#v, want idle runtime state", state)
	}
}

func TestIntegrationManagerQueuesTurns(t *testing.T) {
	ctx := context.Background()
	r := &blockingRunner{
		started: make(chan session.RunInput, 2),
		release: make(chan struct{}),
	}
	mgr := session.NewManager(r)
	sess := types.Session{ID: "session-1", WorkspaceRoot: t.TempDir()}
	mgr.RegisterSession(sess)

	turn1 := types.Turn{ID: "turn-1", SessionID: sess.ID, Kind: types.TurnKindUserMessage, UserMessage: "first"}
	done1 := make(chan error, 1)
	if _, err := mgr.SubmitTurn(ctx, sess.ID, session.SubmitTurnInput{Turn: turn1, Run: session.RunMetadata{Done: done1}}); err != nil {
		t.Fatalf("SubmitTurn(turn1): %v", err)
	}
	started1 := waitStartedRunInput(t, r.started)
	if started1.Turn.ID != turn1.ID {
		t.Fatalf("first started turn = %q, want %q", started1.Turn.ID, turn1.ID)
	}

	state := requireRuntimeState(t, mgr, sess.ID)
	if state.ActiveTurnID != turn1.ID {
		t.Fatalf("state.ActiveTurnID = %q, want %q", state.ActiveTurnID, turn1.ID)
	}

	turn2 := types.Turn{ID: "turn-2", SessionID: sess.ID, Kind: types.TurnKindUserMessage, UserMessage: "second"}
	done2 := make(chan error)
	if _, err := mgr.SubmitTurn(ctx, sess.ID, session.SubmitTurnInput{Turn: turn2, Run: session.RunMetadata{Done: done2}}); err != nil {
		t.Fatalf("SubmitTurn(turn2): %v", err)
	}

	state = requireRuntimeState(t, mgr, sess.ID)
	if state.QueueDepth != 1 {
		t.Fatalf("state.QueueDepth = %d, want 1", state.QueueDepth)
	}

	close(r.release)

	if err := waitDoneError(t, done1); err != nil {
		t.Fatalf("done1 error = %v, want nil", err)
	}

	started2 := waitStartedRunInput(t, r.started)
	if started2.Turn.ID != turn2.ID {
		t.Fatalf("second started turn = %q, want %q", started2.Turn.ID, turn2.ID)
	}

	state = requireRuntimeState(t, mgr, sess.ID)
	if state.ActiveTurnID != turn2.ID {
		t.Fatalf("state.ActiveTurnID = %q, want %q", state.ActiveTurnID, turn2.ID)
	}

	if err := waitDoneError(t, done2); err != nil {
		t.Fatalf("done2 error = %v, want nil", err)
	}
}

func TestIntegrationManagerInterruptsActiveTurn(t *testing.T) {
	ctx := context.Background()
	r := &blockingRunner{
		started: make(chan session.RunInput, 2),
		release: make(chan struct{}),
	}
	mgr := session.NewManager(r)
	sess := types.Session{ID: "session-1", WorkspaceRoot: t.TempDir()}
	mgr.RegisterSession(sess)

	turn1 := types.Turn{ID: "turn-1", SessionID: sess.ID, Kind: types.TurnKindUserMessage, UserMessage: "first"}
	done1 := make(chan error, 1)
	if _, err := mgr.SubmitTurn(ctx, sess.ID, session.SubmitTurnInput{Turn: turn1, Run: session.RunMetadata{Done: done1}}); err != nil {
		t.Fatalf("SubmitTurn(turn1): %v", err)
	}
	started1 := waitStartedRunInput(t, r.started)
	if started1.Turn.ID != turn1.ID {
		t.Fatalf("first started turn = %q, want %q", started1.Turn.ID, turn1.ID)
	}

	state := requireRuntimeState(t, mgr, sess.ID)
	if state.ActiveTurnID != turn1.ID {
		t.Fatalf("state.ActiveTurnID = %q, want %q", state.ActiveTurnID, turn1.ID)
	}

	turn2 := types.Turn{ID: "turn-2", SessionID: sess.ID, Kind: types.TurnKindUserMessage, UserMessage: "second"}
	done2 := make(chan error, 1)
	if _, err := mgr.SubmitTurn(ctx, sess.ID, session.SubmitTurnInput{Turn: turn2, Run: session.RunMetadata{Done: done2}}); err != nil {
		t.Fatalf("SubmitTurn(turn2): %v", err)
	}

	state = requireRuntimeState(t, mgr, sess.ID)
	if state.QueueDepth != 1 {
		t.Fatalf("state.QueueDepth = %d, want 1", state.QueueDepth)
	}

	if !mgr.InterruptTurn(sess.ID, turn1.ID) {
		t.Fatal("InterruptTurn returned false")
	}

	if err := waitDoneError(t, done1); err != context.Canceled {
		t.Fatalf("done1 error = %v, want %v", err, context.Canceled)
	}

	started2 := waitStartedRunInput(t, r.started)
	if started2.Turn.ID != turn2.ID {
		t.Fatalf("second started turn = %q, want %q", started2.Turn.ID, turn2.ID)
	}

	state = requireRuntimeState(t, mgr, sess.ID)
	if state.ActiveTurnID != turn2.ID {
		t.Fatalf("state.ActiveTurnID = %q, want %q", state.ActiveTurnID, turn2.ID)
	}

	close(r.release)

	if err := waitDoneError(t, done2); err != nil {
		t.Fatalf("done2 error = %v, want nil", err)
	}
}

func TestIntegrationManagerCancelsQueuedTurn(t *testing.T) {
	ctx := context.Background()
	r := &blockingRunner{
		started: make(chan session.RunInput, 2),
		release: make(chan struct{}),
	}
	mgr := session.NewManager(r)
	sess := types.Session{ID: "session-1", WorkspaceRoot: t.TempDir()}
	mgr.RegisterSession(sess)

	turn1 := types.Turn{ID: "turn-1", SessionID: sess.ID, Kind: types.TurnKindUserMessage, UserMessage: "first"}
	done1 := make(chan error, 1)
	if _, err := mgr.SubmitTurn(ctx, sess.ID, session.SubmitTurnInput{Turn: turn1, Run: session.RunMetadata{Done: done1}}); err != nil {
		t.Fatalf("SubmitTurn(turn1): %v", err)
	}
	started1 := waitStartedRunInput(t, r.started)
	if started1.Turn.ID != turn1.ID {
		t.Fatalf("first started turn = %q, want %q", started1.Turn.ID, turn1.ID)
	}

	turn2 := types.Turn{ID: "turn-2", SessionID: sess.ID, Kind: types.TurnKindUserMessage, UserMessage: "second"}
	done2 := make(chan error, 1)
	if _, err := mgr.SubmitTurn(ctx, sess.ID, session.SubmitTurnInput{Turn: turn2, Run: session.RunMetadata{Done: done2}}); err != nil {
		t.Fatalf("SubmitTurn(turn2): %v", err)
	}

	if !mgr.CancelTurn(sess.ID, turn2.ID) {
		t.Fatal("CancelTurn returned false")
	}

	state := requireRuntimeState(t, mgr, sess.ID)
	if state.QueueDepth != 0 {
		t.Fatalf("state.QueueDepth = %d, want 0", state.QueueDepth)
	}

	if err := waitDoneError(t, done2); err != context.Canceled {
		t.Fatalf("done2 error = %v, want %v", err, context.Canceled)
	}

	close(r.release)

	if err := waitDoneError(t, done1); err != nil {
		t.Fatalf("done1 error = %v, want nil", err)
	}
}

func TestIntegrationManagerSessionLifecycle(t *testing.T) {
	mgr := session.NewManager(&blockingRunner{
		started: make(chan session.RunInput, 1),
		release: make(chan struct{}),
	})
	sess := types.Session{ID: "session-1", WorkspaceRoot: t.TempDir()}
	mgr.RegisterSession(sess)

	state, ok := mgr.GetRuntimeState(sess.ID)
	if !ok {
		t.Fatalf("runtime state missing")
	}
	if state.ActiveTurnID != "" {
		t.Fatalf("state.ActiveTurnID = %q, want empty", state.ActiveTurnID)
	}

	payload, ok := mgr.QueuePayload(sess.ID)
	if !ok {
		t.Fatalf("queue payload missing")
	}
	if payload.ActiveTurnID != "" || payload.QueueDepth != 0 {
		t.Fatalf("payload = %#v, want idle queue payload", payload)
	}

	if _, ok := mgr.GetRuntimeState("missing-session"); ok {
		t.Fatal("GetRuntimeState returned ok for missing session")
	}
}

func TestIntegrationManagerSubmitTurnUnregisteredSession(t *testing.T) {
	mgr := session.NewManager(&blockingRunner{
		started: make(chan session.RunInput, 1),
		release: make(chan struct{}),
	})
	turn := types.Turn{ID: "turn-1", SessionID: "no-such-session", Kind: types.TurnKindUserMessage, UserMessage: "hello"}
	_, err := mgr.SubmitTurn(context.Background(), "no-such-session", session.SubmitTurnInput{Turn: turn})
	if err == nil {
		t.Fatal("SubmitTurn should fail for unregistered session")
	}
}

func TestIntegrationManagerInterruptCancelWrongIDs(t *testing.T) {
	r := &blockingRunner{
		started: make(chan session.RunInput, 1),
		release: make(chan struct{}),
	}
	mgr := session.NewManager(r)
	sess := types.Session{ID: "session-1", WorkspaceRoot: t.TempDir()}
	mgr.RegisterSession(sess)

	if mgr.InterruptTurn(sess.ID, "nonexistent-turn") {
		t.Fatal("InterruptTurn should return false for nonexistent turn")
	}
	if mgr.CancelTurn(sess.ID, "nonexistent-turn") {
		t.Fatal("CancelTurn should return false for nonexistent turn")
	}
	if mgr.InterruptTurn("nonexistent-session", "any-turn") {
		t.Fatal("InterruptTurn should return false for nonexistent session")
	}
	if mgr.CancelTurn("nonexistent-session", "any-turn") {
		t.Fatal("CancelTurn should return false for nonexistent session")
	}
}

func TestIntegrationManagerPreemptsReportBatchWithUserMessage(t *testing.T) {
	ctx := context.Background()
	r := &blockingRunner{
		started: make(chan session.RunInput, 2),
		release: make(chan struct{}),
	}
	mgr := session.NewManager(r)
	sess := types.Session{ID: "session-1", WorkspaceRoot: t.TempDir()}
	mgr.RegisterSession(sess)

	turn1 := types.Turn{ID: "rb-1", SessionID: sess.ID, Kind: types.TurnKindReportBatch, UserMessage: "report"}
	done1 := make(chan error, 1)
	if _, err := mgr.SubmitTurn(ctx, sess.ID, session.SubmitTurnInput{Turn: turn1, Run: session.RunMetadata{Done: done1}}); err != nil {
		t.Fatalf("SubmitTurn(turn1): %v", err)
	}
	started1 := waitStartedRunInput(t, r.started)
	if started1.Turn.ID != turn1.ID {
		t.Fatalf("first started turn = %q, want %q", started1.Turn.ID, turn1.ID)
	}

	state := requireRuntimeState(t, mgr, sess.ID)
	if state.ActiveTurnKind != types.TurnKindReportBatch {
		t.Fatalf("state.ActiveTurnKind = %q, want %q", state.ActiveTurnKind, types.TurnKindReportBatch)
	}

	turn2 := types.Turn{ID: "msg-1", SessionID: sess.ID, Kind: types.TurnKindUserMessage, UserMessage: "user msg"}
	done2 := make(chan error, 1)
	if _, err := mgr.SubmitTurn(ctx, sess.ID, session.SubmitTurnInput{Turn: turn2, Run: session.RunMetadata{Done: done2}}); err != nil {
		t.Fatalf("SubmitTurn(turn2): %v", err)
	}

	if err := waitDoneError(t, done1); err != context.Canceled {
		t.Fatalf("done1 error = %v, want %v", err, context.Canceled)
	}

	started2 := waitStartedRunInput(t, r.started)
	if started2.Turn.ID != turn2.ID {
		t.Fatalf("second started turn = %q, want %q", started2.Turn.ID, turn2.ID)
	}

	state = requireRuntimeState(t, mgr, sess.ID)
	if state.ActiveTurnID != turn2.ID || state.ActiveTurnKind != types.TurnKindUserMessage {
		t.Fatalf("state = %#v, want active user message turn", state)
	}

	close(r.release)

	if err := waitDoneError(t, done2); err != nil {
		t.Fatalf("done2 error = %v, want nil", err)
	}
}

func waitStartedRunInput(t *testing.T, started <-chan session.RunInput) session.RunInput {
	t.Helper()
	select {
	case in := <-started:
		return in
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for turn start")
		return session.RunInput{}
	}
}

func waitDoneError(t *testing.T, done <-chan error) error {
	t.Helper()
	select {
	case err := <-done:
		return err
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for turn completion")
		return nil
	}
}

func requireRuntimeState(t *testing.T, mgr *session.Manager, sessionID string) session.RuntimeState {
	t.Helper()
	state, ok := mgr.GetRuntimeState(sessionID)
	if !ok {
		t.Fatalf("runtime state missing for session %q", sessionID)
	}
	return state
}
