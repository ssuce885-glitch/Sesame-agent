package tools

import (
	"context"
	"fmt"
	"strings"

	"go-agent/internal/types"
)

type automationQueryMode string

const (
	automationQueryModeGet  automationQueryMode = "get"
	automationQueryModeList automationQueryMode = "list"
)

type automationQueryTool struct{}

type AutomationQueryInput struct {
	Mode              automationQueryMode   `json:"mode"`
	AutomationID      string                `json:"automation_id,omitempty"`
	WorkspaceRoot     string                `json:"workspace_root,omitempty"`
	State             types.AutomationState `json:"state,omitempty"`
	Limit             int                   `json:"limit,omitempty"`
	IncludeWatcher    bool                  `json:"include_watcher,omitempty"`
	IncludeHeartbeats bool                  `json:"include_heartbeats,omitempty"`
	HeartbeatLimit    int                   `json:"heartbeat_limit,omitempty"`
}

type AutomationQueryOutput struct {
	Mode        automationQueryMode             `json:"mode"`
	Automation  *types.AutomationSpec           `json:"automation,omitempty"`
	Automations []types.AutomationSpec          `json:"automations,omitempty"`
	Watcher     *types.AutomationWatcherRuntime `json:"watcher,omitempty"`
	Heartbeats  []types.AutomationHeartbeat     `json:"heartbeats,omitempty"`
}

func (automationQueryTool) IsEnabled(execCtx ExecContext) bool {
	return execCtx.AutomationService != nil
}

func (automationQueryTool) Definition() Definition {
	return Definition{
		Name:        "automation_query",
		Description: "Read-only automation query surface. Use mode=get for one automation, or mode=list for a filtered list. In get mode, you can optionally include watcher runtime and recent heartbeats for cheap runtime inspection.",
		InputSchema: objectSchema(map[string]any{
			"mode": map[string]any{
				"type":        "string",
				"enum":        automationQueryModeEnum(),
				"description": "Query mode: get one automation or list many automations.",
			},
			"automation_id": map[string]any{
				"type":        "string",
				"description": "Automation identifier. Required for mode=get.",
			},
			"workspace_root": map[string]any{
				"type":        "string",
				"description": "Optional workspace filter for mode=list.",
			},
			"state": map[string]any{
				"type":        "string",
				"enum":        automationStateEnum(),
				"description": "Optional state filter for mode=list.",
			},
			"limit": map[string]any{
				"type":        "integer",
				"description": "Optional positive result limit for mode=list.",
			},
			"include_watcher": map[string]any{
				"type":        "boolean",
				"description": "When mode=get, include watcher runtime details.",
			},
			"include_heartbeats": map[string]any{
				"type":        "boolean",
				"description": "When mode=get, include recent automation heartbeats.",
			},
			"heartbeat_limit": map[string]any{
				"type":        "integer",
				"description": "Optional positive heartbeat limit for mode=get when include_heartbeats=true.",
			},
		}, "mode"),
		OutputSchema: automationQueryOutputSchema(),
	}
}

func (automationQueryTool) IsConcurrencySafe() bool { return true }

func (automationQueryTool) Decode(call Call) (DecodedCall, error) {
	mode, err := decodeAutomationQueryMode(call.StringInput("mode"))
	if err != nil {
		return DecodedCall{}, err
	}
	limit, err := decodeOptionalPositiveInt(call.Input["limit"])
	if err != nil {
		return DecodedCall{}, fmt.Errorf("limit %w", err)
	}
	heartbeatLimit, err := decodeOptionalPositiveInt(call.Input["heartbeat_limit"])
	if err != nil {
		return DecodedCall{}, fmt.Errorf("heartbeat_limit %w", err)
	}
	state := types.AutomationState(strings.TrimSpace(call.StringInput("state")))
	if state != "" {
		switch state {
		case types.AutomationStateActive, types.AutomationStatePaused:
		default:
			return DecodedCall{}, fmt.Errorf(`invalid state %q; must be one of active, paused`, state)
		}
	}
	input := AutomationQueryInput{
		Mode:              mode,
		AutomationID:      strings.TrimSpace(call.StringInput("automation_id")),
		WorkspaceRoot:     strings.TrimSpace(call.StringInput("workspace_root")),
		State:             state,
		Limit:             limit,
		IncludeWatcher:    decodeOptionalBool(call.Input["include_watcher"]),
		IncludeHeartbeats: decodeOptionalBool(call.Input["include_heartbeats"]),
		HeartbeatLimit:    heartbeatLimit,
	}
	switch input.Mode {
	case automationQueryModeGet:
		if input.AutomationID == "" {
			return DecodedCall{}, fmt.Errorf("automation_id is required for mode=get")
		}
	case automationQueryModeList:
		if input.IncludeWatcher || input.IncludeHeartbeats || input.HeartbeatLimit > 0 {
			return DecodedCall{}, fmt.Errorf("include_watcher, include_heartbeats, and heartbeat_limit are only supported for mode=get")
		}
	}
	return DecodedCall{Call: call, Input: input}, nil
}

