package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go-agent/internal/skillcatalog"
	"go-agent/internal/v2/contracts"
)

func TestRegistryVisibleToolsRespectsRolePolicy(t *testing.T) {
	reg := NewRegistry()
	RegisterAllTools(reg, nil, skillcatalog.Catalog{})

	visible := reg.VisibleTools(contracts.ExecContext{
		RoleSpec: &contracts.RoleSpec{
			ID:           "auditor",
			AllowedTools: []string{"file_read", "shell"},
			ToolPolicy: map[string]contracts.ToolPolicyRule{
				"shell": {Allowed: boolPtr(false)},
			},
		},
	})

	names := make(map[string]bool, len(visible))
	for _, def := range visible {
		names[def.Name] = true
	}
	if names["shell"] {
		t.Fatalf("expected shell to be hidden by tool_policy, got %+v", visible)
	}
	if !names["file_read"] {
		t.Fatalf("expected file_read to remain visible, got %+v", visible)
	}
	if names["memory_write"] {
		t.Fatalf("expected memory_write to be hidden by legacy allowed_tools, got %+v", visible)
	}
}

func TestToolPolicyExplainTool(t *testing.T) {
	reg := NewRegistry()
	reg.Register(contracts.NamespaceShell, NewShellTool())
	reg.Register(contracts.NamespaceFiles, &fileReadTool{})
	reg.Register(contracts.NamespaceFiles, &fileWriteTool{})
	reg.Register(contracts.NamespaceFiles, &grepTool{})
	reg.Register(contracts.NamespaceWorkspace, NewToolPolicyExplainTool(reg))

	roleSpec := &contracts.RoleSpec{
		ID: "docs_reviewer",
		ToolPolicy: map[string]contracts.ToolPolicyRule{
			"shell": {
				AllowedCommands: []string{"printf"},
			},
			"file_write": {
				AllowedPaths: []string{"docs/reports/*"},
			},
		},
	}
	tool, ok := reg.Lookup("tool_policy_explain")
	if !ok {
		t.Fatal("tool_policy_explain not registered")
	}

	t.Run("denied shell command", func(t *testing.T) {
		result, err := tool.Execute(context.Background(), contracts.ToolCall{
			Name: "tool_policy_explain",
			Args: map[string]any{
				"tool_name": "shell",
				"command":   "pwd",
			},
		}, contracts.ExecContext{WorkspaceRoot: t.TempDir(), RoleSpec: roleSpec})
		if err != nil {
			t.Fatalf("Execute returned error: %v", err)
		}
		if result.IsError {
			t.Fatalf("expected explain tool to succeed, got %+v", result)
		}
		var explained toolPolicyExplainResult
		if err := json.Unmarshal([]byte(result.Output), &explained); err != nil {
			t.Fatalf("decode output: %v", err)
		}
		if explained.Allowed || explained.MatchedRule != "tool_policy.shell.allowed_commands" {
			t.Fatalf("unexpected shell explanation: %+v", explained)
		}
	})

	t.Run("allowed file path", func(t *testing.T) {
		result, err := tool.Execute(context.Background(), contracts.ToolCall{
			Name: "tool_policy_explain",
			Args: map[string]any{
				"tool_name": "file_write",
				"path":      "docs/reports/q1.md",
			},
		}, contracts.ExecContext{WorkspaceRoot: t.TempDir(), RoleSpec: roleSpec})
		if err != nil {
			t.Fatalf("Execute returned error: %v", err)
		}
		var explained toolPolicyExplainResult
		if err := json.Unmarshal([]byte(result.Output), &explained); err != nil {
			t.Fatalf("decode output: %v", err)
		}
		if !explained.Allowed {
			t.Fatalf("expected file_write path to be allowed, got %+v", explained)
		}
	})

	t.Run("denied file path", func(t *testing.T) {
		result, err := tool.Execute(context.Background(), contracts.ToolCall{
			Name: "tool_policy_explain",
			Args: map[string]any{
				"tool_name": "file_write",
				"path":      "docs/index.md",
			},
		}, contracts.ExecContext{WorkspaceRoot: t.TempDir(), RoleSpec: roleSpec})
		if err != nil {
			t.Fatalf("Execute returned error: %v", err)
		}
		var explained toolPolicyExplainResult
		if err := json.Unmarshal([]byte(result.Output), &explained); err != nil {
			t.Fatalf("decode output: %v", err)
		}
		if explained.Allowed || explained.MatchedRule != "tool_policy.file_write.allowed_paths" {
			t.Fatalf("unexpected file_write explanation: %+v", explained)
		}
	})

	t.Run("grep directory explanations follow search start policy", func(t *testing.T) {
		root := t.TempDir()
		if err := os.MkdirAll(filepath.Join(root, "docs"), 0o755); err != nil {
			t.Fatalf("MkdirAll docs: %v", err)
		}
		if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
			t.Fatalf("MkdirAll src: %v", err)
		}

		execCtx := contracts.ExecContext{
			WorkspaceRoot: root,
			RoleSpec:      &contracts.RoleSpec{ID: "docs_only", AllowedPaths: []string{"docs/*"}},
		}

		assertExplain := func(path string) toolPolicyExplainResult {
			t.Helper()
			result, err := tool.Execute(context.Background(), contracts.ToolCall{
				Name: "tool_policy_explain",
				Args: map[string]any{
					"tool_name": "grep",
					"path":      path,
				},
			}, execCtx)
			if err != nil {
				t.Fatalf("Execute returned error for %q: %v", path, err)
			}
			var explained toolPolicyExplainResult
			if err := json.Unmarshal([]byte(result.Output), &explained); err != nil {
				t.Fatalf("decode output for %q: %v", path, err)
			}
			return explained
		}

		if explained := assertExplain("."); !explained.Allowed {
			t.Fatalf("expected grep path . to be allowed, got %+v", explained)
		}
		if explained := assertExplain(root); !explained.Allowed {
			t.Fatalf("expected grep absolute root to be allowed, got %+v", explained)
		}
		if explained := assertExplain("src"); explained.Allowed || explained.MatchedRule != "allowed_paths" {
			t.Fatalf("expected grep path src denial from allowed_paths, got %+v", explained)
		}
	})

	t.Run("protected file path", func(t *testing.T) {
		root := t.TempDir()
		if err := os.WriteFile(filepath.Join(root, ".env"), []byte("TOKEN=secret"), 0o644); err != nil {
			t.Fatalf("WriteFile .env: %v", err)
		}
		result, err := tool.Execute(context.Background(), contracts.ToolCall{
			Name: "tool_policy_explain",
			Args: map[string]any{
				"tool_name": "file_read",
				"path":      ".env",
			},
		}, contracts.ExecContext{WorkspaceRoot: root, RoleSpec: roleSpec})
		if err != nil {
			t.Fatalf("Execute returned error: %v", err)
		}
		var explained toolPolicyExplainResult
		if err := json.Unmarshal([]byte(result.Output), &explained); err != nil {
			t.Fatalf("decode output: %v", err)
		}
		if explained.Allowed || explained.MatchedRule != "protected_paths" {
			t.Fatalf("unexpected protected path explanation: %+v", explained)
		}
	})

	t.Run("protected env directory descendant", func(t *testing.T) {
		root := t.TempDir()
		if err := os.MkdirAll(filepath.Join(root, ".env.d"), 0o755); err != nil {
			t.Fatalf("MkdirAll .env.d: %v", err)
		}
		if err := os.WriteFile(filepath.Join(root, ".env.d", "secret"), []byte("TOKEN=secret"), 0o644); err != nil {
			t.Fatalf("WriteFile .env.d/secret: %v", err)
		}
		result, err := tool.Execute(context.Background(), contracts.ToolCall{
			Name: "tool_policy_explain",
			Args: map[string]any{
				"tool_name": "file_read",
				"path":      ".env.d/secret",
			},
		}, contracts.ExecContext{WorkspaceRoot: root, RoleSpec: roleSpec})
		if err != nil {
			t.Fatalf("Execute returned error: %v", err)
		}
		var explained toolPolicyExplainResult
		if err := json.Unmarshal([]byte(result.Output), &explained); err != nil {
			t.Fatalf("decode output: %v", err)
		}
		if explained.Allowed || explained.MatchedRule != "protected_paths" {
			t.Fatalf("unexpected protected env dir explanation: %+v", explained)
		}
	})

	t.Run("protected symlink path", func(t *testing.T) {
		root := t.TempDir()
		if err := os.MkdirAll(filepath.Join(root, ".git"), 0o755); err != nil {
			t.Fatalf("MkdirAll .git: %v", err)
		}
		if err := os.WriteFile(filepath.Join(root, ".git", "config"), []byte("[core]\n"), 0o644); err != nil {
			t.Fatalf("WriteFile .git/config: %v", err)
		}
		if err := os.MkdirAll(filepath.Join(root, "aliases"), 0o755); err != nil {
			t.Fatalf("MkdirAll aliases: %v", err)
		}
		if err := os.Symlink(filepath.Join(root, ".git"), filepath.Join(root, "aliases", "git-link")); err != nil {
			t.Skipf("symlinks unavailable: %v", err)
		}
		result, err := tool.Execute(context.Background(), contracts.ToolCall{
			Name: "tool_policy_explain",
			Args: map[string]any{
				"tool_name": "file_read",
				"path":      "aliases/git-link/config",
			},
		}, contracts.ExecContext{WorkspaceRoot: root, RoleSpec: roleSpec})
		if err != nil {
			t.Fatalf("Execute returned error: %v", err)
		}
		var explained toolPolicyExplainResult
		if err := json.Unmarshal([]byte(result.Output), &explained); err != nil {
			t.Fatalf("decode output: %v", err)
		}
		if explained.Allowed || explained.MatchedRule != "protected_paths" {
			t.Fatalf("unexpected protected symlink explanation: %+v", explained)
		}
	})

	t.Run("role path checks resolved symlink target", func(t *testing.T) {
		root := t.TempDir()
		if err := os.MkdirAll(filepath.Join(root, "allowed"), 0o755); err != nil {
			t.Fatalf("MkdirAll allowed: %v", err)
		}
		if err := os.MkdirAll(filepath.Join(root, "secret"), 0o755); err != nil {
			t.Fatalf("MkdirAll secret: %v", err)
		}
		if err := os.WriteFile(filepath.Join(root, "secret", "hidden.txt"), []byte("secret"), 0o644); err != nil {
			t.Fatalf("WriteFile secret/hidden.txt: %v", err)
		}
		if err := os.Symlink(filepath.Join(root, "secret", "hidden.txt"), filepath.Join(root, "allowed", "entry.txt")); err != nil {
			t.Skipf("symlinks unavailable: %v", err)
		}
		if err := os.Symlink(filepath.Join(root, "secret"), filepath.Join(root, "allowed", "dir-link")); err != nil {
			t.Skipf("symlinks unavailable: %v", err)
		}
		readResult, err := tool.Execute(context.Background(), contracts.ToolCall{
			Name: "tool_policy_explain",
			Args: map[string]any{
				"tool_name": "file_read",
				"path":      "allowed/entry.txt",
			},
		}, contracts.ExecContext{
			WorkspaceRoot: root,
			RoleSpec:      &contracts.RoleSpec{ID: "allowed_only", AllowedPaths: []string{"allowed/*"}},
		})
		if err != nil {
			t.Fatalf("Execute returned error: %v", err)
		}
		var readExplained toolPolicyExplainResult
		if err := json.Unmarshal([]byte(readResult.Output), &readExplained); err != nil {
			t.Fatalf("decode output: %v", err)
		}
		if readExplained.Allowed || readExplained.MatchedRule != "allowed_paths" {
			t.Fatalf("unexpected symlink read explanation: %+v", readExplained)
		}

		writeResult, err := tool.Execute(context.Background(), contracts.ToolCall{
			Name: "tool_policy_explain",
			Args: map[string]any{
				"tool_name": "file_write",
				"path":      "allowed/dir-link/new.txt",
			},
		}, contracts.ExecContext{
			WorkspaceRoot: root,
			RoleSpec: &contracts.RoleSpec{
				ID: "writer",
				ToolPolicy: map[string]contracts.ToolPolicyRule{
					"file_write": {
						AllowedPaths: []string{"allowed/dir-link/*"},
					},
				},
			},
		})
		if err != nil {
			t.Fatalf("Execute returned error: %v", err)
		}
		var writeExplained toolPolicyExplainResult
		if err := json.Unmarshal([]byte(writeResult.Output), &writeExplained); err != nil {
			t.Fatalf("decode output: %v", err)
		}
		if writeExplained.Allowed || writeExplained.MatchedRule != "tool_policy.file_write.allowed_paths" {
			t.Fatalf("unexpected symlink write explanation: %+v", writeExplained)
		}
	})
}

