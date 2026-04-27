package daemon

import (
	"context"
	"errors"
	"testing"
	"time"

	"go-agent/internal/model"
	"go-agent/internal/reporting"
	"go-agent/internal/session"
	"go-agent/internal/types"
)

type recordingSessionRunner struct {
	started chan types.Turn
}

func (r recordingSessionRunner) RunTurn(_ context.Context, in session.RunInput) error {
	r.started <- in.Turn
	return nil
}

func TestRecoverRuntimeStateEnqueuesQueuedReportBatch(t *testing.T) {
	ctx := context.Background()
	store := newDaemonTestStore(t)
	workspaceRoot := "/tmp/workspace"
	sessionRow, _, _, err := store.EnsureRoleSession(ctx, workspaceRoot, types.SessionRoleMainParent)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, time.April, 24, 8, 0, 0, 0, time.UTC)
	report := types.ReportRecord{
		ID:            "report_1",
		WorkspaceRoot: workspaceRoot,
		SessionID:     sessionRow.ID,
		SourceKind:    types.ReportSourceTaskResult,
		SourceID:      "task_1",
		Envelope: types.ReportEnvelope{
			Source:  string(types.ReportSourceTaskResult),
			Status:  "completed",
			Title:   "task_1",
			Summary: "done",
		},
		ObservedAt: now,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if err := store.UpsertReport(ctx, report); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertReportDelivery(ctx, reporting.DeliveryFromReport(report, now)); err != nil {
		t.Fatal(err)
	}

	runner := recordingSessionRunner{started: make(chan types.Turn, 1)}
	manager := session.NewManager(runner)
	notifier := taskTerminalNotifier{
		store:   store,
		manager: manager,
		now:     func() time.Time { return now },
	}

	if err := recoverRuntimeState(ctx, store, manager, notifier); err != nil {
		t.Fatal(err)
	}

	select {
	case turn := <-runner.started:
		if turn.Kind != types.TurnKindReportBatch {
			t.Fatalf("started turn kind = %q, want report_batch", turn.Kind)
		}
		if turn.SessionID != sessionRow.ID {
			t.Fatalf("started turn session = %q, want %q", turn.SessionID, sessionRow.ID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for recovered child report batch turn")
	}
}

type fakeTurnResultStore struct {
	turn     types.Turn
	requeued int
}

func (s *fakeTurnResultStore) GetTurn(context.Context, string) (types.Turn, bool, error) {
	return s.turn, true, nil
}

func (s *fakeTurnResultStore) RequeueClaimedReportDeliveriesForTurn(context.Context, string) error {
	s.requeued++
	return nil
}

func (s *fakeTurnResultStore) ListSessionEvents(context.Context, string, int64) ([]types.Event, error) {
	return nil, nil
}

func (s *fakeTurnResultStore) AppendEventWithState(_ context.Context, event types.Event) (types.Event, error) {
	return event, nil
}

func (s *fakeTurnResultStore) FinalizeTurn(context.Context, *types.TurnUsage, []types.Event) ([]types.Event, error) {
	return nil, nil
}

func (s *fakeTurnResultStore) InsertConversationItemWithContextHead(context.Context, string, string, string, int, model.ConversationItem) error {
	return nil
}

func (s *fakeTurnResultStore) GetConversationItemIDByContextHeadAndPosition(context.Context, string, string, int) (int64, bool, error) {
	return 0, false, nil
}

func (s *fakeTurnResultStore) InsertConversationItem(context.Context, string, string, int, model.ConversationItem) error {
	return nil
}

func (s *fakeTurnResultStore) ListConversationTimelineItems(context.Context, string) ([]types.ConversationTimelineItem, error) {
	return nil, nil
}

type fakeTurnResultEventSink struct {
	events []types.Event
}

func (s *fakeTurnResultEventSink) Emit(_ context.Context, event types.Event) error {
	s.events = append(s.events, event)
	return nil
}

type fakeReportEnqueuer struct {
	sessionIDs []string
}

func (e *fakeReportEnqueuer) enqueueSyntheticReportTurn(_ context.Context, sessionID string) error {
	e.sessionIDs = append(e.sessionIDs, sessionID)
	return nil
}

func TestTurnResultFallbackRequeuesAndReenqueuesCanceledReportBatch(t *testing.T) {
	store := &fakeTurnResultStore{turn: types.Turn{
		ID:        "turn_child",
		SessionID: "sess_main",
		Kind:      types.TurnKindReportBatch,
		State:     types.TurnStateModelStreaming,
	}}
	sink := &fakeTurnResultEventSink{}
	enqueuer := &fakeReportEnqueuer{}
	fallback := turnResultFallbackSink{
		store:         store,
		eventSink:     sink,
		reportEnqueue: enqueuer,
	}

	fallback.HandleTurnResult(context.Background(), types.Session{ID: "sess_main"}, "turn_child", context.Canceled)

	if store.requeued != 1 {
		t.Fatalf("requeued = %d, want 1", store.requeued)
	}
	if len(enqueuer.sessionIDs) != 1 || enqueuer.sessionIDs[0] != "sess_main" {
		t.Fatalf("enqueued sessions = %#v, want sess_main", enqueuer.sessionIDs)
	}
	if len(sink.events) != 1 || sink.events[0].Type != types.EventTurnInterrupted {
		t.Fatalf("events = %#v, want one turn.interrupted", sink.events)
	}
}

func TestTurnResultFallbackDoesNotReenqueueNonCanceledReportError(t *testing.T) {
	store := &fakeTurnResultStore{turn: types.Turn{
		ID:        "turn_child",
		SessionID: "sess_main",
		Kind:      types.TurnKindReportBatch,
		State:     types.TurnStateModelStreaming,
	}}
	enqueuer := &fakeReportEnqueuer{}
	fallback := turnResultFallbackSink{
		store:         store,
		eventSink:     &fakeTurnResultEventSink{},
		reportEnqueue: enqueuer,
	}

	fallback.HandleTurnResult(context.Background(), types.Session{ID: "sess_main"}, "turn_child", errors.New("model failed"))

	if store.requeued != 0 {
		t.Fatalf("requeued = %d, want 0", store.requeued)
	}
	if len(enqueuer.sessionIDs) != 0 {
		t.Fatalf("enqueued sessions = %#v, want none", enqueuer.sessionIDs)
	}
}
