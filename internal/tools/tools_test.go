package tools

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
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

	tool.definition.InputSchema["properties"].(map[string]any)["value"].(map[string]any)["type"] = "number"

	defs := registry.Definitions()
	if tool.calls != 1 {
		t.Fatalf("Definition() calls after Definitions() = %d, want 1", tool.calls)
	}
	if len(defs) != 1 || defs[0].Name != "snapshot_tool" {
		t.Fatalf("Definitions() = %+v, want frozen snapshot", defs)
	}
	if got := defs[0].InputSchema["properties"].(map[string]any)["value"].(map[string]any)["type"]; got != "string" {
		t.Fatalf("registry definition mutated from original tool change: got %v, want string", got)
	}

	defs[0].InputSchema["properties"].(map[string]any)["value"].(map[string]any)["type"] = "boolean"

	defsAgain := registry.Definitions()
	if tool.calls != 1 {
		t.Fatalf("Definition() calls after repeated Definitions() = %d, want 1", tool.calls)
	}
	if got := defsAgain[0].InputSchema["properties"].(map[string]any)["value"].(map[string]any)["type"]; got != "string" {
		t.Fatalf("registry definition mutated from returned Definitions() change: got %v, want string", got)
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

func TestPermissionProfilesControlLocalTools(t *testing.T) {
	root := t.TempDir()
	registry := NewRegistry()

	t.Run("default profile denies write and shell", func(t *testing.T) {
		_, err := registry.Execute(context.Background(), Call{
			Name:  "file_write",
			Input: map[string]any{"path": filepath.Join(root, "blocked.txt"), "content": "blocked"},
		}, ExecContext{
			WorkspaceRoot:    root,
			PermissionEngine: permissions.NewEngine(),
		})
		if err == nil || !strings.Contains(err.Error(), "denied") {
			t.Fatalf("file_write error = %v, want denied", err)
		}

		_, err = registry.Execute(context.Background(), Call{
			Name:  "shell_command",
			Input: map[string]any{"command": "echo blocked"},
		}, ExecContext{
			WorkspaceRoot:    root,
			PermissionEngine: permissions.NewEngine(),
		})
		if err == nil || !strings.Contains(err.Error(), "denied") {
			t.Fatalf("shell_command error = %v, want denied", err)
		}
	})

	t.Run("trusted_local allows write and shell", func(t *testing.T) {
		writePath := filepath.Join(root, "allowed.txt")
		result, err := registry.Execute(context.Background(), Call{
			Name:  "file_write",
			Input: map[string]any{"path": writePath, "content": "hello"},
		}, ExecContext{
			WorkspaceRoot:    root,
			PermissionEngine: permissions.NewEngine("trusted_local"),
		})
		if err != nil {
			t.Fatalf("file_write error = %v", err)
		}
		if result.Text != writePath {
			t.Fatalf("file_write result.Text = %q, want %q", result.Text, writePath)
		}
		data, err := os.ReadFile(writePath)
		if err != nil {
			t.Fatalf("ReadFile() error = %v", err)
		}
		if string(data) != "hello" {
			t.Fatalf("written file = %q, want %q", string(data), "hello")
		}

		shellResult, err := registry.Execute(context.Background(), Call{
			Name:  "shell_command",
			Input: map[string]any{"command": "echo ok"},
		}, ExecContext{
			WorkspaceRoot:    root,
			PermissionEngine: permissions.NewEngine("trusted_local"),
		})
		if err != nil {
			t.Fatalf("shell_command error = %v", err)
		}
		if !strings.Contains(shellResult.Text, "ok") {
			t.Fatalf("shell_command result.Text = %q, want output", shellResult.Text)
		}
	})
}

func TestShellCommandTruncatesOutputAndUsesWorkspaceDir(t *testing.T) {
	workspace := t.TempDir()
	registry := NewRegistry()

	result, err := registry.Execute(context.Background(), Call{
		Name:  "shell_command",
		Input: map[string]any{"command": "echo %cd% && for /L %i in (1,1,200) do @echo x"},
	}, ExecContext{
		WorkspaceRoot:    workspace,
		PermissionEngine: permissions.NewEngine("trusted_local"),
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if len(result.Text) != shellCommandMaxOutputBytes {
		t.Fatalf("len(result.Text) = %d, want %d", len(result.Text), shellCommandMaxOutputBytes)
	}
	if !strings.Contains(result.Text, workspace) {
		t.Fatalf("result.Text = %q, want workspace root %q", result.Text, workspace)
	}
}

func TestFileWriteRejectsContentAboveLimit(t *testing.T) {
	root := t.TempDir()
	registry := NewRegistry()
	content := strings.Repeat("x", fileWriteMaxBytes+1)

	_, err := registry.Execute(context.Background(), Call{
		Name:  "file_write",
		Input: map[string]any{"path": filepath.Join(root, "too-big.txt"), "content": content},
	}, ExecContext{
		WorkspaceRoot:    root,
		PermissionEngine: permissions.NewEngine("trusted_local"),
	})
	if err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("Execute() error = %v, want size limit error", err)
	}
}