func TestToolPolicyExplainRejectsInvalidPathGlobs(t *testing.T) {
	reg := NewRegistry()
	reg.Register(contracts.NamespaceFiles, &fileWriteTool{})
	reg.Register(contracts.NamespaceWorkspace, NewToolPolicyExplainTool(reg))

	tool, ok := reg.Lookup("tool_policy_explain")
	if !ok {
		t.Fatal("tool_policy_explain not registered")
	}

	result, err := tool.Execute(context.Background(), contracts.ToolCall{
		Name: "tool_policy_explain",
		Args: map[string]any{
			"tool_name": "file_write",
			"path":      "docs/reports/q1.md",
		},
	}, contracts.ExecContext{
		WorkspaceRoot: t.TempDir(),
		RoleSpec: &contracts.RoleSpec{
			ID: "broken",
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
	var explained toolPolicyExplainResult
	if err := json.Unmarshal([]byte(result.Output), &explained); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if explained.Allowed || explained.MatchedRule != "tool_policy.file_write.allowed_paths" || !strings.Contains(explained.Reason, "invalid tool_policy.file_write.allowed_paths glob") {
		t.Fatalf("unexpected invalid glob explanation: %+v", explained)
	}
}

func TestToolPolicyExplainToolSelfGate(t *testing.T) {
	reg := NewRegistry()
	reg.Register(contracts.NamespaceFiles, &fileReadTool{})
	reg.Register(contracts.NamespaceWorkspace, NewToolPolicyExplainTool(reg))

	tool, ok := reg.Lookup("tool_policy_explain")
	if !ok {
		t.Fatal("tool_policy_explain not registered")
	}

	result, err := tool.Execute(context.Background(), contracts.ToolCall{
		Name: "tool_policy_explain",
		Args: map[string]any{
			"tool_name": "file_read",
			"path":      "docs/report.md",
		},
	}, contracts.ExecContext{
		WorkspaceRoot: t.TempDir(),
		RoleSpec: &contracts.RoleSpec{
			ID: "locked_down",
			ToolPolicy: map[string]contracts.ToolPolicyRule{
				"tool_policy_explain": {
					Allowed: boolPtr(false),
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !result.IsError {
		t.Fatalf("expected tool_policy_explain gate denial, got %+v", result)
	}
	if !strings.Contains(result.Output, `tool "tool_policy_explain" denied by role "locked_down": tool_policy.tool_policy_explain.allowed=false`) {
		t.Fatalf("expected self-gate deny reason, got %+v", result)
	}
}

func boolPtr(value bool) *bool {
	return &value
}
