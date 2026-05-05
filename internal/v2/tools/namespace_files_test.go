package tools

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go-agent/internal/v2/contracts"
)

func requireSymlink(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
}

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
		{
			name: "role allowed docs does not cross directories",
			path: "docs/a/b.md",
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
	if err := os.MkdirAll(filepath.Join(root, ".sesame"), 0o755); err != nil {
		t.Fatalf("MkdirAll .sesame: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, ".sesame", "config.json"), []byte("{}"), 0o644); err != nil {
		t.Fatalf("WriteFile .sesame config: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatalf("MkdirAll .git: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, ".git", "config"), []byte("[core]"), 0o644); err != nil {
		t.Fatalf("WriteFile .git/config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "secret.txt"), []byte("secret"), 0o644); err != nil {
		t.Fatalf("WriteFile secret: %v", err)
	}

	tool := &fileReadTool{}
	for _, path := range []string{".env", ".sesame/config.json", ".git/config"} {
		result, err := tool.Execute(ctx, contracts.ToolCall{
			Args: map[string]any{"path": path},
		}, contracts.ExecContext{WorkspaceRoot: root})
		if err != nil {
			t.Fatalf("Execute returned error for %s: %v", path, err)
		}
		if !result.IsError || !strings.Contains(result.Output, "protected") {
			t.Fatalf("expected protected path denial for %q, got %+v", path, result)
		}
	}

	result, err := tool.Execute(ctx, contracts.ToolCall{
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

func TestGlobSkipsProtectedMatches(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte("TOKEN=secret"), 0o644); err != nil {
		t.Fatalf("WriteFile .env: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, ".env.local"), []byte("TOKEN=secret"), 0o644); err != nil {
		t.Fatalf("WriteFile .env.local: %v", err)
	}

	result, err := (&globTool{}).Execute(ctx, contracts.ToolCall{
		Args: map[string]any{"pattern": ".env*"},
	}, contracts.ExecContext{WorkspaceRoot: root})
	if err != nil {
		t.Fatalf("glob Execute returned error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected glob to succeed, got %+v", result)
	}
	if strings.TrimSpace(result.Output) != "" {
		t.Fatalf("expected protected matches to be filtered, got %+v", result)
	}
}

func TestGrepSkipsAndRejectsProtectedPaths(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte("TOKEN=secret"), 0o644); err != nil {
		t.Fatalf("WriteFile .env: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "notes.txt"), []byte("public"), 0o644); err != nil {
		t.Fatalf("WriteFile notes: %v", err)
	}

	grep := &grepTool{}
	result, err := grep.Execute(ctx, contracts.ToolCall{
		Args: map[string]any{"pattern": "TOKEN"},
	}, contracts.ExecContext{WorkspaceRoot: root})
	if err != nil {
		t.Fatalf("grep Execute returned error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected grep to succeed, got %+v", result)
	}
	if strings.TrimSpace(result.Output) != "" {
		t.Fatalf("expected grep to skip protected files, got %+v", result)
	}

	result, err = grep.Execute(ctx, contracts.ToolCall{
		Args: map[string]any{"pattern": "TOKEN", "path": ".env"},
	}, contracts.ExecContext{WorkspaceRoot: root})
	if err != nil {
		t.Fatalf("grep Execute returned error: %v", err)
	}
	if !result.IsError || !strings.Contains(result.Output, "protected pattern") {
		t.Fatalf("expected direct protected grep denial, got %+v", result)
	}
}

func TestGrepExplicitDirectoryHonorsRolePathPolicy(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "docs"), 0o755); err != nil {
		t.Fatalf("MkdirAll docs: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatalf("MkdirAll src: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "docs", "guide.md"), []byte("needle in docs"), 0o644); err != nil {
		t.Fatalf("WriteFile docs/guide.md: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "src", "main.go"), []byte("needle in src"), 0o644); err != nil {
		t.Fatalf("WriteFile src/main.go: %v", err)
	}

	roleSpec := &contracts.RoleSpec{ID: "docs_only", AllowedPaths: []string{"docs/*"}}
	grep := &grepTool{}

	defaultResult, err := grep.Execute(ctx, contracts.ToolCall{
		Args: map[string]any{"pattern": "needle"},
	}, contracts.ExecContext{WorkspaceRoot: root, RoleSpec: roleSpec})
	if err != nil {
		t.Fatalf("grep Execute returned error: %v", err)
	}
	if defaultResult.IsError {
		t.Fatalf("expected default grep to succeed, got %+v", defaultResult)
	}
	if !strings.Contains(defaultResult.Output, "docs/guide.md:1:needle in docs") {
		t.Fatalf("expected default grep to include docs file, got %+v", defaultResult)
	}
	if strings.Contains(defaultResult.Output, "src/main.go") {
		t.Fatalf("expected default grep to skip src file, got %+v", defaultResult)
	}

	explicitAllowed, err := grep.Execute(ctx, contracts.ToolCall{
		Args: map[string]any{"pattern": "needle", "path": "docs"},
	}, contracts.ExecContext{WorkspaceRoot: root, RoleSpec: roleSpec})
	if err != nil {
		t.Fatalf("grep Execute returned error: %v", err)
	}
	if explicitAllowed.IsError || !strings.Contains(explicitAllowed.Output, "docs/guide.md:1:needle in docs") {
		t.Fatalf("expected explicit docs grep to succeed, got %+v", explicitAllowed)
	}

	explicitDenied, err := grep.Execute(ctx, contracts.ToolCall{
		Args: map[string]any{"pattern": "needle", "path": "src"},
	}, contracts.ExecContext{WorkspaceRoot: root, RoleSpec: roleSpec})
	if err != nil {
		t.Fatalf("grep Execute returned error: %v", err)
	}
	if !explicitDenied.IsError || !strings.Contains(explicitDenied.Output, "not in allowed paths") {
		t.Fatalf("expected explicit src grep denial, got %+v", explicitDenied)
	}
}

func TestEnvDirectoriesAreProtected(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".env.d"), 0o755); err != nil {
		t.Fatalf("MkdirAll .env.d: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, ".env.d", "secret"), []byte("TOKEN=secret"), 0o644); err != nil {
		t.Fatalf("WriteFile .env.d/secret: %v", err)
	}

	readResult, err := (&fileReadTool{}).Execute(ctx, contracts.ToolCall{
		Args: map[string]any{"path": ".env.d/secret"},
	}, contracts.ExecContext{WorkspaceRoot: root})
	if err != nil {
		t.Fatalf("file_read Execute returned error: %v", err)
	}
	if !readResult.IsError || !strings.Contains(readResult.Output, "protected") {
		t.Fatalf("expected protected env dir read denial, got %+v", readResult)
	}

	grepResult, err := (&grepTool{}).Execute(ctx, contracts.ToolCall{
		Args: map[string]any{"pattern": "TOKEN", "path": ".env.d/secret"},
	}, contracts.ExecContext{WorkspaceRoot: root})
	if err != nil {
		t.Fatalf("grep Execute returned error: %v", err)
	}
	if !grepResult.IsError || !strings.Contains(grepResult.Output, "protected") {
		t.Fatalf("expected protected env dir grep denial, got %+v", grepResult)
	}

	globResult, err := (&globTool{}).Execute(ctx, contracts.ToolCall{
		Args: map[string]any{"pattern": ".env.d/*"},
	}, contracts.ExecContext{WorkspaceRoot: root})
	if err != nil {
		t.Fatalf("glob Execute returned error: %v", err)
	}
	if globResult.IsError {
		t.Fatalf("expected glob to succeed, got %+v", globResult)
	}
	if strings.TrimSpace(globResult.Output) != "" {
		t.Fatalf("expected protected env dir matches to be filtered, got %+v", globResult)
	}
}

func TestFileToolsRejectProtectedSymlinkTargets(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte("TOKEN=secret"), 0o644); err != nil {
		t.Fatalf("WriteFile .env: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".sesame"), 0o755); err != nil {
		t.Fatalf("MkdirAll .sesame: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, ".sesame", "config.json"), []byte("old"), 0o644); err != nil {
		t.Fatalf("WriteFile .sesame/config.json: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatalf("MkdirAll .git: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, ".git", "config"), []byte("[core]\n"), 0o644); err != nil {
		t.Fatalf("WriteFile .git/config: %v", err)
	}
	aliases := filepath.Join(root, "aliases")
	if err := os.MkdirAll(aliases, 0o755); err != nil {
		t.Fatalf("MkdirAll aliases: %v", err)
	}
	requireSymlink(t, filepath.Join(root, ".env"), filepath.Join(aliases, "env-link"))
	requireSymlink(t, filepath.Join(root, ".sesame"), filepath.Join(aliases, "sesame-link"))
	requireSymlink(t, filepath.Join(root, ".git"), filepath.Join(aliases, "git-link"))

	readResult, err := (&fileReadTool{}).Execute(ctx, contracts.ToolCall{
		Args: map[string]any{"path": "aliases/env-link"},
	}, contracts.ExecContext{WorkspaceRoot: root})
	if err != nil {
		t.Fatalf("file_read Execute returned error: %v", err)
	}
	if !readResult.IsError || !strings.Contains(readResult.Output, "protected") {
		t.Fatalf("expected protected symlink read denial, got %+v", readResult)
	}

	writeResult, err := (&fileWriteTool{}).Execute(ctx, contracts.ToolCall{
		Args: map[string]any{"path": "aliases/git-link/hooks.txt", "content": "blocked"},
	}, contracts.ExecContext{WorkspaceRoot: root})
	if err != nil {
		t.Fatalf("file_write Execute returned error: %v", err)
	}
	if !writeResult.IsError || !strings.Contains(writeResult.Output, "protected") {
		t.Fatalf("expected protected symlink write denial, got %+v", writeResult)
	}

	editResult, err := (&fileEditTool{}).Execute(ctx, contracts.ToolCall{
		Args: map[string]any{
			"path":       "aliases/sesame-link/config.json",
			"old_string": "old",
			"new_string": "new",
		},
	}, contracts.ExecContext{WorkspaceRoot: root})
	if err != nil {
		t.Fatalf("file_edit Execute returned error: %v", err)
	}
	if !editResult.IsError || !strings.Contains(editResult.Output, "protected") {
		t.Fatalf("expected protected symlink edit denial, got %+v", editResult)
	}

	grepResult, err := (&grepTool{}).Execute(ctx, contracts.ToolCall{
		Args: map[string]any{"pattern": "TOKEN", "path": "aliases/env-link"},
	}, contracts.ExecContext{WorkspaceRoot: root})
	if err != nil {
		t.Fatalf("grep Execute returned error: %v", err)
	}
	if !grepResult.IsError || !strings.Contains(grepResult.Output, "protected") {
		t.Fatalf("expected protected symlink grep denial, got %+v", grepResult)
	}

	globResult, err := (&globTool{}).Execute(ctx, contracts.ToolCall{
		Args: map[string]any{"pattern": "aliases/*"},
	}, contracts.ExecContext{WorkspaceRoot: root})
	if err != nil {
		t.Fatalf("glob Execute returned error: %v", err)
	}
	if globResult.IsError {
		t.Fatalf("expected glob to succeed, got %+v", globResult)
	}
	if strings.TrimSpace(globResult.Output) != "" {
		t.Fatalf("expected protected symlink matches to be filtered, got %+v", globResult)
	}
}

func TestFileToolsRejectRoleBypassViaSymlinkTargets(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "allowed"), 0o755); err != nil {
		t.Fatalf("MkdirAll allowed: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "secret"), 0o755); err != nil {
		t.Fatalf("MkdirAll secret: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "secret", "hidden.txt"), []byte("old"), 0o644); err != nil {
		t.Fatalf("WriteFile secret/hidden.txt: %v", err)
	}
	requireSymlink(t, filepath.Join(root, "secret", "hidden.txt"), filepath.Join(root, "allowed", "entry.txt"))
	requireSymlink(t, filepath.Join(root, "secret"), filepath.Join(root, "allowed", "dir-link"))

	roleSpec := &contracts.RoleSpec{ID: "allowed_only", AllowedPaths: []string{"allowed/*"}}

	readResult, err := (&fileReadTool{}).Execute(ctx, contracts.ToolCall{
		Args: map[string]any{"path": "allowed/entry.txt"},
	}, contracts.ExecContext{WorkspaceRoot: root, RoleSpec: roleSpec})
	if err != nil {
		t.Fatalf("file_read Execute returned error: %v", err)
	}
	if !readResult.IsError || !strings.Contains(readResult.Output, "not in allowed paths") {
		t.Fatalf("expected real-path role denial for symlink read, got %+v", readResult)
	}

	editResult, err := (&fileEditTool{}).Execute(ctx, contracts.ToolCall{
		Args: map[string]any{
			"path":       "allowed/entry.txt",
			"old_string": "old",
			"new_string": "new",
		},
	}, contracts.ExecContext{WorkspaceRoot: root, RoleSpec: roleSpec})
	if err != nil {
		t.Fatalf("file_edit Execute returned error: %v", err)
	}
	if !editResult.IsError || !strings.Contains(editResult.Output, "not in allowed paths") {
		t.Fatalf("expected real-path role denial for symlink edit, got %+v", editResult)
	}

	grepResult, err := (&grepTool{}).Execute(ctx, contracts.ToolCall{
		Args: map[string]any{"pattern": "old", "path": "allowed/entry.txt"},
	}, contracts.ExecContext{WorkspaceRoot: root, RoleSpec: roleSpec})
	if err != nil {
		t.Fatalf("grep Execute returned error: %v", err)
	}
	if !grepResult.IsError || !strings.Contains(grepResult.Output, "not in allowed paths") {
		t.Fatalf("expected real-path role denial for symlink grep, got %+v", grepResult)
	}

	globResult, err := (&globTool{}).Execute(ctx, contracts.ToolCall{
		Args: map[string]any{"pattern": "allowed/*"},
	}, contracts.ExecContext{WorkspaceRoot: root, RoleSpec: roleSpec})
	if err != nil {
		t.Fatalf("glob Execute returned error: %v", err)
	}
	if globResult.IsError {
		t.Fatalf("expected glob to succeed, got %+v", globResult)
	}
	if strings.TrimSpace(globResult.Output) != "" {
		t.Fatalf("expected role-denied symlink matches to be filtered, got %+v", globResult)
	}
}

func TestFileWriteRejectsRoleBypassViaSymlinkParent(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "allowed"), 0o755); err != nil {
		t.Fatalf("MkdirAll allowed: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "secret"), 0o755); err != nil {
		t.Fatalf("MkdirAll secret: %v", err)
	}
	requireSymlink(t, filepath.Join(root, "secret"), filepath.Join(root, "allowed", "dir-link"))

	result, err := (&fileWriteTool{}).Execute(ctx, contracts.ToolCall{
		Args: map[string]any{
			"path":    "allowed/dir-link/new.txt",
			"content": "blocked",
		},
	}, contracts.ExecContext{
		WorkspaceRoot: root,
		RoleSpec:      &contracts.RoleSpec{ID: "allowed_only", AllowedPaths: []string{"allowed/dir-link/*"}},
	})
	if err != nil {
		t.Fatalf("file_write Execute returned error: %v", err)
	}
	if !result.IsError || !strings.Contains(result.Output, "not in allowed paths") {
		t.Fatalf("expected real-path role denial for symlink parent write, got %+v", result)
	}
}

func TestProtectedPathsAreCaseInsensitive(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".ENV"), []byte("TOKEN=secret"), 0o644); err != nil {
		t.Fatalf("WriteFile .ENV: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".Sesame"), 0o755); err != nil {
		t.Fatalf("MkdirAll .Sesame: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, ".Sesame", "config.json"), []byte("{}"), 0o644); err != nil {
		t.Fatalf("WriteFile .Sesame/config.json: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".GIT"), 0o755); err != nil {
		t.Fatalf("MkdirAll .GIT: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, ".GIT", "config"), []byte("[core]\n"), 0o644); err != nil {
		t.Fatalf("WriteFile .GIT/config: %v", err)
	}

	for _, candidate := range []string{".ENV", ".Sesame/config.json", ".GIT/config"} {
		result, err := (&fileReadTool{}).Execute(ctx, contracts.ToolCall{
			Args: map[string]any{"path": candidate},
		}, contracts.ExecContext{WorkspaceRoot: root})
		if err != nil {
			t.Fatalf("file_read Execute returned error for %s: %v", candidate, err)
		}
		if !result.IsError || !strings.Contains(result.Output, "protected") {
			t.Fatalf("expected case-insensitive protected denial for %q, got %+v", candidate, result)
		}
	}

	globResult, err := (&globTool{}).Execute(ctx, contracts.ToolCall{
		Args: map[string]any{"pattern": ".*"},
	}, contracts.ExecContext{WorkspaceRoot: root})
	if err != nil {
		t.Fatalf("glob Execute returned error: %v", err)
	}
	if globResult.IsError {
		t.Fatalf("expected glob to succeed, got %+v", globResult)
	}
	if strings.TrimSpace(globResult.Output) != "" {
		t.Fatalf("expected case-insensitive protected matches to be filtered, got %+v", globResult)
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

func TestFileWritePerToolPathPolicy(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	tool := &fileWriteTool{}
	roleSpec := &contracts.RoleSpec{
		ID:           "docs_writer",
		AllowedPaths: []string{"docs/*/*.md"},
		ToolPolicy: map[string]contracts.ToolPolicyRule{
			"file_write": {
				AllowedPaths: []string{"docs/reports/*"},
			},
		},
	}

	deniedResult, err := tool.Execute(ctx, contracts.ToolCall{
		Args: map[string]any{
			"path":    "docs/guides/q1.md",
			"content": "overview",
		},
	}, contracts.ExecContext{WorkspaceRoot: root, RoleSpec: roleSpec})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !deniedResult.IsError || !strings.Contains(deniedResult.Output, "tool_policy.file_write.allowed_paths") {
		t.Fatalf("expected per-tool allowed_paths denial, got %+v", deniedResult)
	}

	allowedResult, err := tool.Execute(ctx, contracts.ToolCall{
		Args: map[string]any{
			"path":    "docs/reports/q1.md",
			"content": "report",
		},
	}, contracts.ExecContext{WorkspaceRoot: root, RoleSpec: roleSpec})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if allowedResult.IsError {
		t.Fatalf("expected per-tool allowed_paths write to succeed, got %+v", allowedResult)
	}
}

func TestFileWriteRejectsInvalidPathGlobs(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	tool := &fileWriteTool{}

	result, err := tool.Execute(ctx, contracts.ToolCall{
		Args: map[string]any{
			"path":    "docs/reports/q1.md",
			"content": "report",
		},
	}, contracts.ExecContext{
		WorkspaceRoot: root,
		RoleSpec: &contracts.RoleSpec{
			ID:           "broken",
			AllowedPaths: []string{"["},
		},
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !result.IsError || !strings.Contains(result.Output, "invalid allowed_paths glob") {
		t.Fatalf("expected invalid role glob denial, got %+v", result)
	}

	result, err = tool.Execute(ctx, contracts.ToolCall{
		Args: map[string]any{
			"path":    "docs/reports/q1.md",
			"content": "report",
		},
	}, contracts.ExecContext{
		WorkspaceRoot: root,
		RoleSpec: &contracts.RoleSpec{
			ID:           "recursive_glob",
			AllowedPaths: []string{"docs/**"},
		},
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !result.IsError || !strings.Contains(result.Output, "recursive \"**\" globs are not supported") {
		t.Fatalf("expected recursive glob denial, got %+v", result)
	}

	result, err = tool.Execute(ctx, contracts.ToolCall{
		Args: map[string]any{
			"path":    "docs/reports/q1.md",
			"content": "report",
		},
	}, contracts.ExecContext{
		WorkspaceRoot: root,
		RoleSpec: &contracts.RoleSpec{
			ID: "broken_tool_policy",
			ToolPolicy: map[string]contracts.ToolPolicyRule{
				"file_write": {
					AllowedPaths: []string{"["},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !result.IsError || !strings.Contains(result.Output, "invalid tool_policy.file_write.allowed_paths glob") {
		t.Fatalf("expected invalid tool policy glob denial, got %+v", result)
	}
}

func TestResolveRealWorkspaceRootPropagatesEvalSymlinksError(t *testing.T) {
	sentinel := errors.New("eval symlinks failed")
	eval := func(string) (string, error) {
		return "", sentinel
	}

	_, err := resolveRealWorkspaceRootWith(t.TempDir(), eval)
	if err == nil {
		t.Fatal("expected EvalSymlinks error")
	}
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected propagated EvalSymlinks error, got %v", err)
	}
}
