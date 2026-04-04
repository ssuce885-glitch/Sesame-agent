package tools

import (
	"context"
	"fmt"
	"sort"

	"go-agent/internal/permissions"
)

type Registry struct {
	tools       map[string]Tool
	definitions map[string]Definition
}

func NewRegistry() *Registry {
	r := &Registry{
		tools:       make(map[string]Tool),
		definitions: make(map[string]Definition),
	}
	r.Register(fileReadTool{})
	r.Register(fileWriteTool{})
	r.Register(globTool{})
	r.Register(grepTool{})
	r.Register(shellTool{})

	return r
}

func (r *Registry) Register(tool Tool) {
	if r.tools == nil {
		r.tools = make(map[string]Tool)
	}
	if r.definitions == nil {
		r.definitions = make(map[string]Definition)
	}

	def := tool.Definition()
	r.tools[def.Name] = tool
	r.definitions[def.Name] = def
}

func (r *Registry) Definitions() []Definition {
	names := make([]string, 0, len(r.definitions))
	for name := range r.definitions {
		names = append(names, name)
	}
	sort.Strings(names)

	defs := make([]Definition, 0, len(names))
	for _, name := range names {
		defs = append(defs, r.definitions[name])
	}
	return defs
}

func (r *Registry) Execute(ctx context.Context, call Call, execCtx ExecContext) (Result, error) {
	tool, ok := r.tools[call.Name]
	if !ok {
		return Result{}, fmt.Errorf("unknown tool %q", call.Name)
	}

	if execCtx.PermissionEngine != nil {
		switch execCtx.PermissionEngine.Decide(call.Name) {
		case permissions.DecisionAllow:
		case permissions.DecisionAsk:
			return Result{}, fmt.Errorf("tool %q requires approval", call.Name)
		case permissions.DecisionDeny:
			return Result{}, fmt.Errorf("tool %q denied", call.Name)
		}
	}

	return tool.Execute(ctx, call, execCtx)
}
