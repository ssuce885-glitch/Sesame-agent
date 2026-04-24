package tools

import (
	"context"
	"fmt"
	"strings"

	"go-agent/internal/types"
)

type automationGetTool struct{}
type automationListTool struct{}
type automationControlTool struct{}

type AutomationGetInput struct {
	AutomationID string `json:"automation_id"`
}

type AutomationListInput struct {
	WorkspaceRoot string                `json:"workspace_root,omitempty"`
	State         types.AutomationState `json:"state,omitempty"`
	Limit         int                   `json:"limit,omitempty"`
}

type AutomationControlInput struct {
	AutomationID string                        `json:"automation_id"`
	Action       types.AutomationControlAction `json:"action"`
}

func (automationGetTool) IsEnabled(execCtx ExecContext) bool {
	return execCtx.AutomationService != nil
}

func (automationListTool) IsEnabled(execCtx ExecContext) bool {
	return execCtx.AutomationService != nil
}

func (automationControlTool) IsEnabled(execCtx ExecContext) bool {
	return execCtx.AutomationService != nil &&
		!isAutomationOwnerTaskMode(execCtx) &&
		hasActiveSkills(execCtx, "automation-standard-behavior")
}

func (automationGetTool) Definition() Definition {
	return Definition{
		Name:        "automation_get",
		Description: "Fetch a stored automation spec by id.",
		InputSchema: objectSchema(map[string]any{
			"automation_id": map[string]any{
				"type":        "string",
				"description": "Automation identifier.",
			},
		}, "automation_id"),
		OutputSchema: automationSpecOutputSchema(),
	}
}

func (automationListTool) Definition() Definition {
	return Definition{
		Name:        "automation_list",
		Description: "List stored automation specs, optionally filtered by workspace or state.",
		InputSchema: objectSchema(map[string]any{
			"workspace_root": map[string]any{
				"type":        "string",
				"description": "Optional workspace filter.",
			},
			"state": map[string]any{
				"type":        "string",
				"enum":        automationStateEnum(),
				"description": "Optional automation state filter.",
			},
			"limit": map[string]any{
				"type":        "integer",
				"description": "Optional positive result limit.",
			},
		}),
		OutputSchema: objectSchema(map[string]any{
			"automations": map[string]any{
				"type":  "array",
				"items": automationSpecOutputSchema(),
			},
		}, "automations"),
	}
}

func (automationControlTool) Definition() Definition {
	return Definition{
		Name:        "automation_control",
		Description: "Pause or resume an automation by id.",
		InputSchema: objectSchema(map[string]any{
			"automation_id": map[string]any{
				"type":        "string",
				"description": "Automation identifier.",
			},
			"action": map[string]any{
				"type":        "string",
				"enum":        automationControlActionEnum(),
				"description": "Control action to apply.",
			},
		}, "automation_id", "action"),
		OutputSchema: automationSpecOutputSchema(),
	}
}

func (automationGetTool) IsConcurrencySafe() bool     { return true }
func (automationListTool) IsConcurrencySafe() bool    { return true }
func (automationControlTool) IsConcurrencySafe() bool { return false }

func (automationGetTool) Decode(call Call) (DecodedCall, error) {
	input := AutomationGetInput{AutomationID: strings.TrimSpace(call.StringInput("automation_id"))}
	if input.AutomationID == "" {
		return DecodedCall{}, fmt.Errorf("automation_id is required")
	}
	return DecodedCall{Call: call, Input: input}, nil
}

func (automationListTool) Decode(call Call) (DecodedCall, error) {
	limit, err := decodeOptionalPositiveInt(call.Input["limit"])
	if err != nil {
		return DecodedCall{}, fmt.Errorf("limit %w", err)
	}
	state := types.AutomationState(strings.TrimSpace(call.StringInput("state")))
	if state != "" {
		switch state {
		case types.AutomationStateActive,
			types.AutomationStatePaused:
		default:
			return DecodedCall{}, fmt.Errorf(`invalid state %q; must be one of active, paused`, state)
		}
	}
	input := AutomationListInput{
		WorkspaceRoot: strings.TrimSpace(call.StringInput("workspace_root")),
		State:         state,
		Limit:         limit,
	}
	return DecodedCall{Call: call, Input: input}, nil
}

func (automationControlTool) Decode(call Call) (DecodedCall, error) {
	input := AutomationControlInput{
		AutomationID: strings.TrimSpace(call.StringInput("automation_id")),
		Action:       types.AutomationControlAction(strings.TrimSpace(call.StringInput("action"))),
	}
	if input.AutomationID == "" {
		return DecodedCall{}, fmt.Errorf("automation_id is required")
	}
	if input.Action == "" {
		return DecodedCall{}, fmt.Errorf("action is required")
	}
	switch input.Action {
	case types.AutomationControlActionPause,
		types.AutomationControlActionResume:
	default:
		return DecodedCall{}, fmt.Errorf(`invalid action %q; must be one of pause, resume`, input.Action)
	}
	return DecodedCall{Call: call, Input: input}, nil
}

