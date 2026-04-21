package tools

import (
	"context"
	"fmt"
	"strings"

	"go-agent/internal/automation"
	"go-agent/internal/types"
)

type automationCreateDetectorTool struct{}
type automationConfigureIncidentPolicyTool struct{}
type automationConfigureDispatchPolicyTool struct{}

func (automationCreateDetectorTool) IsEnabled(execCtx ExecContext) bool {
	return execCtx.AutomationService != nil
}
func (automationConfigureIncidentPolicyTool) IsEnabled(execCtx ExecContext) bool {
	return execCtx.AutomationService != nil
}
func (automationConfigureDispatchPolicyTool) IsEnabled(execCtx ExecContext) bool {
	return execCtx.AutomationService != nil
}

func (automationCreateDetectorTool) IsConcurrencySafe() bool          { return false }
func (automationConfigureIncidentPolicyTool) IsConcurrencySafe() bool { return false }
func (automationConfigureDispatchPolicyTool) IsConcurrencySafe() bool { return false }

func (automationCreateDetectorTool) Definition() Definition {
	return Definition{
		Name:        "automation_create_detector",
		Description: "Preferred high-level builder for detector-first native automation. Compile and persist watcher-native detector automation from high-level intent so validation runs through the native detector/incident pipeline instead of shell-loop substitutes.",
		InputSchema: objectSchema(map[string]any{
			"automation_id": map[string]any{
				"type":        "string",
				"description": "Automation identifier to persist.",
			},
			"title": map[string]any{
				"type":        "string",
				"description": "Optional automation title.",
			},
			"detector_kind": map[string]any{
				"type":        "string",
				"enum":        []string{string(types.NativeDetectorKindFile), string(types.NativeDetectorKindCommand), string(types.NativeDetectorKindHealth)},
				"description": "Detector kind to compile.",
			},
			"target": map[string]any{
				"type":                 "object",
				"additionalProperties": true,
			},
			"schedule": map[string]any{
				"type":                 "object",
				"additionalProperties": true,
			},
			"condition": map[string]any{
				"type":                 "object",
				"additionalProperties": true,
			},
			"facts_schema": map[string]any{
				"type":                 "object",
				"additionalProperties": map[string]any{"type": "string"},
				"description":          "Fact keys and value types emitted by the cheap detector.",
			},
			"dedupe": map[string]any{
				"type":                 "object",
				"additionalProperties": true,
			},
			"state": map[string]any{
				"type":        "string",
				"description": "Optional detector state hint.",
			},
		}, "automation_id", "facts_schema"),
		OutputSchema: automationSpecOutputSchema(),
	}
}

func (automationConfigureIncidentPolicyTool) Definition() Definition {
	return Definition{
		Name:        "automation_configure_incident_policy",
		Description: "Configure incident policy from detector-native facts schema and persist runtime-consumed dedupe settings.",
		InputSchema: objectSchema(map[string]any{
			"automation_id": map[string]any{
				"type": "string",
			},
			"create_incident_on": map[string]any{
				"type": "string",
			},
			"summary_template": map[string]any{
				"type": "string",
			},
			"dedupe_policy": map[string]any{
				"type":                 "object",
				"additionalProperties": true,
			},
			"severity": map[string]any{
				"type": "string",
			},
			"auto_close_minutes": map[string]any{
				"type": "integer",
			},
		}, "automation_id", "create_incident_on"),
		OutputSchema: automationSpecOutputSchema(),
	}
}

func (automationConfigureDispatchPolicyTool) Definition() Definition {
	return Definition{
		Name:        "automation_configure_dispatch_policy",
		Description: "Configure dispatch policy from detector-native facts schema and compile runnable notify/run_task execution paths.",
		InputSchema: objectSchema(map[string]any{
			"automation_id": map[string]any{
				"type": "string",
			},
			"dispatch_mode": map[string]any{
				"type": "string",
				"enum": []string{
					string(types.NativeDispatchModeNotifyOnly),
					string(types.NativeDispatchModeRunTask),
				},
			},
			"action_kind": map[string]any{
				"type": "string",
				"enum": []string{
					string(types.NativeActionKindDeleteFile),
					string(types.NativeActionKindRunScript),
					string(types.NativeActionKindSendEmail),
					string(types.NativeActionKindNotifyOnly),
				},
			},
			"action_args": map[string]any{
				"type":                 "object",
				"additionalProperties": map[string]any{"type": "string"},
			},
			"verification": map[string]any{
				"type":                 "object",
				"additionalProperties": true,
			},
			"reporting": map[string]any{
				"type":                 "object",
				"additionalProperties": true,
			},
		}, "automation_id", "dispatch_mode", "action_kind"),
		OutputSchema: automationSpecOutputSchema(),
	}
}

func (automationCreateDetectorTool) Decode(call Call) (DecodedCall, error) {
	var input types.NativeDetectorBuilderInput
	if err := decodeAutomationJSON(call.Input, &input); err != nil {
		return DecodedCall{}, fmt.Errorf("input must be a valid detector builder payload: %w", err)
	}
	input.AutomationID = strings.TrimSpace(input.AutomationID)
	return DecodedCall{
		Call:  call,
		Input: input,
	}, nil
}

