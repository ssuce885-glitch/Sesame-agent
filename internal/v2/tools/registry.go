package tools

import (
	"sort"
	"sync"

	"go-agent/internal/v2/contracts"
)

type Registry struct {
	mu           sync.RWMutex
	tools        map[string]contracts.Tool
	registeredNS map[string]contracts.ToolNamespace
}

type enabledTool interface {
	IsEnabled(execCtx contracts.ExecContext) bool
}

func NewRegistry() *Registry {
	return &Registry{
		tools:        make(map[string]contracts.Tool),
		registeredNS: make(map[string]contracts.ToolNamespace),
	}
}

var _ contracts.ToolRegistry = (*Registry)(nil)

func (r *Registry) Register(ns contracts.ToolNamespace, tool contracts.Tool) {
	if tool == nil {
		return
	}
	def := tool.Definition()
	if def.Name == "" {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.tools[def.Name] = tool
	r.registeredNS[def.Name] = ns
}

func (r *Registry) Lookup(name string) (contracts.Tool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	tool, ok := r.tools[name]
	return tool, ok
}

func (r *Registry) VisibleTools(execCtx contracts.ExecContext) []contracts.ToolDefinition {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.toolDefinitionsLocked(func(tool contracts.Tool) bool {
		return EvaluateToolAccess(tool, execCtx).Allowed
	})
}

func (r *Registry) AllToolDefinitions() []contracts.ToolDefinition {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.toolDefinitionsLocked(nil)
}

func (r *Registry) toolDefinitionsLocked(include func(contracts.Tool) bool) []contracts.ToolDefinition {
	names := make([]string, 0, len(r.tools))
	for name := range r.tools {
		names = append(names, name)
	}
	sort.Strings(names)

	defs := make([]contracts.ToolDefinition, 0, len(names))
	for _, name := range names {
		tool := r.tools[name]
		if include != nil && !include(tool) {
			continue
		}
		def := tool.Definition()
		if def.Namespace == "" {
			def.Namespace = r.registeredNS[name]
		}
		defs = append(defs, def)
	}
	return defs
}
