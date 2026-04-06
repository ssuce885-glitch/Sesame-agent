package tools

import (
	"context"
	"fmt"
	"strings"

	"go-agent/internal/runtimegraph"
	"go-agent/internal/types"
)

type enterPlanModeTool struct{}

func (enterPlanModeTool) Definition() Definition {
	return Definition{
		Name:        "enter_plan_mode",
		Description: "Create a new active plan for the current session.",
		InputSchema: objectSchema(map[string]any{
			"plan_file": map[string]any{
				"type":        "string",
				"description": "Path to the plan file for the new active plan.",
			},
		}, "plan_file"),
	}
}

func (enterPlanModeTool) IsConcurrencySafe() bool { return false }

func (enterPlanModeTool) Execute(ctx context.Context, call Call, execCtx ExecContext) (Result, error) {
	service, turnCtx, err := requireRuntimeGraph(execCtx)
	if err != nil {
		return Result{}, err
	}

	out, err := service.EnterPlanMode(ctx, turnCtx, runtimegraph.EnterPlanModeInput{
		SessionID: turnCtx.CurrentSessionID,
		TurnID:    turnCtx.CurrentTurnID,
		RunID:     turnCtx.CurrentRunID,
		PlanFile:  strings.TrimSpace(call.StringInput("plan_file")),
	})
	if err != nil {
		return Result{}, err
	}

	return Result{Text: mustJSON(out)}, nil
}

type exitPlanModeTool struct{}

func (exitPlanModeTool) Definition() Definition {
	schema := objectSchema(map[string]any{
		"state": map[string]any{
			"type":        "string",
			"description": "Final state for the current active plan.",
			"enum":        []string{"completed", "approved", "failed"},
		},
	})
	schema["required"] = []string{}
	return Definition{
		Name:        "exit_plan_mode",
		Description: "Finalize the current active plan for the session.",
		InputSchema: schema,
	}
}

func (exitPlanModeTool) IsConcurrencySafe() bool { return false }

func (exitPlanModeTool) Execute(ctx context.Context, call Call, execCtx ExecContext) (Result, error) {
	service, turnCtx, err := requireRuntimeGraph(execCtx)
	if err != nil {
		return Result{}, err
	}

	finalState := types.PlanState(strings.TrimSpace(call.StringInput("state")))
	if finalState == "" {
		finalState = types.PlanStateCompleted
	}

	out, err := service.ExitPlanMode(ctx, runtimegraph.ExitPlanModeInput{
		SessionID:  turnCtx.CurrentSessionID,
		FinalState: finalState,
	})
	if err != nil {
		return Result{}, err
	}

	return Result{Text: mustJSON(out)}, nil
}

func requireRuntimeGraph(execCtx ExecContext) (*runtimegraph.Service, *runtimegraph.TurnContext, error) {
	if execCtx.RuntimeService == nil {
		return nil, nil, fmt.Errorf("runtime service is not configured")
	}
	if execCtx.TurnContext == nil {
		return nil, nil, fmt.Errorf("turn runtime context is not configured")
	}
	return execCtx.RuntimeService, execCtx.TurnContext, nil
}
