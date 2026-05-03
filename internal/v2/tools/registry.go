package tools

import (
	"sort"
	"sync"

	"go-agent/internal/v2/contracts"
)

type Registry struct {
	mu    sync.RWMutex
	tools map[string]contracts.Tool
	byNS  map[contracts.ToolNamespace][]string
}

type enabledTool interface {
	IsEnabled(execCtx contracts.ExecContext) bool
}

func NewRegistry() *Registry {
	return &Registry{
		tools: make(map[string]contracts.Tool),
		byNS:  make(map[contracts.ToolNamespace][]string),
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
	if def.Namespace == "" {
		def.Namespace = ns
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.tools[def.Name]; !exists {
		r.byNS[ns] = append(r.byNS[ns], def.Name)
		sort.Strings(r.byNS[ns])
	}
	r.tools[def.Name] = tool
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

	names := make([]string, 0, len(r.tools))
	for name := range r.tools {
		names = append(names, name)
	}
	sort.Strings(names)

	defs := make([]contracts.ToolDefinition, 0, len(names))
	for _, name := range names {
		tool := r.tools[name]
		if gated, ok := tool.(enabledTool); ok && !gated.IsEnabled(execCtx) {
			continue
		}
		if isDeniedToolForRole(execCtx.RoleSpec, name) {
			continue
		}
		defs = append(defs, tool.Definition())
	}
	return defs
}

func isDeniedToolForRole(spec *contracts.RoleSpec, toolName string) bool {
	if spec == nil {
		return false
	}
	if len(spec.AllowedTools) > 0 && !stringSliceContains(spec.AllowedTools, toolName) {
		return true
	}
	if stringSliceContains(spec.DeniedTools, toolName) {
		return true
	}
	return false
}

func stringSliceContains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
