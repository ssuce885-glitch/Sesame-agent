package tools

import (
	"context"
	"fmt"
	"strings"

	"go-agent/internal/automation"
	rolectx "go-agent/internal/roles"
	"go-agent/internal/types"
)

type automationCreateSimpleTool struct{}

func (automationCreateSimpleTool) IsEnabled(execCtx ExecContext) bool {
	return execCtx.AutomationService != nil &&
		!isAutomationOwnerTaskMode(execCtx) &&
		hasActiveSkills(execCtx, "automation-standard-behavior", "automation-normalizer")
}

func (automationCreateSimpleTool) IsConcurrencySafe() bool { return false }

func (automationCreateSimpleTool) Definition() Definition {
	return Definition{
		Name:        "automation_create_simple",
		Description: "Preferred high-level builder for role-owned watcher automation: role watcher script -> pause watcher after dispatch -> owner role task -> main_agent report -> policy-driven watcher resume. Compiles and persists a valid simple-mode automation spec.",
		InputSchema: objectSchema(map[string]any{
			"automation_id": map[string]any{
				"type":        "string",
				"description": "Automation identifier to persist.",
			},
			"owner": map[string]any{
				"type":        "string",
				"description": "Owner role for automation source and task dispatch. Must be role:<role_id>, and this tool must be called from that owning specialist role session.",
			},
			"watch_script": map[string]any{
				"type":        "string",
				"description": "Shell command/script that emits one detector JSON object for trigger_on=script_status. The runtime pauses the watcher after a needs_agent dispatch and resumes it after the owner task according to simple_policy. No match example: {\"status\":\"healthy\",\"summary\":\"no files found\",\"facts\":{\"count\":0}}. Match example: {\"status\":\"needs_agent\",\"summary\":\"found files to clean\",\"facts\":{\"count\":2}}. Do not output triggered/found/NO_MATCH or wrap inside script_status.",
			},
			"interval_seconds": map[string]any{
				"type":        "integer",
				"description": "Watcher poll interval in seconds.",
			},
			"title": map[string]any{
				"type": "string",
			},
			"goal": map[string]any{
				"type": "string",
			},
			"timeout_seconds": map[string]any{
				"type":        "integer",
				"description": "Optional watcher command timeout in seconds (default 30).",
			},
			"simple_policy": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"on_success": map[string]any{
						"type": "string",
						"enum": []string{"continue", "pause", "escalate"},
					},
					"on_failure": map[string]any{
						"type": "string",
						"enum": []string{"continue", "pause", "escalate"},
					},
					"on_blocked": map[string]any{
						"type": "string",
						"enum": []string{"continue", "pause", "escalate"},
					},
				},
				"additionalProperties": false,
			},
		}, "automation_id", "owner", "watch_script", "interval_seconds"),
		OutputSchema: automationSpecOutputSchema(),
	}
}

func (automationCreateSimpleTool) Decode(call Call) (DecodedCall, error) {
	var input types.SimpleAutomationBuilderInput
	if err := decodeAutomationJSON(call.Input, &input); err != nil {
		return DecodedCall{}, fmt.Errorf("input must be a valid simple automation builder payload: %w", err)
	}
	input.AutomationID = strings.TrimSpace(input.AutomationID)
	input.Owner = strings.TrimSpace(input.Owner)
	input.WatchScript = strings.TrimSpace(input.WatchScript)
	input.ReportTarget = strings.TrimSpace(input.ReportTarget)
	input.EscalationTarget = strings.TrimSpace(input.EscalationTarget)
	input.Title = strings.TrimSpace(input.Title)
	input.Goal = strings.TrimSpace(input.Goal)
	return DecodedCall{
		Call:  call,
		Input: input,
	}, nil
}

func (t automationCreateSimpleTool) Execute(ctx context.Context, call Call, execCtx ExecContext) (Result, error) {
	decoded, err := t.Decode(call)
	if err != nil {
		return Result{}, err
	}
	output, err := t.ExecuteDecoded(ctx, decoded, execCtx)
	return output.Result, err
}

func (automationCreateSimpleTool) ExecuteDecoded(ctx context.Context, decoded DecodedCall, execCtx ExecContext) (ToolExecutionResult, error) {
	service, err := requireAutomationService(execCtx)
	if err != nil {
		return ToolExecutionResult{}, err
	}
	if err := rejectAutomationDefinitionMutationFromOwnerTask(execCtx); err != nil {
		return ToolExecutionResult{}, err
	}
	if err := requireActiveSkills(execCtx, "automation-standard-behavior", "automation-normalizer"); err != nil {
		return ToolExecutionResult{}, err
	}
	input, _ := decoded.Input.(types.SimpleAutomationBuilderInput)
	if err := requireOwningRoleAutomationContext(ctx, input.Owner); err != nil {
		return ToolExecutionResult{}, err
	}
	if _, err := automation.CompileSimpleAutomationBuilder(input, execCtx.WorkspaceRoot); err != nil {
		return ToolExecutionResult{}, err
	}
	// Validate the user-provided watcher before writing role-bound source files.
	if err := automation.ValidateWatcherContract(ctx, input.WatchScript, execCtx.WorkspaceRoot); err != nil {
		return ToolExecutionResult{}, err
	}
	layout, err := automation.MaterializeRoleBoundSimpleAutomationSource(execCtx.WorkspaceRoot, input)
	if err != nil {
		return ToolExecutionResult{}, err
	}
	input.WatchScript = layout.Selector
	spec, err := automation.CompileSimpleAutomationBuilder(input, execCtx.WorkspaceRoot)
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

func rejectAutomationDefinitionMutationFromOwnerTask(execCtx ExecContext) error {
	if !isAutomationOwnerTaskMode(execCtx) {
		return nil
	}
	return fmt.Errorf("automation_create_simple is not allowed in Owner Task Mode; execute the automation_goal and report the result instead of creating or modifying automation definitions")
}

func isAutomationOwnerTaskMode(execCtx ExecContext) bool {
	if execCtx.TaskManager == nil || execCtx.TurnContext == nil {
		return false
	}
	taskID := strings.TrimSpace(execCtx.TurnContext.CurrentTaskID)
	if taskID == "" {
		return false
	}
	currentTask, err := getTaskForWorkspace(execCtx.TaskManager, taskID, execCtx.WorkspaceRoot)
	if err != nil {
		return false
	}
	if !strings.EqualFold(strings.TrimSpace(currentTask.Kind), "automation_simple") {
		return false
	}
	return true
}

func requireOwningRoleAutomationContext(ctx context.Context, owner string) error {
	normalizedOwner := types.NormalizeRoleAutomationOwner(owner)
	if !strings.HasPrefix(normalizedOwner, "role:") {
		return fmt.Errorf("role-owned automation owner must be role:<role_id>")
	}
	ownerRoleID := strings.TrimSpace(strings.TrimPrefix(normalizedOwner, "role:"))
	currentRoleID := strings.TrimSpace(rolectx.SpecialistRoleIDFromContext(ctx))
	if currentRoleID == ownerRoleID {
		return nil
	}
	if currentRoleID == "" {
		return fmt.Errorf("role-owned automation must be created from the owning specialist role session %q; delegate_to_role to that role and have it call automation_create_simple", ownerRoleID)
	}
	return fmt.Errorf("role-owned automation for %q cannot be created from specialist role %q", ownerRoleID, currentRoleID)
}

func (automationCreateSimpleTool) MapModelResult(output ToolExecutionResult) ModelToolResult {
	return defaultStructuredModelResult(output)
}
