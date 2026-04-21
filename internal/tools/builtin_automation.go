package tools

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"go-agent/internal/types"
)

type automationApplyTool struct{}
type automationGetTool struct{}
type automationListTool struct{}
type automationControlTool struct{}
type incidentAckTool struct{}
type incidentControlTool struct{}
type incidentGetTool struct{}
type incidentListTool struct{}

type AutomationApplyInput struct {
	Confirmed bool                    `json:"confirmed"`
	Spec      types.AutomationSpec    `json:"spec"`
	Assets    []types.AutomationAsset `json:"assets,omitempty"`
}

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

type IncidentAckInput struct {
	IncidentID string `json:"incident_id"`
}

type IncidentControlInput struct {
	IncidentID string                      `json:"incident_id"`
	Action     types.IncidentControlAction `json:"action"`
}

type IncidentGetInput struct {
	IncidentID string `json:"incident_id"`
}

type IncidentListInput struct {
	WorkspaceRoot string                         `json:"workspace_root,omitempty"`
	AutomationID  string                         `json:"automation_id,omitempty"`
	Status        types.AutomationIncidentStatus `json:"status,omitempty"`
	Limit         int                            `json:"limit,omitempty"`
}

func (automationApplyTool) IsEnabled(execCtx ExecContext) bool {
	return execCtx.AutomationService != nil
}

func (automationGetTool) IsEnabled(execCtx ExecContext) bool {
	return execCtx.AutomationService != nil
}

func (automationListTool) IsEnabled(execCtx ExecContext) bool {
	return execCtx.AutomationService != nil
}

func (automationControlTool) IsEnabled(execCtx ExecContext) bool {
	return execCtx.AutomationService != nil
}

func (incidentAckTool) IsEnabled(execCtx ExecContext) bool {
	return execCtx.AutomationService != nil
}

func (incidentControlTool) IsEnabled(execCtx ExecContext) bool {
	return execCtx.AutomationService != nil
}

func (incidentGetTool) IsEnabled(execCtx ExecContext) bool {
	return execCtx.AutomationService != nil
}

func (incidentListTool) IsEnabled(execCtx ExecContext) bool {
	return execCtx.AutomationService != nil
}

func (automationApplyTool) Definition() Definition {
	return Definition{
		Name:        "automation_apply",
		Description: "Persist an automation spec plus any workspace assets. For script-backed automations, save detector facts in scripts/detect.sh and child-agent strategy in child_agents/<phase>/<agent_id>/{strategy.json,prompt.md,skills.json} before applying.",
		InputSchema: objectSchema(map[string]any{
			"confirmed": map[string]any{
				"type":        "boolean",
				"description": "Must be true after the user has reviewed and approved the final automation draft.",
			},
			"spec": automationSpecInputSchema(),
			"assets": map[string]any{
				"type":        "array",
				"items":       automationAssetSchema(),
				"description": "Workspace files to persist before validation, including scripts/detect.sh and child_agents/<phase>/<agent_id> asset bundles.",
			},
		}, "confirmed", "spec"),
		OutputSchema: automationSpecOutputSchema(),
	}
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

func (automationApplyTool) IsConcurrencySafe() bool   { return false }
func (automationGetTool) IsConcurrencySafe() bool     { return true }
func (automationListTool) IsConcurrencySafe() bool    { return true }
func (automationControlTool) IsConcurrencySafe() bool { return false }

func (automationApplyTool) Decode(call Call) (DecodedCall, error) {
	rawConfirmed, ok := call.Input["confirmed"]
	if !ok {
		return DecodedCall{}, fmt.Errorf("confirmed is required")
	}
	confirmed, ok := rawConfirmed.(bool)
	if !ok {
		return DecodedCall{}, fmt.Errorf("confirmed must be a boolean")
	}
	raw, ok := call.Input["spec"]
	if !ok {
		return DecodedCall{}, fmt.Errorf("spec is required")
	}
	var spec types.AutomationSpec
	if err := decodeAutomationJSON(raw, &spec); err != nil {
		return DecodedCall{}, fmt.Errorf("spec must be a valid automation spec: %w", err)
	}
	var assets []types.AutomationAsset
	if rawAssets, ok := call.Input["assets"]; ok {
		if err := decodeAutomationJSON(rawAssets, &assets); err != nil {
			return DecodedCall{}, fmt.Errorf("assets must be valid automation assets: %w", err)
		}
	}
	return DecodedCall{Call: call, Input: AutomationApplyInput{
		Confirmed: confirmed,
		Spec:      spec,
		Assets:    assets,
	}}, nil
}

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

func (t automationApplyTool) Execute(ctx context.Context, call Call, execCtx ExecContext) (Result, error) {
	decoded, err := t.Decode(call)
	if err != nil {
		return Result{}, err
	}
	output, err := t.ExecuteDecoded(ctx, decoded, execCtx)
	return output.Result, err
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

func (automationApplyTool) ExecuteDecoded(ctx context.Context, decoded DecodedCall, execCtx ExecContext) (ToolExecutionResult, error) {
	service, err := requireAutomationService(execCtx)
	if err != nil {
		return ToolExecutionResult{}, err
	}
	input, _ := decoded.Input.(AutomationApplyInput)
	input.Spec.WorkspaceRoot = collapseAutomationAssetBundleWorkspaceRoot(input.Spec.WorkspaceRoot, execCtx.WorkspaceRoot)
	spec, err := service.ApplyRequest(ctx, types.ApplyAutomationRequest{
		Confirmed: input.Confirmed,
		Spec:      input.Spec,
		Assets:    input.Assets,
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

func collapseAutomationAssetBundleWorkspaceRoot(specRoot, sessionRoot string) string {
	specRoot = strings.TrimSpace(specRoot)
	sessionRoot = strings.TrimSpace(sessionRoot)
	if specRoot == "" || sessionRoot == "" {
		return specRoot
	}
	rel, err := filepath.Rel(sessionRoot, specRoot)
	if err != nil {
		return specRoot
	}
	rel = filepath.ToSlash(rel)
	if rel == "." || rel == ".." || strings.HasPrefix(rel, "../") {
		return specRoot
	}
	if rel == "automations" || strings.HasPrefix(rel, "automations/") {
		return sessionRoot
	}
	return specRoot
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

func (automationApplyTool) MapModelResult(output ToolExecutionResult) ModelToolResult {
	return defaultStructuredModelResult(output)
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
