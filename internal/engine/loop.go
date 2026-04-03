package engine

import (
	"context"
	"errors"
	"fmt"

	"go-agent/internal/model"
	"go-agent/internal/permissions"
	"go-agent/internal/tools"
	"go-agent/internal/types"
)

func runLoop(ctx context.Context, modelClient model.StreamingClient, registry *tools.Registry, permission *permissions.Engine, in Input) error {
	if modelClient == nil {
		return errors.New("model client is required")
	}
	if registry == nil {
		return errors.New("tool registry is required")
	}
	if permission == nil {
		return errors.New("permission engine is required")
	}
	if in.Sink == nil {
		return errors.New("event sink is required")
	}

	emit := func(eventType string, payload any) error {
		event, err := types.NewEvent(in.Session.ID, in.Turn.ID, eventType, payload)
		if err != nil {
			return err
		}
		return in.Sink.Emit(ctx, event)
	}
	emitFailed := func(message string) error {
		return emit(types.EventTurnFailed, types.TurnFailedPayload{Message: message})
	}

	if err := emit(types.EventTurnStarted, types.TurnStartedPayload{
		WorkspaceRoot: in.Session.WorkspaceRoot,
	}); err != nil {
		return err
	}

	stream, errs := modelClient.Stream(ctx, model.Request{
		UserMessage: in.Turn.UserMessage,
	})

	assistantStarted := false
	messageEnded := false
	for event := range stream {
		switch event.Kind {
		case model.StreamEventTextDelta:
			if !assistantStarted {
				if err := emit(types.EventAssistantStarted, struct{}{}); err != nil {
					return err
				}
				assistantStarted = true
			}
			if err := emit(types.EventAssistantDelta, types.AssistantDeltaPayload{Text: event.TextDelta}); err != nil {
				return err
			}
		case model.StreamEventMessageEnd:
			messageEnded = true
		case model.StreamEventUsage:
			continue
		case model.StreamEventToolCallStart, model.StreamEventToolCallDelta, model.StreamEventToolCallEnd:
			err := fmt.Errorf("tool call streaming events are not supported yet: %s", event.Kind)
			if emitErr := emitFailed(err.Error()); emitErr != nil {
				return errors.Join(err, emitErr)
			}
			return err
		default:
			err := fmt.Errorf("unsupported stream event kind: %s", event.Kind)
			if emitErr := emitFailed(err.Error()); emitErr != nil {
				return errors.Join(err, emitErr)
			}
			return err
		}
	}

	if errs == nil {
		return nil
	}

	err := <-errs
	if err != nil {
		if emitErr := emitFailed(err.Error()); emitErr != nil {
			return errors.Join(err, emitErr)
		}
		return err
	}

	if messageEnded {
		if err := emit(types.EventAssistantCompleted, struct{}{}); err != nil {
			return err
		}
		if err := emit(types.EventTurnCompleted, struct{}{}); err != nil {
			return err
		}
	}

	return nil
}
