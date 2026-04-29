package tools

import (
	"context"
	"fmt"
	"strings"

	"go-agent/internal/permissions"
)

func (r *Registry) Execute(ctx context.Context, call Call, execCtx ExecContext) (Result, error) {
	output, err := r.executePreparedRich(ctx, r.prepareCall(call), execCtx)
	return output.Result, err
}

func (r *Registry) executeResolved(ctx context.Context, tool Tool, def Definition, resolvedName string, call Call, execCtx ExecContext) (Result, error) {
	output, err := r.executePreparedRich(ctx, r.prepareResolvedCall(tool, def, resolvedName, call), execCtx)
	return output.Result, err
}

func (r *Registry) prepareCall(call Call) PreparedCall {
	tool, def, resolvedName, ok := r.lookup(call.Name)
	if !ok {
		return PreparedCall{
			Original:   call,
			Call:       call,
			PrepareErr: fmt.Errorf("unknown tool %q", call.Name),
		}
	}
	return r.prepareResolvedCall(tool, def, resolvedName, call)
}

func (r *Registry) prepareResolvedCall(tool Tool, def Definition, resolvedName string, call Call) PreparedCall {
	if call.Input == nil {
		call.Input = map[string]any{}
	}
	prepared := PreparedCall{
		Original:     call,
		Call:         call,
		ResolvedName: resolvedName,
		Tool:         tool,
		Definition:   def,
	}
	prepared.Call.Name = resolvedName
	if err := validateInputSchema(def.InputSchema, call.Input); err != nil {
		prepared.PrepareErr = err
		return prepared
	}
	decoded, err := decodeToolCall(tool, prepared.Call)
	if err != nil {
		prepared.PrepareErr = err
		return prepared
	}
	prepared.Call = decoded.Call
	prepared.Decoded = decoded
	return prepared
}

func (r *Registry) executePreparedRich(ctx context.Context, prepared PreparedCall, execCtx ExecContext) (ToolExecutionResult, error) {
	if prepared.PrepareErr != nil {
		return ToolExecutionResult{}, prepared.PrepareErr
	}
	if err := checkRoleToolPolicy(prepared.ResolvedName, prepared.Definition, execCtx); err != nil {
		return ToolExecutionResult{}, err
	}
	interrupt, err := checkToolPermission(ctx, prepared.Tool, prepared.ResolvedName, prepared.Decoded, execCtx)
	if err != nil {
		return ToolExecutionResult{}, err
	}
	if interrupt != nil {
		return *interrupt, nil
	}
	return executePreparedTool(ctx, prepared, execCtx)
}

func decodeToolCall(tool Tool, call Call) (DecodedCall, error) {
	if call.Input == nil {
		call.Input = map[string]any{}
	}
	if decoder, ok := tool.(decodingTool); ok {
		decoded, err := decoder.Decode(call)
		if err != nil {
			return DecodedCall{}, err
		}
		if decoded.Call.Name == "" {
			decoded.Call.Name = call.Name
		}
		if decoded.Call.ID == "" {
			decoded.Call.ID = call.ID
		}
		if decoded.Call.Input == nil {
			decoded.Call.Input = call.Input
		}
		if decoded.Input == nil {
			decoded.Input = decoded.Call.Input
		}
		return decoded, nil
	}
	return DecodedCall{
		Call:  call,
		Input: call.Input,
	}, nil
}

func executePreparedTool(ctx context.Context, prepared PreparedCall, execCtx ExecContext) (ToolExecutionResult, error) {
	if !toolEnabled(prepared.Tool, execCtx) {
		return ToolExecutionResult{}, fmt.Errorf("tool %q is not enabled in the current context", prepared.ResolvedName)
	}
	if executor, ok := prepared.Tool.(decodedExecutor); ok {
		return executor.ExecuteDecoded(ctx, prepared.Decoded, execCtx)
	}
	result, err := prepared.Tool.Execute(ctx, prepared.Decoded.Call, execCtx)
	if err != nil {
		return ToolExecutionResult{}, err
	}
	return ToolExecutionResult{Result: result}, nil
}

