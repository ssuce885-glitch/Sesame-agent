package tools

import (
	"strings"
	"testing"

	"go-agent/internal/roles"
)

func TestCheckRoleToolPolicyAllowsToolsWhenNotDenied(t *testing.T) {
	err := checkRoleToolPolicy("shell_command", Definition{Name: "shell_command"}, ExecContext{
		RoleSpec: &roles.Spec{Policy: &roles.RolePolicyConfig{
			DeniedTools: []string{"file_read"},
		}},
	})
	if err != nil {
		t.Fatalf("checkRoleToolPolicy error = %v, want nil (tool not in denied list)", err)
	}
}

func TestCheckRoleToolPolicyDeniesExplicitDeniedTool(t *testing.T) {
	err := checkRoleToolPolicy("shell_command", Definition{Name: "shell_command"}, ExecContext{
		RoleSpec: &roles.Spec{Policy: &roles.RolePolicyConfig{
			DeniedTools: []string{"shell_command"},
		}},
	})
	if err == nil || !strings.Contains(err.Error(), "denied_tools") {
		t.Fatalf("checkRoleToolPolicy error = %v, want denied_tools denial", err)
	}
}
