package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"go-agent/internal/types"
)

type automationApplyTool struct{}
type automationGetTool struct{}
type automationListTool struct{}
type automationControlTool struct{}
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

func (incidentGetTool) IsEnabled(execCtx ExecContext) bool {
	return execCtx.AutomationService != nil
}

func (incidentListTool) IsEnabled(execCtx ExecContext) bool {
	return execCtx.AutomationService != nil
}

func (automationApplyTool) Definition() Definition {
	return Definition{
		Name:        "automation_apply",
		Description: "Persist a normalized automation spec. Active specs auto-install their watcher runtime, so use this only after the draft has complete response, delivery, runtime, and runnable signal details.",
		InputSchema: objectSchema(map[string]any{
			"confirmed": map[string]any{
				"type":        "boolean",
				"description": "Must be true after the user has reviewed and approved the final automation draft.",
			},
			"spec": automationSpecInputSchema(),
			"assets": map[string]any{
				"type":  "array",
				"items": automationAssetSchema(),
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

func (incidentGetTool) Definition() Definition {
	return Definition{
		Name:        "incident_get",
		Description: "Fetch a stored automation incident by id.",
		InputSchema: objectSchema(map[string]any{
			"incident_id": map[string]any{
				"type":        "string",
				"description": "Incident identifier.",
			},
		}, "incident_id"),
		OutputSchema: automationIncidentOutputSchema(),
	}
}

func (incidentListTool) Definition() Definition {
	return Definition{
		Name:        "incident_list",
		Description: "List automation incidents, optionally filtered by workspace, automation, or status.",
		InputSchema: objectSchema(map[string]any{
			"workspace_root": map[string]any{
				"type":        "string",
				"description": "Optional workspace filter.",
			},
			"automation_id": map[string]any{
				"type":        "string",
				"description": "Optional automation id filter.",
			},
			"status": map[string]any{
				"type":        "string",
				"enum":        automationIncidentStatusEnum(),
				"description": "Optional incident status filter.",
			},
			"limit": map[string]any{
				"type":        "integer",
				"description": "Optional positive result limit.",
			},
		}),
		OutputSchema: objectSchema(map[string]any{
			"incidents": map[string]any{
				"type":  "array",
				"items": automationIncidentOutputSchema(),
			},
		}, "incidents"),
	}
}

func (automationApplyTool) IsConcurrencySafe() bool   { return false }
func (automationGetTool) IsConcurrencySafe() bool     { return true }
func (automationListTool) IsConcurrencySafe() bool    { return true }
func (automationControlTool) IsConcurrencySafe() bool { return false }
func (incidentGetTool) IsConcurrencySafe() bool       { return true }
func (incidentListTool) IsConcurrencySafe() bool      { return true }

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
	input := AutomationListInput{
		WorkspaceRoot: strings.TrimSpace(call.StringInput("workspace_root")),
		State:         types.AutomationState(strings.TrimSpace(call.StringInput("state"))),
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
	return DecodedCall{Call: call, Input: input}, nil
}

func (incidentGetTool) Decode(call Call) (DecodedCall, error) {
	input := IncidentGetInput{IncidentID: strings.TrimSpace(call.StringInput("incident_id"))}
	if input.IncidentID == "" {
		return DecodedCall{}, fmt.Errorf("incident_id is required")
	}
	return DecodedCall{Call: call, Input: input}, nil
}

func (incidentListTool) Decode(call Call) (DecodedCall, error) {
	limit, err := decodeOptionalPositiveInt(call.Input["limit"])
	if err != nil {
		return DecodedCall{}, fmt.Errorf("limit %w", err)
	}
	input := IncidentListInput{
		WorkspaceRoot: strings.TrimSpace(call.StringInput("workspace_root")),
		AutomationID:  strings.TrimSpace(call.StringInput("automation_id")),
		Status:        types.AutomationIncidentStatus(strings.TrimSpace(call.StringInput("status"))),
		Limit:         limit,
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

func (t incidentGetTool) Execute(ctx context.Context, call Call, execCtx ExecContext) (Result, error) {
	decoded, err := t.Decode(call)
	if err != nil {
		return Result{}, err
	}
	output, err := t.ExecuteDecoded(ctx, decoded, execCtx)
	return output.Result, err
}

func (t incidentListTool) Execute(ctx context.Context, call Call, execCtx ExecContext) (Result, error) {
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

func (incidentGetTool) ExecuteDecoded(ctx context.Context, decoded DecodedCall, execCtx ExecContext) (ToolExecutionResult, error) {
	service, err := requireAutomationService(execCtx)
	if err != nil {
		return ToolExecutionResult{}, err
	}
	input, _ := decoded.Input.(IncidentGetInput)
	incident, ok, err := service.GetIncident(ctx, input.IncidentID)
	if err != nil {
		return ToolExecutionResult{}, err
	}
	if !ok {
		return ToolExecutionResult{}, fmt.Errorf("incident %q not found", input.IncidentID)
	}
	return ToolExecutionResult{
		Result: Result{
			Text:      mustJSON(incident),
			ModelText: mustJSON(incident),
		},
		Data: incident,
	}, nil
}

func (incidentListTool) ExecuteDecoded(ctx context.Context, decoded DecodedCall, execCtx ExecContext) (ToolExecutionResult, error) {
	service, err := requireAutomationService(execCtx)
	if err != nil {
		return ToolExecutionResult{}, err
	}
	input, _ := decoded.Input.(IncidentListInput)
	incidents, err := service.ListIncidents(ctx, types.AutomationIncidentFilter{
		WorkspaceRoot: input.WorkspaceRoot,
		AutomationID:  input.AutomationID,
		Status:        input.Status,
		Limit:         input.Limit,
	})
	if err != nil {
		return ToolExecutionResult{}, err
	}
	output := types.ListAutomationIncidentsResponse{Incidents: incidents}
	return ToolExecutionResult{
		Result: Result{
			Text:      mustJSON(output),
			ModelText: mustJSON(output),
		},
		Data: output,
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

func (incidentGetTool) MapModelResult(output ToolExecutionResult) ModelToolResult {
	return defaultStructuredModelResult(output)
}

func (incidentListTool) MapModelResult(output ToolExecutionResult) ModelToolResult {
	return defaultStructuredModelResult(output)
}

func requireAutomationService(execCtx ExecContext) (AutomationService, error) {
	if execCtx.AutomationService == nil {
		return nil, fmt.Errorf("automation service is not configured")
	}
	return execCtx.AutomationService, nil
}

func decodeAutomationJSON(raw any, out any) error {
	data, err := json.Marshal(raw)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, out)
}

func decodeOptionalPositiveInt(raw any) (int, error) {
	if raw == nil {
		return 0, nil
	}
	switch value := raw.(type) {
	case int:
		if value <= 0 {
			return 0, fmt.Errorf("must be greater than zero")
		}
		return value, nil
	case int32:
		if value <= 0 {
			return 0, fmt.Errorf("must be greater than zero")
		}
		return int(value), nil
	case int64:
		if value <= 0 {
			return 0, fmt.Errorf("must be greater than zero")
		}
		return int(value), nil
	case float64:
		if value <= 0 || value != float64(int(value)) {
			return 0, fmt.Errorf("must be a positive integer")
		}
		return int(value), nil
	default:
		return 0, fmt.Errorf("must be a positive integer")
	}
}

func automationSpecInputSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"properties":           automationSpecProperties(),
		"additionalProperties": false,
	}
}

func automationSpecOutputSchema() map[string]any {
	return objectSchema(automationSpecProperties(), "id", "title", "workspace_root", "goal", "state")
}

func automationAssetSchema() map[string]any {
	return objectSchema(map[string]any{
		"path": map[string]any{
			"type": "string",
		},
		"content": map[string]any{
			"type": "string",
		},
		"executable": map[string]any{
			"type": "boolean",
		},
	}, "path", "content")
}

func automationSpecProperties() map[string]any {
	return map[string]any{
		"id": map[string]any{
			"type": "string",
		},
		"title": map[string]any{
			"type": "string",
		},
		"workspace_root": map[string]any{
			"type": "string",
		},
		"goal": map[string]any{
			"type": "string",
		},
		"state": map[string]any{
			"type": "string",
			"enum": automationStateEnum(),
		},
		"context": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"targets": map[string]any{
					"type":  "array",
					"items": map[string]any{"type": "string"},
				},
				"labels": map[string]any{
					"type":                 "object",
					"additionalProperties": true,
				},
				"owner": map[string]any{
					"type": "string",
				},
				"environment": map[string]any{
					"type": "string",
				},
			},
			"additionalProperties": false,
		},
		"signals": map[string]any{
			"type": "array",
			"items": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"kind": map[string]any{
						"type": "string",
					},
					"source": map[string]any{
						"type": "string",
					},
					"selector": map[string]any{
						"type": "string",
					},
					"payload": map[string]any{},
				},
				"additionalProperties": false,
			},
		},
		"incident_policy": map[string]any{
			"type":                 "object",
			"additionalProperties": true,
		},
		"response_plan": map[string]any{
			"type":                 "object",
			"additionalProperties": true,
		},
		"verification_plan": map[string]any{
			"type":                 "object",
			"additionalProperties": true,
		},
		"escalation_policy": map[string]any{
			"type":                 "object",
			"additionalProperties": true,
		},
		"delivery_policy": map[string]any{
			"type":                 "object",
			"additionalProperties": true,
		},
		"runtime_policy": map[string]any{
			"type":                 "object",
			"additionalProperties": true,
		},
		"watcher_lifecycle": map[string]any{
			"type":                 "object",
			"additionalProperties": true,
		},
		"retrigger_policy": map[string]any{
			"type":                 "object",
			"additionalProperties": true,
		},
		"run_policy": map[string]any{
			"type":                 "object",
			"additionalProperties": true,
		},
		"assumptions": map[string]any{
			"type":  "array",
			"items": map[string]any{"type": "string"},
		},
		"created_at": map[string]any{
			"type": "string",
		},
		"updated_at": map[string]any{
			"type": "string",
		},
	}
}

func automationIncidentOutputSchema() map[string]any {
	return objectSchema(map[string]any{
		"id": map[string]any{
			"type": "string",
		},
		"automation_id": map[string]any{
			"type": "string",
		},
		"workspace_root": map[string]any{
			"type": "string",
		},
		"status": map[string]any{
			"type": "string",
			"enum": automationIncidentStatusEnum(),
		},
		"signal_kind": map[string]any{
			"type": "string",
		},
		"source": map[string]any{
			"type": "string",
		},
		"summary": map[string]any{
			"type": "string",
		},
		"payload": map[string]any{},
		"observed_at": map[string]any{
			"type": "string",
		},
		"created_at": map[string]any{
			"type": "string",
		},
		"updated_at": map[string]any{
			"type": "string",
		},
	}, "id", "automation_id", "workspace_root", "status")
}

func automationStateEnum() []string {
	return []string{
		string(types.AutomationStateActive),
		string(types.AutomationStatePaused),
	}
}

func automationControlActionEnum() []string {
	return []string{
		string(types.AutomationControlActionPause),
		string(types.AutomationControlActionResume),
	}
}

func automationIncidentStatusEnum() []string {
	return []string{
		string(types.AutomationIncidentStatusOpen),
	}
}