func (automationConfigureIncidentPolicyTool) Decode(call Call) (DecodedCall, error) {
	var input types.NativeIncidentPolicyInput
	if err := decodeAutomationJSON(call.Input, &input); err != nil {
		return DecodedCall{}, fmt.Errorf("input must be a valid incident policy payload: %w", err)
	}
	input.AutomationID = strings.TrimSpace(input.AutomationID)
	return DecodedCall{
		Call:  call,
		Input: input,
	}, nil
}

func (automationConfigureDispatchPolicyTool) Decode(call Call) (DecodedCall, error) {
	var input types.NativeDispatchPolicyInput
	if err := decodeAutomationJSON(call.Input, &input); err != nil {
		return DecodedCall{}, fmt.Errorf("input must be a valid dispatch policy payload: %w", err)
	}
	input.AutomationID = strings.TrimSpace(input.AutomationID)
	return DecodedCall{
		Call:  call,
		Input: input,
	}, nil
}

func (t automationCreateDetectorTool) Execute(ctx context.Context, call Call, execCtx ExecContext) (Result, error) {
	decoded, err := t.Decode(call)
	if err != nil {
		return Result{}, err
	}
	output, err := t.ExecuteDecoded(ctx, decoded, execCtx)
	return output.Result, err
}

func (t automationConfigureIncidentPolicyTool) Execute(ctx context.Context, call Call, execCtx ExecContext) (Result, error) {
	decoded, err := t.Decode(call)
	if err != nil {
		return Result{}, err
	}
	output, err := t.ExecuteDecoded(ctx, decoded, execCtx)
	return output.Result, err
}

func (t automationConfigureDispatchPolicyTool) Execute(ctx context.Context, call Call, execCtx ExecContext) (Result, error) {
	decoded, err := t.Decode(call)
	if err != nil {
		return Result{}, err
	}
	output, err := t.ExecuteDecoded(ctx, decoded, execCtx)
	return output.Result, err
}

func (automationCreateDetectorTool) ExecuteDecoded(ctx context.Context, decoded DecodedCall, execCtx ExecContext) (ToolExecutionResult, error) {
	service, err := requireAutomationService(execCtx)
	if err != nil {
		return ToolExecutionResult{}, err
	}
	input, _ := decoded.Input.(types.NativeDetectorBuilderInput)
	spec, err := automation.CompileNativeDetectorBuilder(input, execCtx.WorkspaceRoot)
	if err != nil {
		return ToolExecutionResult{}, err
	}
	spec, err = service.Apply(ctx, spec)
	if err != nil {
		return ToolExecutionResult{}, err
	}
	return ToolExecutionResult{
		Result: Result{
			Text:      mustJSON(spec),
			ModelText: mustJSON(spec),
		},
		Data: spec,
	}, nil
}

func (automationConfigureIncidentPolicyTool) ExecuteDecoded(ctx context.Context, decoded DecodedCall, execCtx ExecContext) (ToolExecutionResult, error) {
	service, err := requireAutomationService(execCtx)
	if err != nil {
		return ToolExecutionResult{}, err
	}
	input, _ := decoded.Input.(types.NativeIncidentPolicyInput)
	spec, ok, err := service.Get(ctx, input.AutomationID)
	if err != nil {
		return ToolExecutionResult{}, err
	}
	if !ok {
		return ToolExecutionResult{}, fmt.Errorf("automation %q not found", input.AutomationID)
	}
	spec, err = automation.CompileNativeIncidentPolicy(spec, input)
	if err != nil {
		return ToolExecutionResult{}, err
	}
	spec, err = service.Apply(ctx, spec)
	if err != nil {
		return ToolExecutionResult{}, err
	}
	return ToolExecutionResult{
		Result: Result{
			Text:      mustJSON(spec),
			ModelText: mustJSON(spec),
		},
		Data: spec,
	}, nil
}

func (automationConfigureDispatchPolicyTool) ExecuteDecoded(ctx context.Context, decoded DecodedCall, execCtx ExecContext) (ToolExecutionResult, error) {
	service, err := requireAutomationService(execCtx)
	if err != nil {
		return ToolExecutionResult{}, err
	}
	input, _ := decoded.Input.(types.NativeDispatchPolicyInput)
	spec, ok, err := service.Get(ctx, input.AutomationID)
	if err != nil {
		return ToolExecutionResult{}, err
	}
	if !ok {
		return ToolExecutionResult{}, fmt.Errorf("automation %q not found", input.AutomationID)
	}
	spec, assets, err := automation.CompileNativeDispatchPolicy(spec, input)
	if err != nil {
		return ToolExecutionResult{}, err
	}
	spec, err = service.ApplyRequest(ctx, types.ApplyAutomationRequest{
		Confirmed: true,
		Spec:      spec,
		Assets:    assets,
	})
	if err != nil {
		return ToolExecutionResult{}, err
	}
	return ToolExecutionResult{
		Result: Result{
			Text:      mustJSON(spec),
			ModelText: mustJSON(spec),
		},
		Data: spec,
	}, nil
}

func (automationCreateDetectorTool) MapModelResult(output ToolExecutionResult) ModelToolResult {
	return defaultStructuredModelResult(output)
}

func (automationConfigureIncidentPolicyTool) MapModelResult(output ToolExecutionResult) ModelToolResult {
	return defaultStructuredModelResult(output)
}

func (automationConfigureDispatchPolicyTool) MapModelResult(output ToolExecutionResult) ModelToolResult {
	return defaultStructuredModelResult(output)
}
