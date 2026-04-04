package tools

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"go-agent/internal/permissions"
)

type countingTool struct {
	definition Definition
	calls      int
}

func (t *countingTool) Definition() Definition {
	t.calls++
	return t.definition
}

func (t *countingTool) IsConcurrencySafe() bool { return true }

func (t *countingTool) Execute(context.Context, Call, ExecContext) (Result, error) {
	return Result{}, nil
}

func TestFileReadToolRespectsWorkspaceBoundary(t *testing.T) {
	root := t.TempDir()
	allowed := filepath.Join(root, "allowed.txt")
	if err := os.WriteFile(allowed, []byte("hello"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	registry := NewRegistry()
	result, err := registry.Execute(context.Background(), Call{
		Name:  "file_read",
		Input: map[string]any{"path": allowed},
	}, ExecContext{
		WorkspaceRoot:    root,
		PermissionEngine: permissions.NewEngine(),
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Text != "hello" {
		t.Fatalf("result.Text = %q, want %q", result.Text, "hello")
	}
}

func TestRegistryDefinitionsExposeLocalToolSchemas(t *testing.T) {
	registry := NewRegistry()

	defs := registry.Definitions()
	if len(defs) != 5 {
		t.Fatalf("len(Definitions) = %d, want 5", len(defs))
	}

	gotNames := make([]string, 0, len(defs))
	for _, def := range defs {
		gotNames = append(gotNames, def.Name)
	}
	wantNames := []string{"file_read", "file_write", "glob", "grep", "shell_command"}
	if !reflect.DeepEqual(gotNames, wantNames) {
		t.Fatalf("Definitions() names = %v, want %v", gotNames, wantNames)
	}

	defsAgain := registry.Definitions()
	gotNamesAgain := make([]string, 0, len(defsAgain))
	for _, def := range defsAgain {
		gotNamesAgain = append(gotNamesAgain, def.Name)
	}
	if !reflect.DeepEqual(gotNamesAgain, wantNames) {
		t.Fatalf("Definitions() second call names = %v, want %v", gotNamesAgain, wantNames)
	}

	for _, def := range defs {
		if def.Description == "" {
			t.Fatalf("definition = %+v, want description", def)
		}
		if def.InputSchema == nil {
			t.Fatalf("definition = %+v, want schema", def)
		}
		if got, ok := def.InputSchema["type"].(string); !ok || got != "object" {
			t.Fatalf("definition %q schema type = %#v, want object", def.Name, def.InputSchema["type"])
		}
		if got, ok := def.InputSchema["additionalProperties"].(bool); !ok || got {
			t.Fatalf("definition %q additionalProperties = %#v, want false", def.Name, def.InputSchema["additionalProperties"])
		}
	}

	requireSchemaFields := func(name string, required []string, props ...string) {
		t.Helper()

		var def Definition
		found := false
		for _, candidate := range defs {
			if candidate.Name == name {
				def = candidate
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("missing definition %q", name)
		}

		gotRequired, _ := def.InputSchema["required"].([]string)
		if !reflect.DeepEqual(gotRequired, required) {
			t.Fatalf("definition %q required = %v, want %v", name, gotRequired, required)
		}

		properties, _ := def.InputSchema["properties"].(map[string]any)
		for _, prop := range props {
			if _, ok := properties[prop]; !ok {
				t.Fatalf("definition %q properties missing %q", name, prop)
			}
		}
	}

	requireSchemaFields("file_read", []string{"path"}, "path")
	requireSchemaFields("file_write", []string{"path", "content"}, "path", "content")
	requireSchemaFields("glob", []string{"pattern"}, "pattern")
	requireSchemaFields("grep", []string{"path", "pattern"}, "path", "pattern")
	requireSchemaFields("shell_command", []string{"command"}, "command")
}

func TestRegistryDefinitionsUseFrozenSnapshots(t *testing.T) {
	tool := &countingTool{definition: Definition{
		Name:        "snapshot_tool",
		Description: "snapshot test tool",
		InputSchema: objectSchema(map[string]any{
			"value": map[string]any{"type": "string"},
		}, "value"),
	}}

	registry := &Registry{tools: make(map[string]Tool)}
	registry.Register(tool)
	if tool.calls != 1 {
		t.Fatalf("Definition() calls after Register = %d, want 1", tool.calls)
	}

	defs := registry.Definitions()
	if tool.calls != 1 {
		t.Fatalf("Definition() calls after Definitions() = %d, want 1", tool.calls)
	}
	if len(defs) != 1 || defs[0].Name != "snapshot_tool" {
		t.Fatalf("Definitions() = %+v, want frozen snapshot", defs)
	}

	_ = registry.Definitions()
	if tool.calls != 1 {
		t.Fatalf("Definition() calls after repeated Definitions() = %d, want 1", tool.calls)
	}
}

func TestGlobToolRejectsEscapeOutsideWorkspace(t *testing.T) {
	root := t.TempDir()
	parent := filepath.Dir(root)
	outside := filepath.Join(parent, "outside-glob-escape.txt")
	if err := os.WriteFile(outside, []byte("outside"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	t.Cleanup(func() {
		_ = os.Remove(outside)
	})

	registry := NewRegistry()
	_, err := registry.Execute(context.Background(), Call{
		Name:  "glob",
		Input: map[string]any{"pattern": filepath.Join("..", filepath.Base(outside))},
	}, ExecContext{
		WorkspaceRoot:    root,
		PermissionEngine: permissions.NewEngine(),
	})
	if err == nil {
		t.Fatalf("Execute() error = nil, want escape rejection")
	}
}