func (t automationGetTool) Execute(ctx context.Context, call Call, execCtx ExecContext) (Result, error) {
	decoded, err := t.Decode(call)
	if err != nil {
		return Result{}, err
	}
	output, err := t.ExecuteDecoded(ctx, decoded, execCtx)
	return output.Result, err
}

func (t automationListTool) Execute(ctx context.Context, call Call, execCtx ExecContext) (Result, error) {
	decoded, err := t.Decode(call)
	if err != nil {
		return Result{}, err
	}
	output, err := t.ExecuteDecoded(ctx, decoded, execCtx)
	return output.Result, err
}

func (t automationControlTool) Execute(ctx context.Context, call Call, execCtx ExecContext) (Result, error) {
	decoded, err := t.Decode(call)
	if err != nil {
		return Result{}, err
	}
	output, err := t.ExecuteDecoded(ctx, decoded, execCtx)
	return output.Result, err
}

func (automationGetTool) ExecuteDecoded(ctx context.Context, decoded DecodedCall, execCtx ExecContext) (ToolExecutionResult, error) {
	service, err := requireAutomationService(execCtx)
	if err != nil {
		return ToolExecutionResult{}, err
	}
	input, _ := decoded.Input.(AutomationGetInput)
	spec, ok, err := service.Get(ctx, input.AutomationID)
	if err != nil {
		return ToolExecutionResult{}, err
	}
	if !ok {
		return ToolExecutionResult{}, fmt.Errorf("automation %q not found", input.AutomationID)
	}
	return ToolExecutionResult{
		Result: Result{
			Text:      mustJSON(spec),
			ModelText: mustJSON(spec),
		},
		Data: spec,
	}, nil
}

func (automationListTool) ExecuteDecoded(ctx context.Context, decoded DecodedCall, execCtx ExecContext) (ToolExecutionResult, error) {
	service, err := requireAutomationService(execCtx)
	if err != nil {
		return ToolExecutionResult{}, err
	}
	input, _ := decoded.Input.(AutomationListInput)
	automations, err := service.List(ctx, types.AutomationListFilter{
		WorkspaceRoot: input.WorkspaceRoot,
		State:         input.State,
		Limit:         input.Limit,
	})
	if err != nil {
		return ToolExecutionResult{}, err
	}
	output := types.ListAutomationsResponse{Automations: automations}
	return ToolExecutionResult{
		Result: Result{
			Text:      mustJSON(output),
			ModelText: mustJSON(output),
		},
		Data: output,
	}, nil
}

func (automationControlTool) ExecuteDecoded(ctx context.Context, decoded DecodedCall, execCtx ExecContext) (ToolExecutionResult, error) {
	service, err := requireAutomationService(execCtx)
	if err != nil {
		return ToolExecutionResult{}, err
	}
	if err := rejectAutomationControlFromOwnerTask(execCtx); err != nil {
		return ToolExecutionResult{}, err
	}
	if err := requireActiveSkills(execCtx, "automation-standard-behavior"); err != nil {
		return ToolExecutionResult{}, err
	}
	input, _ := decoded.Input.(AutomationControlInput)
	spec, ok, err := service.Control(ctx, input.AutomationID, input.Action)
	if err != nil {
		return ToolExecutionResult{}, err
	}
	if !ok {
		return ToolExecutionResult{}, fmt.Errorf("automation %q not found", input.AutomationID)
	}
	return ToolExecutionResult{
		Result: Result{
			Text:      mustJSON(spec),
			ModelText: mustJSON(spec),
		},
		Data: spec,
	}, nil
}

func rejectAutomationControlFromOwnerTask(execCtx ExecContext) error {
	if !isAutomationOwnerTaskMode(execCtx) {
		return nil
	}
	return fmt.Errorf("automation_control is not allowed in Owner Task Mode; execute the automation_goal and report the result instead of pausing or resuming automations")
}

func (automationGetTool) MapModelResult(output ToolExecutionResult) ModelToolResult {
	return defaultStructuredModelResult(output)
}

func (automationListTool) MapModelResult(output ToolExecutionResult) ModelToolResult {
	return defaultStructuredModelResult(output)
}

func (automationControlTool) MapModelResult(output ToolExecutionResult) ModelToolResult {
	return defaultStructuredModelResult(output)
}
