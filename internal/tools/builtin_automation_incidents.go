package tools

import (
	"context"
	"fmt"
	"strings"

	"go-agent/internal/types"
)

func (incidentAckTool) Definition() Definition {
	return Definition{
		Name:        "incident_ack",
		Description: "Acknowledge an automation incident by id.",
		InputSchema: objectSchema(map[string]any{
			"incident_id": map[string]any{
				"type":        "string",
				"description": "Incident identifier.",
			},
		}, "incident_id"),
		OutputSchema: automationIncidentOutputSchema(),
	}
}

func (incidentControlTool) Definition() Definition {
	return Definition{
		Name:        "incident_control",
		Description: "Acknowledge, close, reopen, or escalate an automation incident by id.",
		InputSchema: objectSchema(map[string]any{
			"incident_id": map[string]any{
				"type":        "string",
				"description": "Incident identifier.",
			},
			"action": map[string]any{
				"type":        "string",
				"enum":        incidentControlActionEnum(),
				"description": "Control action to apply.",
			},
		}, "incident_id", "action"),
		OutputSchema: automationIncidentOutputSchema(),
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

func (incidentAckTool) IsConcurrencySafe() bool     { return false }
func (incidentControlTool) IsConcurrencySafe() bool { return false }
func (incidentGetTool) IsConcurrencySafe() bool     { return true }
func (incidentListTool) IsConcurrencySafe() bool    { return true }

func (incidentAckTool) Decode(call Call) (DecodedCall, error) {
	input := IncidentAckInput{IncidentID: strings.TrimSpace(call.StringInput("incident_id"))}
	if input.IncidentID == "" {
		return DecodedCall{}, fmt.Errorf("incident_id is required")
	}
	return DecodedCall{Call: call, Input: input}, nil
}

func (incidentControlTool) Decode(call Call) (DecodedCall, error) {
	input := IncidentControlInput{
		IncidentID: strings.TrimSpace(call.StringInput("incident_id")),
		Action:     types.IncidentControlAction(strings.TrimSpace(call.StringInput("action"))),
	}
	if input.IncidentID == "" {
		return DecodedCall{}, fmt.Errorf("incident_id is required")
	}
	if input.Action == "" {
		return DecodedCall{}, fmt.Errorf("action is required")
	}
	switch input.Action {
	case types.IncidentControlActionAck,
		types.IncidentControlActionClose,
		types.IncidentControlActionReopen,
		types.IncidentControlActionEscalate:
	default:
		return DecodedCall{}, fmt.Errorf(`invalid action %q; must be one of ack, close, reopen, escalate`, input.Action)
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
	status := types.AutomationIncidentStatus(strings.TrimSpace(call.StringInput("status")))
	if status != "" {
		switch status {
		case types.AutomationIncidentStatusOpen,
			types.AutomationIncidentStatusSuppressed,
			types.AutomationIncidentStatusQueued,
			types.AutomationIncidentStatusActive,
			types.AutomationIncidentStatusMonitoring,
			types.AutomationIncidentStatusResolved,
			types.AutomationIncidentStatusEscalated,
			types.AutomationIncidentStatusFailed,
			types.AutomationIncidentStatusCanceled,
			types.AutomationIncidentStatusClosed:
		default:
			return DecodedCall{}, fmt.Errorf(`invalid status %q; must be one of open, suppressed, queued, active, monitoring, resolved, escalated, failed, canceled, closed`, status)
		}
	}
	input := IncidentListInput{
		WorkspaceRoot: strings.TrimSpace(call.StringInput("workspace_root")),
		AutomationID:  strings.TrimSpace(call.StringInput("automation_id")),
		Status:        status,
		Limit:         limit,
	}
	return DecodedCall{Call: call, Input: input}, nil
}

func (t incidentAckTool) Execute(ctx context.Context, call Call, execCtx ExecContext) (Result, error) {
	decoded, err := t.Decode(call)
	if err != nil {
		return Result{}, err
	}
	output, err := t.ExecuteDecoded(ctx, decoded, execCtx)
	return output.Result, err
}

func (t incidentControlTool) Execute(ctx context.Context, call Call, execCtx ExecContext) (Result, error) {
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

func (incidentAckTool) ExecuteDecoded(ctx context.Context, decoded DecodedCall, execCtx ExecContext) (ToolExecutionResult, error) {
	service, err := requireAutomationService(execCtx)
	if err != nil {
		return ToolExecutionResult{}, err
	}
	input, _ := decoded.Input.(IncidentAckInput)
	incident, ok, err := service.ControlIncident(ctx, input.IncidentID, types.IncidentControlActionAck)
	if err != nil {
		return ToolExecutionResult{}, err
	}
	if !ok {
		return ToolExecutionResult{}, fmt.Errorf("incident %q not found", input.IncidentID)
	}
	return ToolExecutionResult{
		Result: Result{Text: mustJSON(incident), ModelText: mustJSON(incident)},
		Data:   incident,
	}, nil
}

func (incidentControlTool) ExecuteDecoded(ctx context.Context, decoded DecodedCall, execCtx ExecContext) (ToolExecutionResult, error) {
	service, err := requireAutomationService(execCtx)
	if err != nil {
		return ToolExecutionResult{}, err
	}
	input, _ := decoded.Input.(IncidentControlInput)
	incident, ok, err := service.ControlIncident(ctx, input.IncidentID, input.Action)
	if err != nil {
		return ToolExecutionResult{}, err
	}
	if !ok {
		return ToolExecutionResult{}, fmt.Errorf("incident %q not found", input.IncidentID)
	}
	return ToolExecutionResult{
		Result: Result{Text: mustJSON(incident), ModelText: mustJSON(incident)},
		Data:   incident,
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
		Result: Result{Text: mustJSON(incident), ModelText: mustJSON(incident)},
		Data:   incident,
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
		Result: Result{Text: mustJSON(output), ModelText: mustJSON(output)},
		Data:   output,
	}, nil
}

func (incidentAckTool) MapModelResult(output ToolExecutionResult) ModelToolResult {
	return defaultStructuredModelResult(output)
}

func (incidentControlTool) MapModelResult(output ToolExecutionResult) ModelToolResult {
	return defaultStructuredModelResult(output)
}

func (incidentGetTool) MapModelResult(output ToolExecutionResult) ModelToolResult {
	return defaultStructuredModelResult(output)
}

func (incidentListTool) MapModelResult(output ToolExecutionResult) ModelToolResult {
	return defaultStructuredModelResult(output)
}
