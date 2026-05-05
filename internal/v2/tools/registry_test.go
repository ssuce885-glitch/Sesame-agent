package tools

import (
	"context"
	"testing"

	"go-agent/internal/skillcatalog"
	"go-agent/internal/v2/contracts"
)

type namespaceFallbackTool struct{}

func (namespaceFallbackTool) Definition() contracts.ToolDefinition {
	return contracts.ToolDefinition{
		Name:        "ns_fallback",
		Description: "test tool",
	}
}

func (namespaceFallbackTool) Execute(context.Context, contracts.ToolCall, contracts.ExecContext) (contracts.ToolResult, error) {
	return contracts.ToolResult{Ok: true}, nil
}

func TestRegistryVisibleToolsAppliesRegisteredNamespaceFallback(t *testing.T) {
	reg := NewRegistry()
	tool := namespaceFallbackTool{}
	if tool.Definition().Namespace != "" {
		t.Fatalf("expected tool definition namespace to stay empty")
	}

	reg.Register(contracts.NamespaceFiles, tool)
	visible := reg.VisibleTools(contracts.ExecContext{})
	if len(visible) != 1 {
		t.Fatalf("expected one visible tool, got %+v", visible)
	}
	if visible[0].Namespace != contracts.NamespaceFiles {
		t.Fatalf("expected registered namespace fallback, got %+v", visible[0])
	}
	if tool.Definition().Namespace != "" {
		t.Fatalf("expected original tool definition to remain unchanged")
	}
}

func TestRegistryAllToolDefinitionsIncludesGatedTools(t *testing.T) {
	reg := NewRegistry()
	RegisterAllTools(reg, nil, skillcatalog.Catalog{})

	all := reg.AllToolDefinitions()
	visible := reg.VisibleTools(contracts.ExecContext{})

	if !toolDefinitionsContain(all, "automation_create_simple") {
		t.Fatalf("expected automation_create_simple in all tool definitions, got %+v", all)
	}
	if toolDefinitionsContain(visible, "automation_create_simple") {
		t.Fatalf("expected automation_create_simple to stay gated from empty visible set, got %+v", visible)
	}
}

func toolDefinitionsContain(defs []contracts.ToolDefinition, name string) bool {
	for _, def := range defs {
		if def.Name == name {
			return true
		}
	}
	return false
}
