package tools

import (
	"context"
	"fmt"
	"strings"

	"go-agent/internal/session"
)

type delegateToRoleTool struct{}

type DelegateToRoleInput struct {
	TargetRole string `json:"target_role"`
	Message    string `json:"message"`
	Reason     string `json:"reason,omitempty"`
}

type DelegateToRoleOutput struct {
	TaskID     string `json:"task_id"`
	TargetRole string `json:"target_role"`
	Accepted   bool   `json:"accepted"`
}

func (delegateToRoleTool) IsEnabled(execCtx ExecContext) bool {
	return execCtx.SessionDelegationService != nil && execCtx.TurnContext != nil && strings.TrimSpace(currentSessionID(execCtx)) != ""
}

func (delegateToRoleTool) Definition() Definition {
	return Definition{
		Name:        "delegate_to_role",
		Description: "Hand off work from main_parent to an installed specialist role task. After calling this tool, read the returned task_id, confirm the delegation in one sentence, then end the turn. Do not wait, sleep, poll, or inspect the delegated task — its result returns through reports.",
		InputSchema: objectSchema(map[string]any{
			"target_role": map[string]any{
				"type":        "string",
				"description": "The installed specialist role id that should own the work.",
			},
			"message": map[string]any{
				"type":        "string",
				"description": "The delegated work item for the target role task.",
			},
			"reason": map[string]any{
				"type":        "string",
				"description": "Optional short reason for the handoff.",
			},
		}, "target_role", "message"),
		OutputSchema: objectSchema(map[string]any{
			"task_id":     map[string]any{"type": "string"},
			"target_role": map[string]any{"type": "string"},
			"accepted":    map[string]any{"type": "boolean"},
		}, "task_id", "target_role", "accepted"),
	}
}

func (delegateToRoleTool) IsConcurrencySafe() bool { return false }

func (delegateToRoleTool) CompletesTurnOnSuccess() bool { return false }

func (delegateToRoleTool) Decode(call Call) (DecodedCall, error) {
	targetRole, err := validateRoleID(call.StringInput("target_role"))
	if err != nil {
		return DecodedCall{}, err
	}
	input := DelegateToRoleInput{
		TargetRole: targetRole,
		Message:    strings.TrimSpace(call.StringInput("message")),
		Reason:     strings.TrimSpace(call.StringInput("reason")),
	}
	if input.Message == "" {
		return DecodedCall{}, fmt.Errorf("message is required")
	}
	return DecodedCall{
		Call: Call{
			Name: call.Name,
			Input: map[string]any{
				"target_role": input.TargetRole,
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
	if execCtx.RoleSpec != nil && execCtx.RoleSpec.Policy != nil && execCtx.RoleSpec.Policy.CanDelegate != nil && !*execCtx.RoleSpec.Policy.CanDelegate {
		return ToolExecutionResult{}, fmt.Errorf("role %s cannot delegate to other roles", strings.TrimSpace(execCtx.RoleSpec.RoleID))
	}
	input, _ := decoded.Input.(DelegateToRoleInput)
	out, err := execCtx.SessionDelegationService.DelegateToRole(ctx, session.DelegateToRoleInput{
		WorkspaceRoot:   execCtx.WorkspaceRoot,
		SourceSessionID: currentSessionID(execCtx),
		SourceTurnID:    currentTurnID(execCtx),
		TargetRole:      input.TargetRole,
		Message:         input.Message,
		Reason:          input.Reason,
	})
	if err != nil {
		return ToolExecutionResult{}, err
	}
	output := DelegateToRoleOutput{
		TaskID:     out.TaskID,
		TargetRole: out.TargetRole,
		Accepted:   out.Accepted,
	}
	text := mustJSON(output)
	modelText := fmt.Sprintf(
		"Work has been delegated to %s (background task %s). The result will return through a report.",
		output.TargetRole,
		output.TaskID,
	)
	return ToolExecutionResult{
		Result: Result{
			Text:      text,
			ModelText: modelText,
		},
		Data:        output,
		PreviewText: fmt.Sprintf("Delegated to %s", output.TargetRole),
		Metadata: map[string]any{
			"delegated_task_id": output.TaskID,
			"target_role":       output.TargetRole,
			"turn_handoff":      true,
		},
		CompleteTurn: false,
	}, nil
}

func (delegateToRoleTool) MapModelResult(output ToolExecutionResult) ModelToolResult {
	return defaultStructuredModelResult(output)
}

func validateRoleID(raw string) (string, error) {
	roleID := strings.TrimSpace(raw)
	if roleID == "" {
		return "", fmt.Errorf("target_role is required")
	}
	if strings.HasPrefix(roleID, ".") || strings.Contains(roleID, "/") || strings.Contains(roleID, "\\") || strings.Contains(roleID, "..") {
		return "", fmt.Errorf("invalid target_role %q", roleID)
	}
	return roleID, nil
}
