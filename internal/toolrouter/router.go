package toolrouter

import (
	"slices"

	"go-agent/internal/skills"
)

// DecideInput describes the request-shape profile and explicit active skills.
type DecideInput struct {
	Profile      string
	ActiveSkills []skills.ActivatedSkill
}

// Decide returns the visible runtime tools for the request profile plus active skill overlays.
func Decide(in DecideInput) []string {
	base := profileTools[in.Profile]
	visible := make(map[string]struct{}, len(base)+4)
	for _, toolName := range base {
		visible[toolName] = struct{}{}
	}

	// skill_use must stay visible for every profile.
	visible["skill_use"] = struct{}{}

	for _, activated := range in.ActiveSkills {
		for _, dep := range activated.Skill.ToolDependencies {
			visible[dep] = struct{}{}
		}
	}

	names := make([]string, 0, len(visible))
	for name := range visible {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

var profileTools = map[string][]string{
	"codebase-edit": composeProfileTools(
		commonOrchestrationTools,
		[]string{
			"apply_patch",
			"file_edit",
			"file_read",
			"file_write",
			"glob",
			"grep",
			"list_dir",
			"notebook_edit",
		},
	),
	"web-lookup": composeProfileTools(
		commonOrchestrationTools,
		[]string{
			"file_read",
			"glob",
			"grep",
			"list_dir",
			"view_image",
			"web_fetch",
		},
	),
	"system-inspect": composeProfileTools(
		commonOrchestrationTools,
		[]string{
			"file_read",
			"glob",
			"grep",
			"list_dir",
			"shell_command",
		},
	),
}

var commonOrchestrationTools = []string{
	"request_permissions",
	"request_user_input",
	"schedule_report",
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

func composeProfileTools(shared []string, specific []string) []string {
	combined := make([]string, 0, len(shared)+len(specific))
	combined = append(combined, shared...)
	combined = append(combined, specific...)
	return combined
}
