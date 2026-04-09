package tools

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"go-agent/internal/permissions"
)

type Registry struct {
	tools       map[string]Tool
	aliases     map[string]string
	definitions map[string]Definition
}

func NewRegistry() *Registry {
	r := &Registry{
		tools:       make(map[string]Tool),
		aliases:     make(map[string]string),
		definitions: make(map[string]Definition),
	}
	r.Register(enterPlanModeTool{})
	r.Register(exitPlanModeTool{})
	r.Register(fileReadTool{})
	r.Register(fileWriteTool{})
	r.Register(fileEditTool{})
	r.Register(applyPatchTool{})
	r.Register(globTool{})
	r.Register(grepTool{})
	r.Register(listDirTool{})
	r.Register(notebookEditTool{})
	r.Register(requestPermissionsTool{})
	r.Register(requestUserInputTool{})
	r.Register(shellTool{})
	r.Register(taskCreateTool{})
	r.Register(taskGetTool{})
	r.Register(taskListTool{})
	r.Register(taskOutputTool{})
	r.Register(taskStopTool{})
	r.Register(taskUpdateTool{})
	r.Register(todoWriteTool{})
	r.Register(viewImageTool{})
	r.Register(webFetchTool{})
	r.Register(enterWorktreeTool{})
	r.Register(exitWorktreeTool{})

	return r
}

func (r *Registry) Register(tool Tool) {
	if r.tools == nil {
		r.tools = make(map[string]Tool)
	}
	if r.aliases == nil {
		r.aliases = make(map[string]string)
	}
	if r.definitions == nil {
		r.definitions = make(map[string]Definition)
	}

	def := tool.Definition()
	r.tools[def.Name] = tool
	r.definitions[def.Name] = cloneDefinition(def)
	for _, alias := range def.Aliases {
		alias = strings.TrimSpace(alias)
		if alias == "" || alias == def.Name {
			continue
		}
		r.aliases[alias] = def.Name
	}
}

func (r *Registry) Definitions() []Definition {
	names := make([]string, 0, len(r.definitions))
	for name := range r.definitions {
		names = append(names, name)
	}
	sort.Strings(names)

	defs := make([]Definition, 0, len(names))
	for _, name := range names {
		defs = append(defs, cloneDefinition(r.definitions[name]))
	}
	return defs
}

func (r *Registry) VisibleDefinitions(execCtx ExecContext) []Definition {
	names := make([]string, 0, len(r.definitions))
	for name := range r.definitions {
		names = append(names, name)
	}
	sort.Strings(names)

	defs := make([]Definition, 0, len(names))
	for _, name := range names {
		tool := r.tools[name]
		if !toolEnabled(tool, execCtx) {
			continue
		}
		if execCtx.PermissionEngine != nil {
			switch execCtx.PermissionEngine.Decide(name) {
			case permissions.DecisionAllow:
			case permissions.DecisionAsk, permissions.DecisionDeny:
				continue
			}
		}
		defs = append(defs, cloneDefinition(r.definitions[name]))
	}
	return defs
}

func (r *Registry) Execute(ctx context.Context, call Call, execCtx ExecContext) (Result, error) {
	output, err := r.executePreparedRich(ctx, r.prepareCall(call), execCtx)
	return output.Result, err
}

func (r *Registry) lookup(name string) (Tool, Definition, string, bool) {
	if r == nil {
		return nil, Definition{}, "", false
	}
	if tool, ok := r.tools[name]; ok {
		def, ok := r.definitions[name]
		if !ok {
			return nil, Definition{}, "", false
		}
		return tool, cloneDefinition(def), name, true
	}
	canonical, ok := r.aliases[name]
	if !ok {
		return nil, Definition{}, "", false
	}
	tool, ok := r.tools[canonical]
	if !ok {
		return nil, Definition{}, "", false
	}
	def, ok := r.definitions[canonical]
	if !ok {
		return nil, Definition{}, "", false
	}
	return tool, cloneDefinition(def), canonical, true
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
	if err := checkToolPermission(ctx, prepared.Tool, prepared.ResolvedName, prepared.Decoded, execCtx); err != nil {
		return ToolExecutionResult{}, err
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

func checkToolPermission(ctx context.Context, tool Tool, resolvedName string, decoded DecodedCall, execCtx ExecContext) error {
	if execCtx.PermissionEngine != nil {
		switch execCtx.PermissionEngine.Decide(resolvedName) {
		case permissions.DecisionAllow:
		case permissions.DecisionAsk:
			return permissionDecisionError(resolvedName, permissions.DecisionAsk, "")
		case permissions.DecisionDeny:
			return permissionDecisionError(resolvedName, permissions.DecisionDeny, "")
		}
	}
	if checker, ok := tool.(permissionAwareTool); ok {
		decision, reason, err := checker.CheckPermission(ctx, decoded, execCtx)
		if err != nil {
			return err
		}
		switch decision {
		case permissions.DecisionAllow:
			return nil
		case permissions.DecisionAsk, permissions.DecisionDeny:
			return permissionDecisionError(resolvedName, decision, reason)
		}
	}
	return nil
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
	case permissions.DecisionAsk:
		message = fmt.Sprintf("tool %q requires approval", toolName)
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