func mapToolModelResult(tool Tool, output ToolExecutionResult) ModelToolResult {
	if mapper, ok := tool.(modelResultMapper); ok {
		return mapper.MapModelResult(output)
	}

	text := strings.TrimSpace(output.ModelText)
	if text == "" {
		text = output.Text
	}
	return ModelToolResult{
		Text:       text,
		Structured: output.Data,
	}
}

func checkToolPermission(ctx context.Context, tool Tool, resolvedName string, decoded DecodedCall, execCtx ExecContext) (*ToolExecutionResult, error) {
	if execCtx.PermissionEngine != nil {
		switch execCtx.PermissionEngine.Decide(resolvedName) {
		case permissions.DecisionAllow:
			if execCtx.PermissionEngine.AllowsAll() {
				return nil, nil
			}
		case permissions.DecisionDeny:
			return nil, permissionDecisionError(resolvedName, permissions.DecisionDeny, "")
		}
	}
	if checker, ok := tool.(permissionAwareTool); ok {
		decision, reason, err := checker.CheckPermission(ctx, decoded, execCtx)
		if err != nil {
			return nil, err
		}
		switch decision {
		case permissions.DecisionAllow:
			return nil, nil
		case permissions.DecisionDeny:
			return nil, permissionDecisionError(resolvedName, decision, reason)
		}
	}
	return nil, nil
}

func checkRoleToolPolicy(toolName string, def Definition, execCtx ExecContext) error {
	if execCtx.RoleSpec == nil || execCtx.RoleSpec.Policy == nil {
		return nil
	}
	policy := execCtx.RoleSpec.Policy
	toolName = strings.TrimSpace(toolName)
	if toolName == "" {
		return nil
	}
	if len(policy.DeniedTools) > 0 && toolDefinitionInList(toolName, def, policy.DeniedTools) {
		return fmt.Errorf("tool %q denied by role policy denied_tools", toolName)
	}
	return nil
}

func toolDefinitionInList(toolName string, def Definition, names []string) bool {
	wanted := map[string]struct{}{}
	if name := strings.ToLower(strings.TrimSpace(toolName)); name != "" {
		wanted[name] = struct{}{}
	}
	if name := strings.ToLower(strings.TrimSpace(def.Name)); name != "" {
		wanted[name] = struct{}{}
	}
	for _, alias := range def.Aliases {
		if name := strings.ToLower(strings.TrimSpace(alias)); name != "" {
			wanted[name] = struct{}{}
		}
	}
	if len(wanted) == 0 {
		return false
	}
	for _, name := range names {
		if _, ok := wanted[strings.ToLower(strings.TrimSpace(name))]; ok {
			return true
		}
	}
	return false
}

func toolConcurrencySafe(tool Tool, decoded DecodedCall, execCtx ExecContext) bool {
	if tool == nil {
		return false
	}
	if aware, ok := tool.(concurrencyAwareTool); ok {
		return aware.IsConcurrencySafeCall(decoded, execCtx)
	}
	return tool.IsConcurrencySafe()
}

func permissionDecisionError(toolName string, decision permissions.Decision, reason string) error {
	message := fmt.Sprintf("tool %q denied", toolName)
	switch decision {
	case permissions.DecisionDeny:
		message = fmt.Sprintf("tool %q denied", toolName)
	}
	if reason = strings.TrimSpace(reason); reason != "" {
		message += ": " + reason
	}
	return fmt.Errorf("%s", message)
}

func toolEnabled(tool Tool, execCtx ExecContext) bool {
	if tool == nil {
		return false
	}
	enabledTool, ok := tool.(enablableTool)
	if !ok {
		return true
	}
	return enabledTool.IsEnabled(execCtx)
}
