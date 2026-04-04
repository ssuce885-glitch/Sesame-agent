package tools

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"go-agent/internal/permissions"
)

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
	if len(defs) < 5 {
		t.Fatalf("len(Definitions) = %d, want at least 5", len(defs))
	}
	if defs[0].Name == "" || defs[0].Description == "" {
		t.Fatalf("first definition = %+v, want name and description", defs[0])
	}
	if defs[0].InputSchema == nil {
		t.Fatalf("first definition = %+v, want schema", defs[0])
	}
}
