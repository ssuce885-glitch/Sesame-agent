package tools

import (
	"context"
	"fmt"
	"strings"

	"go-agent/internal/session"
	"go-agent/internal/sessionrole"
	"go-agent/internal/types"
)

type delegateToRoleTool struct{}

type DelegateToRoleInput struct {
	TargetRole string `json:"target_role"`
	Message    string `json:"message"`
	Reason     string `json:"reason,omitempty"`
}

type DelegateToRoleOutput struct {
	TargetRole      string `json:"target_role"`
	TargetSessionID string `json:"target_session_id"`
	TargetTurnID    string `json:"target_turn_id"`
	Accepted        bool   `json:"accepted"`
	CreatedSession  bool   `json:"created_session"`
}

func (delegateToRoleTool) IsEnabled(execCtx ExecContext) bool {
	return execCtx.SessionDelegationService != nil && execCtx.TurnContext != nil && strings.TrimSpace(currentSessionID(execCtx)) != ""
}

func (delegateToRoleTool) Definition() Definition {
	return Definition{
		Name:        "delegate_to_role",
		Description: "Hand off work to a long-lived role session such as monitoring_parent. Use this instead of task_create when transferring responsibility to another parent-role session.",
		InputSchema: objectSchema(map[string]any{
			"target_role": map[string]any{
				"type":        "string",
				"description": "The long-lived role session that should own the work.",
				"enum":        []string{string(types.SessionRoleMainParent), string(types.SessionRoleMonitoringParent)},
			},
			"message": map[string]any{
				"type":        "string",
				"description": "The user-style message to enqueue on the target role session.",
			},
			"reason": map[string]any{
				"type":        "string",
				"description": "Optional short reason for the handoff.",
			},
		}, "target_role", "message"),
		OutputSchema: objectSchema(map[string]any{
			"target_role":       map[string]any{"type": "string"},
			"target_session_id": map[string]any{"type": "string"},
			"target_turn_id":    map[string]any{"type": "string"},
			"accepted":          map[string]any{"type": "boolean"},
			"created_session":   map[string]any{"type": "boolean"},
		}, "target_role", "target_session_id", "target_turn_id", "accepted", "created_session"),
	}
}

func (delegateToRoleTool) IsConcurrencySafe() bool { return false }

func (delegateToRoleTool) Decode(call Call) (DecodedCall, error) {
	input := DelegateToRoleInput{
		TargetRole: strings.TrimSpace(call.StringInput("target_role")),
		Message:    strings.TrimSpace(call.StringInput("message")),
		Reason:     strings.TrimSpace(call.StringInput("reason")),
	}
	if input.TargetRole == "" {
		return DecodedCall{}, fmt.Errorf("target_role is required")
	}
	if input.TargetRole != string(types.SessionRoleMainParent) && input.TargetRole != string(types.SessionRoleMonitoringParent) {
		return DecodedCall{}, fmt.Errorf("invalid target_role %q", input.TargetRole)
	}
	if input.Message == "" {
		return DecodedCall{}, fmt.Errorf("message is required")
	}
	return DecodedCall{
		Call: Call{
			Name: call.Name,
			Input: map[string]any{
				"target_role": string(sessionrole.Normalize(input.TargetRole)),
				"message":     input.Message,
				"reason":      input.Reason,
			},
		},
		Input: input,
	}, nil
}

func (t delegateToRoleTool) Execute(ctx context.Context, call Call, execCtx ExecContext) (Result, error) {
	decoded, err := t.Decode(call)
	if err != nil {
		return Result{}, err
	}
	output, err := t.ExecuteDecoded(ctx, decoded, execCtx)
	return output.Result, err
}

func (delegateToRoleTool) ExecuteDecoded(ctx context.Context, decoded DecodedCall, execCtx ExecContext) (ToolExecutionResult, error) {
	if execCtx.SessionDelegationService == nil {
		return ToolExecutionResult{}, fmt.Errorf("session delegation service is not configured")
	}
	input, _ := decoded.Input.(DelegateToRoleInput)
	out, err := execCtx.SessionDelegationService.DelegateToRole(ctx, session.DelegateToRoleInput{
		WorkspaceRoot:   execCtx.WorkspaceRoot,
		SourceSessionID: currentSessionID(execCtx),
		SourceTurnID:    currentTurnID(execCtx),
		TargetRole:      sessionrole.Normalize(input.TargetRole),
		Message:         input.Message,
		Reason:          input.Reason,
	})
	if err != nil {
		return ToolExecutionResult{}, err
	}
	output := DelegateToRoleOutput{
		TargetRole:      string(out.TargetRole),
		TargetSessionID: out.TargetSessionID,
		TargetTurnID:    out.TargetTurnID,
		Accepted:        out.Accepted,
		CreatedSession:  out.CreatedSession,
	}
	text := mustJSON(output)
	modelText := fmt.Sprintf(
		"Delegated to %s session %s with turn %s. Continue this turn instead of waiting here.",
		output.TargetRole,
		output.TargetSessionID,
		output.TargetTurnID,
	)
	return ToolExecutionResult{
		Result: Result{
			Text:      text,
			ModelText: modelText,
		},
		Data:        output,
		PreviewText: fmt.Sprintf("Delegated to %s", output.TargetRole),
	}, nil
}

func (delegateToRoleTool) MapModelResult(output ToolExecutionResult) ModelToolResult {
	return defaultStructuredModelResult(output)
}
