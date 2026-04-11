package toolrouter

import (
	"reflect"
	"testing"

	"go-agent/internal/skills"
)

func TestDecideUsesRequestShapeBaseProfiles(t *testing.T) {
	t.Run("codebase-edit", func(t *testing.T) {
		got := Decide(DecideInput{Profile: "codebase-edit"})
		want := []string{
			"apply_patch",
			"file_edit",
			"file_read",
			"file_write",
			"glob",
			"grep",
			"list_dir",
			"notebook_edit",
			"request_permissions",
			"request_user_input",
			"skill_use",
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("Decide(codebase-edit) = %v, want %v", got, want)
		}
	})

	t.Run("web-lookup", func(t *testing.T) {
		got := Decide(DecideInput{Profile: "web-lookup"})
		want := []string{
			"file_read",
			"glob",
			"grep",
			"list_dir",
			"request_permissions",
			"request_user_input",
			"skill_use",
			"view_image",
			"web_fetch",
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("Decide(web-lookup) = %v, want %v", got, want)
		}
	})

	t.Run("system-inspect", func(t *testing.T) {
		got := Decide(DecideInput{Profile: "system-inspect"})
		want := []string{
			"file_read",
			"glob",
			"grep",
			"list_dir",
			"request_permissions",
			"request_user_input",
			"shell_command",
			"skill_use",
			"task_create",
			"task_get",
			"task_list",
			"task_output",
			"task_result",
			"task_stop",
			"task_update",
			"task_wait",
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("Decide(system-inspect) = %v, want %v", got, want)
		}
	})
}

func TestDecideKeepsSkillUseVisibleAndAppliesSkillOverlay(t *testing.T) {
	got := Decide(DecideInput{
		Profile: "web-lookup",
		ActiveSkills: []skills.ActivatedSkill{
			{
				Skill: skills.SkillSpec{
					Name:             "overlay-skill",
					ToolDependencies: []string{"shell_command", "shell_command", "task_create"},
				},
			},
		},
	})

	want := []string{
		"file_read",
		"glob",
		"grep",
		"list_dir",
		"request_permissions",
		"request_user_input",
		"shell_command",
		"skill_use",
		"task_create",
		"view_image",
		"web_fetch",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Decide() = %v, want %v", got, want)
	}
}

func TestDecideDoesNotNormalizeOverlayDependencyNames(t *testing.T) {
	got := Decide(DecideInput{
		Profile: "web-lookup",
		ActiveSkills: []skills.ActivatedSkill{
			{
				Skill: skills.SkillSpec{
					Name:             "malformed-overlay",
					ToolDependencies: []string{" shell_command ", ""},
				},
			},
		},
	})

	if !contains(got, " shell_command ") {
		t.Fatalf("Decide() missing exact overlay dependency with whitespace: %v", got)
	}
	if !contains(got, "") {
		t.Fatalf("Decide() missing empty-string overlay dependency: %v", got)
	}
	if contains(got, "shell_command") {
		t.Fatalf("Decide() unexpectedly normalized whitespace dependency to shell_command: %v", got)
	}
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
