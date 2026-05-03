package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go-agent/internal/v2/contracts"
)

func TestFileWriteSafety(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	tool := &fileWriteTool{}

	tests := []struct {
		name     string
		path     string
		roleSpec *contracts.RoleSpec
		wantErr  string
	}{
		{
			name:    "outside workspace",
			path:    filepath.Join(root, "..", "outside.txt"),
			wantErr: "path escapes workspace root",
		},
		{
			name:    "git config",
			path:    ".git/config",
			wantErr: `touches protected area ".git"`,
		},
		{
			name:    "env file",
			path:    ".env",
			wantErr: `matches protected pattern ".env*"`,
		},
		{
			name:    "sesame database",
			path:    ".sesame/sesame.db",
			wantErr: `touches protected area ".sesame"`,
		},
		{
			name: "role denied go file",
			path: "main.go",
			roleSpec: &contracts.RoleSpec{ID: "restricted", DeniedPaths: []string{
				"*.go",
			}},
			wantErr: `matches denied pattern "*.go"`,
		},
		{
			name: "role allowed docs only",
			path: "src/main.txt",
			roleSpec: &contracts.RoleSpec{ID: "docs", AllowedPaths: []string{
				"docs/*",
			}},
			wantErr: "not in allowed paths",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := tool.Execute(ctx, contracts.ToolCall{
				Args: map[string]any{
					"path":    tt.path,
					"content": "content",
				},
			}, contracts.ExecContext{WorkspaceRoot: root, RoleSpec: tt.roleSpec})
			if err != nil {
				t.Fatalf("Execute returned error: %v", err)
			}
			if !result.IsError || !strings.Contains(result.Output, tt.wantErr) {
				t.Fatalf("expected error containing %q, got result=%+v", tt.wantErr, result)
			}
		})
	}

	result, err := tool.Execute(ctx, contracts.ToolCall{
		Args: map[string]any{
			"path":    "notes/today.txt",
			"content": "normal",
		},
	}, contracts.ExecContext{WorkspaceRoot: root})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected normal write to succeed, got %+v", result)
	}
	data, err := os.ReadFile(filepath.Join(root, "notes", "today.txt"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != "normal" {
		t.Fatalf("unexpected written content: %q", string(data))
	}
}

func TestFileEditSafety(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	tool := &fileEditTool{}

	result, err := tool.Execute(ctx, contracts.ToolCall{
		Args: map[string]any{
			"path":       filepath.Join(root, "..", "outside.txt"),
			"old_string": "old",
			"new_string": "new",
		},
	}, contracts.ExecContext{WorkspaceRoot: root})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !result.IsError || !strings.Contains(result.Output, "path escapes workspace root") {
		t.Fatalf("expected outside workspace error, got %+v", result)
	}

	gitignore := filepath.Join(root, ".gitignore")
	if err := os.WriteFile(gitignore, []byte("old\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	result, err = tool.Execute(ctx, contracts.ToolCall{
		Args: map[string]any{
			"path":       ".gitignore",
			"old_string": "old",
			"new_string": "new",
		},
	}, contracts.ExecContext{WorkspaceRoot: root})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected .gitignore edit to succeed, got %+v", result)
	}
	data, err := os.ReadFile(gitignore)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != "new\n" {
		t.Fatalf("unexpected edited content: %q", string(data))
	}
}

func TestFileReadPathPolicies(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte("TOKEN=secret"), 0o644); err != nil {
		t.Fatalf("WriteFile .env: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "secret.txt"), []byte("secret"), 0o644); err != nil {
		t.Fatalf("WriteFile secret: %v", err)
	}

	tool := &fileReadTool{}
	result, err := tool.Execute(ctx, contracts.ToolCall{
		Args: map[string]any{"path": ".env"},
	}, contracts.ExecContext{WorkspaceRoot: root})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result.IsError || result.Output != "TOKEN=secret" {
		t.Fatalf("expected unrestricted .env read to succeed, got %+v", result)
	}

	result, err = tool.Execute(ctx, contracts.ToolCall{
		Args: map[string]any{"path": "secret.txt"},
	}, contracts.ExecContext{
		WorkspaceRoot: root,
		RoleSpec:      &contracts.RoleSpec{ID: "restricted", DeniedPaths: []string{"secret.txt"}},
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !result.IsError || !strings.Contains(result.Output, `matches denied pattern "secret.txt"`) {
		t.Fatalf("expected denied read, got %+v", result)
	}

	result, err = tool.Execute(ctx, contracts.ToolCall{
		Args: map[string]any{"path": "secret.txt"},
	}, contracts.ExecContext{
		WorkspaceRoot: root,
		RoleSpec:      &contracts.RoleSpec{ID: "docs", AllowedPaths: []string{"docs/*"}},
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !result.IsError || !strings.Contains(result.Output, "not in allowed paths") {
		t.Fatalf("expected allowed-path read denial, got %+v", result)
	}
}

func TestFileToolsRejectOversizedContent(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()

	large := strings.Repeat("x", maxFileWriteBytes+1)
	writeResult, err := (&fileWriteTool{}).Execute(ctx, contracts.ToolCall{
		Args: map[string]any{
			"path":    "large.txt",
			"content": large,
		},
	}, contracts.ExecContext{WorkspaceRoot: root})
	if err != nil {
		t.Fatalf("file_write Execute returned error: %v", err)
	}
	if !writeResult.IsError || !strings.Contains(writeResult.Output, "too large to write") {
		t.Fatalf("expected large write rejection, got %+v", writeResult)
	}

	if err := os.WriteFile(filepath.Join(root, "existing.txt"), []byte(large), 0o644); err != nil {
		t.Fatalf("WriteFile existing: %v", err)
	}
	readResult, err := (&fileReadTool{}).Execute(ctx, contracts.ToolCall{
		Args: map[string]any{"path": "existing.txt"},
	}, contracts.ExecContext{WorkspaceRoot: root})
	if err != nil {
		t.Fatalf("file_read Execute returned error: %v", err)
	}
	if !readResult.IsError || !strings.Contains(readResult.Output, "too large to read") {
		t.Fatalf("expected large read rejection, got %+v", readResult)
	}

	editResult, err := (&fileEditTool{}).Execute(ctx, contracts.ToolCall{
		Args: map[string]any{
			"path":       "existing.txt",
			"old_string": "x",
			"new_string": "y",
		},
	}, contracts.ExecContext{WorkspaceRoot: root})
	if err != nil {
		t.Fatalf("file_edit Execute returned error: %v", err)
	}
	if !editResult.IsError || !strings.Contains(editResult.Output, "too large to edit") {
		t.Fatalf("expected large edit rejection, got %+v", editResult)
	}
}