func (t automationQueryTool) Execute(ctx context.Context, call Call, execCtx ExecContext) (Result, error) {
	decoded, err := t.Decode(call)
	if err != nil {
		return Result{}, err
	}
	output, err := t.ExecuteDecoded(ctx, decoded, execCtx)
	return output.Result, err
}

func (automationQueryTool) ExecuteDecoded(ctx context.Context, decoded DecodedCall, execCtx ExecContext) (ToolExecutionResult, error) {
	service, err := requireAutomationService(execCtx)
	if err != nil {
		return ToolExecutionResult{}, err
	}
	input, _ := decoded.Input.(AutomationQueryInput)
	switch input.Mode {
	case automationQueryModeGet:
		spec, ok, err := service.Get(ctx, input.AutomationID)
		if err != nil {
			return ToolExecutionResult{}, err
		}
		if !ok {
			return ToolExecutionResult{}, fmt.Errorf("automation %q not found", input.AutomationID)
		}
		output := AutomationQueryOutput{
			Mode:       input.Mode,
			Automation: &spec,
		}
		if input.IncludeWatcher {
			watcher, ok, err := service.GetWatcher(ctx, input.AutomationID)
			if err != nil {
				return ToolExecutionResult{}, err
			}
			if ok {
				output.Watcher = &watcher
			}
		}
		if input.IncludeHeartbeats {
			heartbeats, err := service.ListHeartbeats(ctx, types.AutomationHeartbeatFilter{
				AutomationID: input.AutomationID,
				Limit:        input.HeartbeatLimit,
			})
			if err != nil {
				return ToolExecutionResult{}, err
			}
			output.Heartbeats = heartbeats
		}
		return ToolExecutionResult{
			Result: Result{Text: mustJSON(output), ModelText: mustJSON(output)},
			Data:   output,
		}, nil
	case automationQueryModeList:
		automations, err := service.List(ctx, types.AutomationListFilter{
			WorkspaceRoot: input.WorkspaceRoot,
			State:         input.State,
			Limit:         input.Limit,
		})
		if err != nil {
			return ToolExecutionResult{}, err
		}
		output := AutomationQueryOutput{
			Mode:        input.Mode,
			Automations: automations,
		}
		return ToolExecutionResult{
			Result: Result{Text: mustJSON(output), ModelText: mustJSON(output)},
			Data:   output,
		}, nil
	default:
		return ToolExecutionResult{}, fmt.Errorf("unsupported mode %q", input.Mode)
	}
}

func (automationQueryTool) MapModelResult(output ToolExecutionResult) ModelToolResult {
	return defaultStructuredModelResult(output)
}

func automationQueryModeEnum() []string {
	return []string{string(automationQueryModeGet), string(automationQueryModeList)}
}

func decodeAutomationQueryMode(raw string) (automationQueryMode, error) {
	mode := automationQueryMode(strings.ToLower(strings.TrimSpace(raw)))
	switch mode {
	case automationQueryModeGet, automationQueryModeList:
		return mode, nil
	default:
		return "", fmt.Errorf(`invalid mode %q; must be one of get, list`, strings.TrimSpace(raw))
	}
}

func decodeOptionalBool(raw any) bool {
	value, _ := raw.(bool)
	return value
}

func automationQueryOutputSchema() map[string]any {
	return objectSchema(map[string]any{
		"mode": map[string]any{
			"type": "string",
			"enum": automationQueryModeEnum(),
		},
		"automation": automationSpecOutputSchema(),
		"automations": map[string]any{
			"type":  "array",
			"items": automationSpecOutputSchema(),
		},
		"watcher": automationWatcherOutputSchema(),
		"heartbeats": map[string]any{
			"type":  "array",
			"items": automationHeartbeatOutputSchema(),
		},
	}, "mode")
}

func automationWatcherOutputSchema() map[string]any {
	return objectSchema(map[string]any{
		"id":              map[string]any{"type": "string"},
		"automation_id":   map[string]any{"type": "string"},
		"workspace_root":  map[string]any{"type": "string"},
		"watcher_id":      map[string]any{"type": "string"},
		"state":           map[string]any{"type": "string"},
		"effective_state": map[string]any{"type": "string"},
		"task_id":         map[string]any{"type": "string"},
		"command":         map[string]any{"type": "string"},
		"last_error":      map[string]any{"type": "string"},
		"created_at":      map[string]any{"type": "string"},
		"updated_at":      map[string]any{"type": "string"},
	}, "automation_id", "watcher_id", "state")
}

func automationHeartbeatOutputSchema() map[string]any {
	return objectSchema(map[string]any{
		"automation_id":  map[string]any{"type": "string"},
		"watcher_id":     map[string]any{"type": "string"},
		"workspace_root": map[string]any{"type": "string"},
		"status":         map[string]any{"type": "string"},
		"payload":        map[string]any{},
		"observed_at":    map[string]any{"type": "string"},
		"created_at":     map[string]any{"type": "string"},
		"updated_at":     map[string]any{"type": "string"},
	}, "automation_id", "watcher_id")
}
