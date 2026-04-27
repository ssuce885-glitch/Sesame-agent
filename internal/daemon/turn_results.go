package daemon

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"go-agent/internal/model"
	"go-agent/internal/session"
	"go-agent/internal/stream"
	"go-agent/internal/types"
)

type turnResultEventSink interface {
	Emit(ctx context.Context, event types.Event) error
}

type turnResultFallbackSink struct {
	store         turnResultStore
	eventSink     turnResultEventSink
	reportEnqueue reportTurnEnqueuer
}

type reportTurnEnqueuer interface {
	enqueueSyntheticReportTurn(context.Context, string) error
}

type turnResultStore interface {
	eventSinkStore
	GetTurn(context.Context, string) (types.Turn, bool, error)
	RequeueClaimedReportDeliveriesForTurn(context.Context, string) error
	ListSessionEvents(context.Context, string, int64) ([]types.Event, error)
	InsertConversationItemWithContextHead(context.Context, string, string, string, int, model.ConversationItem) error
	GetConversationItemIDByContextHeadAndPosition(context.Context, string, string, int) (int64, bool, error)
	InsertConversationItem(context.Context, string, string, int, model.ConversationItem) error
	ListConversationTimelineItems(context.Context, string) ([]types.ConversationTimelineItem, error)
}

func newTurnResultFallbackSink(store turnResultStore, bus *stream.Bus, reportEnqueue ...reportTurnEnqueuer) session.TurnResultSink {
	if store == nil || bus == nil {
		return turnResultFallbackSink{}
	}
	var enqueuer reportTurnEnqueuer
	if len(reportEnqueue) > 0 {
		enqueuer = reportEnqueue[0]
	}
	return turnResultFallbackSink{
		store:         store,
		eventSink:     storeAndBusSink{store: store, bus: bus},
		reportEnqueue: enqueuer,
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

	if !isTurnTerminal(turn.State) {
		eventType := types.EventTurnFailed
		payload := any(types.TurnFailedPayload{Message: err.Error()})
		if errors.Is(err, context.Canceled) {
			if turn.Kind == types.TurnKindReportBatch {
				_ = s.store.RequeueClaimedReportDeliveriesForTurn(ctx, turnID)
				if s.reportEnqueue != nil {
					_ = s.reportEnqueue.enqueueSyntheticReportTurn(ctx, sess.ID)
				}
			}
			eventType = types.EventTurnInterrupted
			payload = map[string]string{"reason": "run_context_canceled"}
		}

		event, eventErr := types.NewEvent(sess.ID, turnID, eventType, payload)
		if eventErr == nil {
			_ = s.eventSink.Emit(ctx, event)
		}
	}

	s.maybeBackfillParentReplyCommitted(ctx, sess, turn, err)
}

func isTurnTerminal(state types.TurnState) bool {
	return state == types.TurnStateCompleted || state == types.TurnStateFailed || state == types.TurnStateInterrupted
}

func (s turnResultFallbackSink) maybeBackfillParentReplyCommitted(ctx context.Context, sess types.Session, turn types.Turn, runErr error) {
	if errors.Is(runErr, context.Canceled) {
		return
	}

	alreadyCommitted, err := s.hasParentReplyCommitted(ctx, sess.ID, turn.ID)
	if err != nil || alreadyCommitted {
		return
	}

	fallbackText := buildParentFacingFailureSummary(runErr)
	itemID, err := s.persistFallbackReplyItem(ctx, sess.ID, turn, fallbackText)
	if err != nil {
		return
	}

	payload := types.ParentReplyCommittedPayload{
		WorkspaceRoot: strings.TrimSpace(sess.WorkspaceRoot),
		SessionID:     sess.ID,
		TurnID:        turn.ID,
		TurnKind:      turn.Kind,
		ItemID:        itemID,
		Text:          fallbackText,
		CreatedAt:     time.Now().UTC().Format(time.RFC3339),
	}
	event, eventErr := types.NewEvent(sess.ID, turn.ID, types.EventParentReplyCommitted, payload)
	if eventErr != nil {
		return
	}
	_ = s.eventSink.Emit(ctx, event)
}

func (s turnResultFallbackSink) hasParentReplyCommitted(ctx context.Context, sessionID, turnID string) (bool, error) {
	events, err := s.store.ListSessionEvents(ctx, sessionID, 0)
	if err != nil {
		return false, err
	}
	for _, event := range events {
		if event.Type != types.EventParentReplyCommitted {
			continue
		}
		if strings.TrimSpace(event.TurnID) == strings.TrimSpace(turnID) {
			return true, nil
		}
	}
	return false, nil
}

func (s turnResultFallbackSink) persistFallbackReplyItem(ctx context.Context, sessionID string, turn types.Turn, text string) (int64, error) {
	replyItem := model.ConversationItem{
		Kind: model.ConversationItemAssistantText,
		Text: text,
	}
	var lastErr error
	for attempts := 0; attempts < 3; attempts++ {
		nextPosition, err := s.nextConversationPosition(ctx, sessionID)
		if err != nil {
			return 0, err
		}

		if strings.TrimSpace(turn.ContextHeadID) != "" {
			err = s.store.InsertConversationItemWithContextHead(ctx, sessionID, turn.ContextHeadID, turn.ID, nextPosition, replyItem)
			if err != nil {
				if isConversationPositionConflict(err) {
					lastErr = err
					continue
				}
				return 0, err
			}
			itemID, ok, err := s.store.GetConversationItemIDByContextHeadAndPosition(ctx, sessionID, turn.ContextHeadID, nextPosition)
			if err != nil {
				return 0, err
			}
			if !ok {
				return 0, fmt.Errorf("fallback parent reply item lookup failed at position %d", nextPosition)
			}
			return itemID, nil
		}

		err = s.store.InsertConversationItem(ctx, sessionID, turn.ID, nextPosition, replyItem)
		if err != nil {
			if isConversationPositionConflict(err) {
				lastErr = err
				continue
			}
			return 0, err
		}
		itemID, ok, err := s.lookupConversationItemIDByPosition(ctx, sessionID, nextPosition)
		if err != nil {
			return 0, err
		}
		if !ok {
			return 0, fmt.Errorf("fallback parent reply item lookup failed at position %d", nextPosition)
		}
		return itemID, nil
	}
	if lastErr == nil {
		lastErr = errors.New("persist fallback parent reply item exceeded retry limit")
	}
	return 0, lastErr
}

func (s turnResultFallbackSink) nextConversationPosition(ctx context.Context, sessionID string) (int, error) {
	items, err := s.store.ListConversationTimelineItems(ctx, sessionID)
	if err != nil {
		return 0, err
	}
	nextPosition := 1
	for _, item := range items {
		if item.Position >= nextPosition {
			nextPosition = item.Position + 1
		}
	}
	return nextPosition, nil
}

func (s turnResultFallbackSink) lookupConversationItemIDByPosition(ctx context.Context, sessionID string, position int) (int64, bool, error) {
	items, err := s.store.ListConversationTimelineItems(ctx, sessionID)
	if err != nil {
		return 0, false, err
	}
	for _, item := range items {
		if item.Position == position {
			return item.ItemID, true, nil
		}
	}
	return 0, false, nil
}

func buildParentFacingFailureSummary(runErr error) string {
	message := strings.TrimSpace(runErr.Error())
	if message == "" {
		message = "unknown runtime error"
	}
	return fmt.Sprintf("I ran into an internal error before I could finish this turn: %s", message)
}

func isConversationPositionConflict(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "conversation_items.session_id, conversation_items.position")
}
