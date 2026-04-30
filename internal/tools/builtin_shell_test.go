package tools

import (
	"context"
	"testing"

	"go-agent/internal/permissions"
)

func TestShellDecodeRejectsWaitOnlyCommands(t *testing.T) {
	tool := shellTool{}
	cases := []string{
		"sleep 25",
		"sleep 25 && echo done",
		"sleep 25; echo done",
	}
	for _, command := range cases {
		if _, err := tool.Decode(Call{Name: "shell_command", Input: map[string]any{"command": command}}); err == nil {
			t.Fatalf("Decode(%q) succeeded, want error", command)
		}
	}
}

func TestShellDecodeAllowsUsefulCommandsContainingSleep(t *testing.T) {
	tool := shellTool{}
	_, err := tool.Decode(Call{
		Name: "shell_command",
		Input: map[string]any{
			"command": "printf 'sleep 25\\n'",
		},
	})
	if err != nil {
		t.Fatalf("Decode returned error: %v", err)
	}
}

func TestTrustedLocalBypassesDestructiveShellApproval(t *testing.T) {
	tool := shellTool{}
	decoded, err := tool.Decode(Call{
		Name: "shell_command",
		Input: map[string]any{
			"command": "rm -rf /tmp/sesame-permission-test",
		},
	})
	if err != nil {
		t.Fatalf("Decode returned error: %v", err)
	}

	err = checkToolPermission(context.Background(), tool, "shell_command", decoded, ExecContext{
		PermissionEngine: permissions.NewEngine(permissions.ProfileTrustedLocal),
	})
	if err != nil {
		t.Fatalf("checkToolPermission returned error: %v", err)
	}
}
