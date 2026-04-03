package engine

import (
	"context"
	"errors"

	"go-agent/internal/model"
	"go-agent/internal/permissions"
	"go-agent/internal/tools"
	"go-agent/internal/types"
)

func runLoop(ctx context.Context, modelClient model.Client, registry *tools.Registry, permission *permissions.Engine, in Input) ([]types.Event, error) {
	if modelClient == nil {
		return nil, errors.New("model client is required")
	}
	if registry == nil {
		return nil, errors.New("tool registry is required")
	}
	if permission == nil {
		return nil, errors.New("permission engine is required")
	}

	var events []types.Event
	started, err := types.NewEvent(in.Session.ID, in.Turn.ID, types.EventTurnStarted, types.TurnStartedPayload{
		WorkspaceRoot: in.Session.WorkspaceRoot,
	})
	if err != nil {
		return nil, err
	}
	events = append(events, started)

	var toolResults []string
	for {
		resp, err := modelClient.Next(ctx, model.Request{
			UserMessage: in.Turn.UserMessage,
			ToolResults: toolResults,
		})
		if err != nil {
			return nil, err
		}

		events, err = appendAssistantDelta(events, in.Session.ID, in.Turn.ID, resp.AssistantText)
		if err != nil {
			return nil, err
		}

		if ShouldStop(len(resp.ToolCalls) > 0) {
			done, err := types.NewEvent(in.Session.ID, in.Turn.ID, types.EventTurnCompleted, map[string]string{"result": resp.AssistantText})
			if err != nil {
				return nil, err
			}
			events = append(events, done)
			return events, nil
		}

		for _, call := range resp.ToolCalls {
			if permission.Decide(call.Name) == permissions.DecisionDeny {
				return nil, context.Canceled
			}

			result, err := registry.Execute(ctx, tools.Call{Name: call.Name, Input: call.Input}, tools.ExecContext{
				WorkspaceRoot:    in.Session.WorkspaceRoot,
				PermissionEngine: permission,
			})
			if err != nil {
				return nil, err
			}

			toolResults = append(toolResults, result.Text)

			completed, err := types.NewEvent(in.Session.ID, in.Turn.ID, types.EventToolCompleted, map[string]string{
				"tool_name": call.Name,
				"result":    result.Text,
			})
			if err != nil {
				return nil, err
			}
			events = append(events, completed)
		}
	}
}
