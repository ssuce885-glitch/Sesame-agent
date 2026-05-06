package tools

import (
	"context"
	"os"
	"path/filepath"
	stdruntime "runtime"
	"strings"
	"testing"
	"time"

	"go-agent/internal/v2/contracts"
)

func TestShellToolPolicyAllowedCommands(t *testing.T) {
	skipWindowsUnixShellTest(t)

	root := t.TempDir()
	tool := NewShellTool()
	roleSpec := &contracts.RoleSpec{
		ID: "restricted_shell",
		ToolPolicy: map[string]contracts.ToolPolicyRule{
			"shell": {
				AllowedCommands: []string{"printf"},
			},
		},
	}

	result, err := tool.Execute(context.Background(), contracts.ToolCall{
		Name: "shell",
		Args: map[string]any{"command": "printf ok"},
	}, contracts.ExecContext{WorkspaceRoot: root, RoleSpec: roleSpec})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result.IsError || strings.TrimSpace(result.Output) != "ok" {
		t.Fatalf("expected allowed shell command to succeed, got %+v", result)
	}
}

func TestShellToolPolicyRejectsCommand(t *testing.T) {
	root := t.TempDir()
	tool := NewShellTool()
	roleSpec := &contracts.RoleSpec{
		ID: "restricted_shell",
		ToolPolicy: map[string]contracts.ToolPolicyRule{
			"shell": {
				AllowedCommands: []string{"printf"},
			},
		},
	}

	result, err := tool.Execute(context.Background(), contracts.ToolCall{
		Name: "shell",
		Args: map[string]any{"command": "pwd"},
	}, contracts.ExecContext{WorkspaceRoot: root, RoleSpec: roleSpec})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !result.IsError || !strings.Contains(result.Output, "allowed_commands") {
		t.Fatalf("expected shell command denial, got %+v", result)
	}
}

func TestShellToolPolicyRejectsShellMetacharacters(t *testing.T) {
	root := t.TempDir()
	tool := NewShellTool()
	roleSpec := &contracts.RoleSpec{
		ID: "restricted_shell",
		ToolPolicy: map[string]contracts.ToolPolicyRule{
			"shell": {
				AllowedCommands: []string{"printf"},
			},
		},
	}

	for _, command := range []string{
		"printf ok; pwd",
		"printf $(pwd)",
		"printf ok && pwd",
	} {
		t.Run(command, func(t *testing.T) {
			result, err := tool.Execute(context.Background(), contracts.ToolCall{
				Name: "shell",
				Args: map[string]any{"command": command},
			}, contracts.ExecContext{WorkspaceRoot: root, RoleSpec: roleSpec})
			if err != nil {
				t.Fatalf("Execute returned error: %v", err)
			}
			if !result.IsError || !strings.Contains(result.Output, "forbidden shell syntax") {
				t.Fatalf("expected shell syntax denial for %q, got %+v", command, result)
			}
		})
	}
}

func TestShellToolPolicyTimeoutOverride(t *testing.T) {
	skipWindowsUnixShellTest(t)

	root := t.TempDir()
	tool := NewShellTool()
	roleSpec := &contracts.RoleSpec{
		ID: "timed_shell",
		ToolPolicy: map[string]contracts.ToolPolicyRule{
			"shell": {
				AllowedCommands: []string{"sleep"},
				TimeoutSeconds:  1,
			},
		},
	}

	result, err := tool.Execute(context.Background(), contracts.ToolCall{
		Name: "shell",
		Args: map[string]any{"command": "sleep 2"},
	}, contracts.ExecContext{WorkspaceRoot: root, RoleSpec: roleSpec})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !result.IsError || !strings.Contains(result.Output, "timed out after 1s") {
		t.Fatalf("expected shell timeout denial, got %+v", result)
	}
}

func TestShellToolPolicyOutputCapOverride(t *testing.T) {
	skipWindowsUnixShellTest(t)

	root := t.TempDir()
	tool := NewShellTool()
	roleSpec := &contracts.RoleSpec{
		ID: "capped_shell",
		ToolPolicy: map[string]contracts.ToolPolicyRule{
			"shell": {
				AllowedCommands: []string{"printf"},
				MaxOutputBytes:  4,
			},
		},
	}

	result, err := tool.Execute(context.Background(), contracts.ToolCall{
		Name: "shell",
		Args: map[string]any{"command": "printf 1234567890"},
	}, contracts.ExecContext{WorkspaceRoot: root, RoleSpec: roleSpec})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !result.IsError || !strings.Contains(result.Output, "truncated after 4 bytes") {
		t.Fatalf("expected shell output truncation, got %+v", result)
	}
}

func skipWindowsUnixShellTest(t *testing.T) {
	t.Helper()
	if stdruntime.GOOS == "windows" {
		t.Skip("unix shell command execution is not supported on Windows")
	}
}

func TestShellToolTimeoutKillsBackgroundProcess(t *testing.T) {
	if stdruntime.GOOS == "windows" {
		t.Skip("process group test is unix-specific")
	}

	root := t.TempDir()
	sentinel := filepath.Join(root, "child.txt")
	tool := NewShellTool()
	roleSpec := &contracts.RoleSpec{
		ID: "timed_shell",
		ToolPolicy: map[string]contracts.ToolPolicyRule{
			"shell": {
				TimeoutSeconds: 1,
			},
		},
	}

	result, err := tool.Execute(context.Background(), contracts.ToolCall{
		Name: "shell",
		Args: map[string]any{"command": "(sleep 2; printf leaked > child.txt) & sleep 10"},
	}, contracts.ExecContext{WorkspaceRoot: root, RoleSpec: roleSpec})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !result.IsError || !strings.Contains(result.Output, "timed out after 1s") {
		t.Fatalf("expected shell timeout denial, got %+v", result)
	}

	time.Sleep(2500 * time.Millisecond)
	if _, statErr := os.Stat(sentinel); !os.IsNotExist(statErr) {
		t.Fatalf("expected background child to be terminated, stat err=%v", statErr)
	}
}
