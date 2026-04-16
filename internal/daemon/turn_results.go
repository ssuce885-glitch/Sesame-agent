package daemon

import (
	"context"
	"errors"

	"go-agent/internal/session"
	"go-agent/internal/store/sqlite"
	"go-agent/internal/stream"
	"go-agent/internal/types"
)

type turnResultEventSink interface {
	Emit(ctx context.Context, event types.Event) error
}

type turnResultFallbackSink struct {
	store     *sqlite.Store
	eventSink turnResultEventSink
}

func newTurnResultFallbackSink(store *sqlite.Store, bus *stream.Bus) session.TurnResultSink {
	if store == nil || bus == nil {
		return turnResultFallbackSink{}
	}
	return turnResultFallbackSink{
		store:     store,
		eventSink: storeAndBusSink{store: store, bus: bus},
	}
}

func (s turnResultFallbackSink) HandleTurnResult(ctx context.Context, sess types.Session, turnID string, err error) {
	if err == nil || s.store == nil || s.eventSink == nil {
		return
	}

	turn, ok, getErr := s.store.GetTurn(ctx, turnID)
	if getErr != nil || !ok {
		return
	}
	if isTurnTerminal(turn.State) {
		return
	}

	eventType := types.EventTurnFailed
	payload := any(types.TurnFailedPayload{Message: err.Error()})
	if errors.Is(err, context.Canceled) {
		if turn.Kind == types.TurnKindChildReportBatch {
			_ = s.store.RequeueClaimedChildReportsForTurn(ctx, turnID)
		}
		eventType = types.EventTurnInterrupted
		payload = map[string]string{"reason": "run_context_canceled"}
	}

	event, eventErr := types.NewEvent(sess.ID, turnID, eventType, payload)
	if eventErr != nil {
		return
	}
	_ = s.eventSink.Emit(ctx, event)
}

func isTurnTerminal(state types.TurnState) bool {
	return state == types.TurnStateCompleted || state == types.TurnStateFailed || state == types.TurnStateInterrupted
}
