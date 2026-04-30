package tools

import (
	"context"
	"testing"

	"go-agent/internal/roles"
)

func TestRoleMutationToolsHiddenFromSpecialists(t *testing.T) {
	execCtx := ExecContext{
		RoleService: roles.NewService(),
		RoleSpec:    &roles.Spec{RoleID: "analyst"},
	}
	if (roleCreateTool{}).IsEnabled(execCtx) {
		t.Fatal("role_create should be hidden in specialist role context")
	}
	if (roleUpdateTool{}).IsEnabled(execCtx) {
		t.Fatal("role_update should be hidden in specialist role context")
	}
	if !(roleListTool{}).IsEnabled(execCtx) {
		t.Fatal("role_list should remain visible in specialist role context")
	}
	if !(roleGetTool{}).IsEnabled(execCtx) {
		t.Fatal("role_get should remain visible in specialist role context")
	}
}

func TestRoleUpdatePreservesCatalogMetadataWhenOmitted(t *testing.T) {
	workspaceRoot := t.TempDir()
	roleService := roles.NewService()
	createTool := roleCreateTool{}
	updateTool := roleUpdateTool{}

	_, err := createTool.ExecuteDecoded(context.Background(), DecodedCall{
		Input: RoleUpsertInput{
			RoleID:      "reddit_researcher",
			DisplayName: "Reddit 话题研究员",
			Description: "整理 Reddit 热门讨论",
			Prompt:      "initial prompt",
			Skills:      []string{"scrapling"},
		},
	}, ExecContext{
		WorkspaceRoot: workspaceRoot,
		RoleService:   roleService,
	})
	if err != nil {
		t.Fatalf("create role: %v", err)
	}

	output, err := updateTool.ExecuteDecoded(context.Background(), DecodedCall{
		Input: RoleUpsertInput{
			RoleID: "reddit_researcher",
			Prompt: "updated prompt",
		},
	}, ExecContext{
		WorkspaceRoot: workspaceRoot,
		RoleService:   roleService,
	})
	if err != nil {
		t.Fatalf("update role: %v", err)
	}
	roleOutput := output.Data.(RoleOutput)
	if roleOutput.DisplayName != "Reddit 话题研究员" {
		t.Fatalf("DisplayName = %q", roleOutput.DisplayName)
	}
	if roleOutput.Description != "整理 Reddit 热门讨论" {
		t.Fatalf("Description = %q", roleOutput.Description)
	}
	if len(roleOutput.Skills) != 1 || roleOutput.Skills[0] != "scrapling" {
		t.Fatalf("Skills = %#v", roleOutput.Skills)
	}
	if roleOutput.Prompt == "initial prompt" {
		t.Fatalf("Prompt was not updated")
	}

	spec, err := roleService.Get(workspaceRoot, "reddit_researcher")
	if err != nil {
		t.Fatalf("get role: %v", err)
	}
	if spec.DisplayName != "Reddit 话题研究员" || spec.Description != "整理 Reddit 热门讨论" {
		t.Fatalf("persisted metadata = %q / %q", spec.DisplayName, spec.Description)
	}
}

func TestRoleUpdateAllowsReplacingCatalogMetadata(t *testing.T) {
	workspaceRoot := t.TempDir()
	roleService := roles.NewService()

	if _, err := (roleCreateTool{}).ExecuteDecoded(context.Background(), DecodedCall{
		Input: RoleUpsertInput{
			RoleID:      "researcher",
			DisplayName: "Old Name",
			Description: "Old description",
			Prompt:      "initial prompt",
		},
	}, ExecContext{WorkspaceRoot: workspaceRoot, RoleService: roleService}); err != nil {
		t.Fatalf("create role: %v", err)
	}

	output, err := (roleUpdateTool{}).ExecuteDecoded(context.Background(), DecodedCall{
		Input: RoleUpsertInput{
			RoleID:      "researcher",
			DisplayName: "New Name",
			Description: "New description",
			Prompt:      "updated prompt",
		},
	}, ExecContext{WorkspaceRoot: workspaceRoot, RoleService: roleService})
	if err != nil {
		t.Fatalf("update role: %v", err)
	}
	roleOutput := output.Data.(RoleOutput)
	if roleOutput.DisplayName != "New Name" || roleOutput.Description != "New description" {
		t.Fatalf("metadata = %q / %q", roleOutput.DisplayName, roleOutput.Description)
	}
}
