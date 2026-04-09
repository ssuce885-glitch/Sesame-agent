package tools

import (
	"context"
	"fmt"
	"strings"

	"go-agent/internal/runtimegraph"
	"go-agent/internal/types"
)

type enterPlanModeTool struct{}

type EnterPlanModeInput struct {
	PlanFile string `json:"plan_file"`
}

type ExitPlanModeInput struct {
	State string `json:"state,omitempty"`
}

func (enterPlanModeTool) IsEnabled(execCtx ExecContext) bool {
	return execCtx.RuntimeService != nil && execCtx.TurnContext != nil
}

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
		OutputSchema: objectSchema(map[string]any{
			"plan_id":   map[string]any{"type": "string"},
			"run_id":    map[string]any{"type": "string"},
			"state":     map[string]any{"type": "string"},
			"plan_file": map[string]any{"type": "string"},
		}, "plan_id", "run_id", "state", "plan_file"),
	}
}

func (enterPlanModeTool) IsConcurrencySafe() bool { return false }

func (enterPlanModeTool) Decode(call Call) (DecodedCall, error) {
	planFile := strings.TrimSpace(call.StringInput("plan_file"))
	if planFile == "" {
		return DecodedCall{}, fmt.Errorf("plan_file is required")
	}
	return DecodedCall{
		Call: Call{
			Name: call.Name,
			Input: map[string]any{
				"plan_file": planFile,
			},
		},
		Input: EnterPlanModeInput{PlanFile: planFile},
	}, nil
}

func (t enterPlanModeTool) Execute(ctx context.Context, call Call, execCtx ExecContext) (Result, error) {
	decoded, err := t.Decode(call)
	if err != nil {
		return Result{}, err
	}
	output, err := t.ExecuteDecoded(ctx, decoded, execCtx)
	return output.Result, err
}

func (enterPlanModeTool) ExecuteDecoded(ctx context.Context, decoded DecodedCall, execCtx ExecContext) (ToolExecutionResult, error) {
	service, turnCtx, err := requireRuntimeGraph(execCtx)
	if err != nil {
		return ToolExecutionResult{}, err
	}

	input, _ := decoded.Input.(EnterPlanModeInput)
	out, err := service.EnterPlanMode(ctx, turnCtx, runtimegraph.EnterPlanModeInput{
		SessionID: turnCtx.CurrentSessionID,
		TurnID:    turnCtx.CurrentTurnID,
		RunID:     turnCtx.CurrentRunID,
		PlanFile:  input.PlanFile,
	})
	if err != nil {
		return ToolExecutionResult{}, err
	}
	emitTimelineBlockEvent(ctx, execCtx, types.EventPlanUpdated, types.TimelineBlock{
		ID:     out.PlanID,
		RunID:  out.RunID,
		Kind:   "plan_block",
		Status: string(out.State),
		Title:  input.PlanFile,
		PlanID: out.PlanID,
		Path:   input.PlanFile,
	})

	text := mustJSON(out)
	preview := fmt.Sprintf("Plan mode entered: %s", out.State)
	return ToolExecutionResult{
		Result: Result{
			Text:      text,
			ModelText: text,
		},
		Data:        out,
		PreviewText: preview,
		Metadata: map[string]any{
			"plan_id": out.PlanID,
			"run_id":  out.RunID,
			"state":   out.State,
		},
	}, nil
}

func (enterPlanModeTool) MapModelResult(output ToolExecutionResult) ModelToolResult {
	return defaultStructuredModelResult(output)
}

type exitPlanModeTool struct{}

func (exitPlanModeTool) IsEnabled(execCtx ExecContext) bool {
	return execCtx.RuntimeService != nil && execCtx.TurnContext != nil
}

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
		OutputSchema: objectSchema(map[string]any{
			"plan_id": map[string]any{"type": "string"},
			"state":   map[string]any{"type": "string"},
		}, "plan_id", "state"),
	}
}

func (exitPlanModeTool) IsConcurrencySafe() bool { return false }

func (exitPlanModeTool) Decode(call Call) (DecodedCall, error) {
	state := strings.TrimSpace(call.StringInput("state"))
	if state != "" {
		switch types.PlanState(state) {
		case types.PlanStateCompleted, types.PlanStateApproved, types.PlanStateFailed:
		default:
			return DecodedCall{}, fmt.Errorf("invalid plan state %q", state)
		}
	}
	return DecodedCall{
		Call: Call{
			Name: call.Name,
			Input: map[string]any{
				"state": state,
			},
		},
		Input: ExitPlanModeInput{State: state},
	}, nil
}

func (t exitPlanModeTool) Execute(ctx context.Context, call Call, execCtx ExecContext) (Result, error) {
	decoded, err := t.Decode(call)
	if err != nil {
		return Result{}, err
	}
	output, err := t.ExecuteDecoded(ctx, decoded, execCtx)
	return output.Result, err
}

func (exitPlanModeTool) ExecuteDecoded(ctx context.Context, decoded DecodedCall, execCtx ExecContext) (ToolExecutionResult, error) {
	service, turnCtx, err := requireRuntimeGraph(execCtx)
	if err != nil {
		return ToolExecutionResult{}, err
	}

	input, _ := decoded.Input.(ExitPlanModeInput)
	finalState := types.PlanState(input.State)
	if finalState == "" {
		finalState = types.PlanStateCompleted
	}

	out, err := service.ExitPlanMode(ctx, runtimegraph.ExitPlanModeInput{
		SessionID:  turnCtx.CurrentSessionID,
		FinalState: finalState,
	})
	if err != nil {
		return ToolExecutionResult{}, err
	}
	emitTimelineBlockEvent(ctx, execCtx, types.EventPlanUpdated, types.TimelineBlock{
		ID:     out.PlanID,
		RunID:  turnCtx.CurrentRunID,
		Kind:   "plan_block",
		Status: string(out.State),
		PlanID: out.PlanID,
	})

	text := mustJSON(out)
	preview := fmt.Sprintf("Plan mode exited: %s", out.State)
	return ToolExecutionResult{
		Result: Result{
			Text:      text,
			ModelText: text,
		},
		Data:        out,
		PreviewText: preview,
		Metadata: map[string]any{
			"plan_id": out.PlanID,
			"state":   out.State,
		},
	}, nil
}

func (exitPlanModeTool) MapModelResult(output ToolExecutionResult) ModelToolResult {
	return defaultStructuredModelResult(output)
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
