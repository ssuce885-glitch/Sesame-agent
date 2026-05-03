package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go-agent/internal/v2/contracts"
	"go-agent/internal/v2/roles"
)

func TestRoleCreateAndUpdateTools(t *testing.T) {
	root := t.TempDir()
	service := roles.NewService(root)

	createResult, err := NewRoleCreateTool(service).Execute(context.Background(), contracts.ToolCall{
		Name: "role_create",
		Args: map[string]any{
			"id":            "backend_reviewer",
			"name":          "Backend Reviewer",
			"system_prompt": "Review backend changes.",
			"skill_names":   []any{"go", "testing"},
		},
	}, contracts.ExecContext{WorkspaceRoot: root})
	if err != nil {
		t.Fatalf("role_create returned error: %v", err)
	}
	if createResult.IsError {
		t.Fatalf("role_create failed: %s", createResult.Output)
	}
	var created roles.RoleSpec
	if err := json.Unmarshal([]byte(createResult.Output), &created); err != nil {
		t.Fatalf("decode create output: %v", err)
	}
	if created.ID != "backend_reviewer" || created.Version != 1 || len(created.SkillNames) != 2 {
		t.Fatalf("unexpected created role: %+v", created)
	}

	updateResult, err := NewRoleUpdateTool(service).Execute(context.Background(), contracts.ToolCall{
		Name: "role_update",
		Args: map[string]any{
			"id":             "backend_reviewer",
			"description":    "Reviews service and storage changes.",
			"max_tool_calls": float64(12),
		},
	}, contracts.ExecContext{WorkspaceRoot: root})
	if err != nil {
		t.Fatalf("role_update returned error: %v", err)
	}
	if updateResult.IsError {
		t.Fatalf("role_update failed: %s", updateResult.Output)
	}
	var updated roles.RoleSpec
	if err := json.Unmarshal([]byte(updateResult.Output), &updated); err != nil {
		t.Fatalf("decode update output: %v", err)
	}
	if updated.Description != "Reviews service and storage changes." || updated.SystemPrompt != "Review backend changes." || updated.MaxToolCalls != 12 {
		t.Fatalf("unexpected updated role: %+v", updated)
	}
}

func TestRoleInstallTool(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(t.TempDir(), "ops_role")
	if err := os.MkdirAll(source, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "role.yaml"), []byte("display_name: Ops Role\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "prompt.md"), []byte("Operate carefully."), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := NewRoleInstallTool(roles.NewService(root)).Execute(context.Background(), contracts.ToolCall{
		Name: "role_install",
		Args: map[string]any{"source_path": source},
	}, contracts.ExecContext{WorkspaceRoot: root})
	if err != nil {
		t.Fatalf("role_install returned error: %v", err)
	}
	if result.IsError {
		t.Fatalf("role_install failed: %s", result.Output)
	}
	if _, err := os.Stat(filepath.Join(root, "roles", "ops_role", "prompt.md")); err != nil {
		t.Fatalf("expected installed prompt: %v", err)
	}
}

func TestDelegateToRoleToolRejectsRoleWithoutDelegationPolicy(t *testing.T) {
	result, err := NewDelegateToRoleTool(DelegateToolDeps{}).Execute(context.Background(), contracts.ToolCall{
		Name: "delegate_to_role",
		Args: map[string]any{"role": "researcher", "task": "inspect"},
	}, contracts.ExecContext{
		RoleSpec: &contracts.RoleSpec{ID: "worker", CanDelegate: false},
	})
	if err != nil {
		t.Fatalf("delegate_to_role returned error: %v", err)
	}
	if !result.IsError || !strings.Contains(result.Output, `role "worker" cannot delegate`) {
		t.Fatalf("expected delegation policy denial, got %+v", result)
	}
}
