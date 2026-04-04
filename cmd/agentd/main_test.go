package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go-agent/internal/config"
	contextstate "go-agent/internal/context"
	"go-agent/internal/permissions"
	"go-agent/internal/tools"
)

func TestEnsureDataDirCreatesMissingDirectory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runtime", "data")

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected %q to not exist before ensureDataDir, err = %v", path, err)
	}

	if err := ensureDataDir(path); err != nil {
		t.Fatalf("ensureDataDir() error = %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("%q is not a directory", path)
	}
}

func TestBuildRuntimeWiringUsesConfig(t *testing.T) {
	cfg := config.Config{
		PermissionProfile:   "trusted_local",
		MaxToolSteps:        11,
		MaxShellOutputBytes: 18,
		ShellTimeoutSeconds: 4,
		MaxFileWriteBytes:   23,
		MaxRecentItems:      2,
		CompactionThreshold: 7,
		MaxEstimatedTokens:  42,
	}

	permissionEngine := buildPermissionEngine(cfg)
	if permissionEngine.Decide("file_write") != permissions.DecisionAllow {
		t.Fatal("buildPermissionEngine() did not honor trusted_local profile for file_write")
	}
	if permissionEngine.Decide("shell_command") != permissions.DecisionAllow {
		t.Fatal("buildPermissionEngine() did not honor trusted_local profile for shell_command")
	}

	ctxCfg := buildContextManagerConfig(cfg)
	if ctxCfg != (contextstate.Config{
		MaxRecentItems:      2,
		MaxEstimatedTokens:  42,
		CompactionThreshold: 7,
	}) {
		t.Fatalf("buildContextManagerConfig() = %#v, want cfg-derived context settings", ctxCfg)
	}

	if got := buildMaxToolSteps(cfg); got != 11 {
		t.Fatalf("buildMaxToolSteps() = %d, want 11", got)
	}
}

func TestConfigureRuntimeGuardrailsAffectsTools(t *testing.T) {
	t.Cleanup(func() {
		tools.SetShellCommandGuardrails(256, 30)
		tools.SetFileWriteMaxBytes(1 << 20)
	})

	configureRuntimeGuardrails(config.Config{
		MaxShellOutputBytes: 12,
		ShellTimeoutSeconds: 30,
		MaxFileWriteBytes:   7,
	})

	workspace := t.TempDir()
	registry := tools.NewRegistry()

	t.Run("file write respects configured limit", func(t *testing.T) {
		_, err := registry.Execute(context.Background(), tools.Call{
			Name:  "file_write",
			Input: map[string]any{"path": filepath.Join(workspace, "too-big.txt"), "content": "12345678"},
		}, tools.ExecContext{
			WorkspaceRoot:    workspace,
			PermissionEngine: permissions.NewEngine("trusted_local"),
		})
		if err == nil || !strings.Contains(err.Error(), "exceeds max size") {
			t.Fatalf("file_write error = %v, want size limit error", err)
		}
	})

	t.Run("shell command respects configured output limit and workspace", func(t *testing.T) {
		tools.SetShellCommandGuardrails(128, 30)
		result, err := registry.Execute(context.Background(), tools.Call{
			Name:  "shell_command",
			Input: map[string]any{"command": "echo %cd%"},
		}, tools.ExecContext{
			WorkspaceRoot:    workspace,
			PermissionEngine: permissions.NewEngine("trusted_local"),
		})
		if err != nil {
			t.Fatalf("shell_command error = %v", err)
		}
		if !strings.Contains(result.Text, workspace) {
			t.Fatalf("shell_command output = %q, want workspace root %q", result.Text, workspace)
		}

		tools.SetShellCommandGuardrails(4, 30)
		result, err = registry.Execute(context.Background(), tools.Call{
			Name:  "shell_command",
			Input: map[string]any{"command": "echo 1234567890"},
		}, tools.ExecContext{
			WorkspaceRoot:    workspace,
			PermissionEngine: permissions.NewEngine("trusted_local"),
		})
		if err != nil {
			t.Fatalf("shell_command error = %v", err)
		}
		if len(result.Text) != 4 {
			t.Fatalf("len(shell_command output) = %d, want 4", len(result.Text))
		}
	})
}
