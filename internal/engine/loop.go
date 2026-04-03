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

	req := model.Request{
		UserMessage: in.Turn.UserMessage,
	}

	assistantStarted := false
	for {
		stream, errs := modelClient.Stream(ctx, req)
		messageEnded := false
		var toolCalls []model.ToolCallChunk

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
			case model.StreamEventToolCallStart, model.StreamEventToolCallDelta:
				continue
			case model.StreamEventToolCallEnd:
				toolCalls = append(toolCalls, event.ToolCall)
			case model.StreamEventMessageEnd:
				messageEnded = true
			case model.StreamEventUsage:
				continue
			default:
				err := fmt.Errorf("unsupported stream event kind: %s", event.Kind)
				if emitErr := emitFailed(err.Error()); emitErr != nil {
					return errors.Join(err, emitErr)
				}
				return err
			}
		}

		if errs != nil {
			err := <-errs
			if err != nil {
				if emitErr := emitFailed(err.Error()); emitErr != nil {
					return errors.Join(err, emitErr)
				}
				return err
			}
		}

		if len(toolCalls) == 0 {
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

		for _, call := range toolCalls {
			payload := struct {
				ToolCallID string `json:"tool_call_id"`
				ToolName   string `json:"tool_name"`
			}{
				ToolCallID: call.ID,
				ToolName:   call.Name,
			}
			if err := emit(types.EventToolStarted, payload); err != nil {
				return err
			}

			result, err := registry.Execute(ctx, tools.Call{
				Name:  call.Name,
				Input: call.Input,
			}, tools.ExecContext{
				WorkspaceRoot:    in.Session.WorkspaceRoot,
				PermissionEngine: permission,
			})
			if err != nil {
				if emitErr := emitFailed(err.Error()); emitErr != nil {
					return errors.Join(err, emitErr)
				}
				return err
			}

			if err := emit(types.EventToolCompleted, payload); err != nil {
				return err
			}

			req.ToolResults = append(req.ToolResults, model.ToolResult{
				ToolCallID: call.ID,
				ToolName:   call.Name,
				Content:    result.Text,
				IsError:    false,
			})
		}
	}
}
