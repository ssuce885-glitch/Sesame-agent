package tools

import (
	"context"
	"fmt"
	"strings"

	"go-agent/internal/automation"
	"go-agent/internal/types"
)

type automationCreateDetectorTool struct{}

func (automationCreateDetectorTool) IsEnabled(execCtx ExecContext) bool {
	return execCtx.AutomationService != nil
}

func (automationCreateDetectorTool) IsConcurrencySafe() bool { return false }

func (automationCreateDetectorTool) Definition() Definition {
	return Definition{
		Name:        "automation_create_detector",
		Description: "Build and persist a native detector automation spec from high-level detector intent.",
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
				"description":          "Fact keys and value types the detector emits.",
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

func (t automationCreateDetectorTool) Execute(ctx context.Context, call Call, execCtx ExecContext) (Result, error) {
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

func (automationCreateDetectorTool) MapModelResult(output ToolExecutionResult) ModelToolResult {
	return defaultStructuredModelResult(output)
}
