package permissions

import "strings"

type Decision string

const (
	DecisionAllow Decision = "allow"
	DecisionAsk   Decision = "ask"
	DecisionDeny  Decision = "deny"
)

type Engine struct {
	profile string
}

func NewEngine(profile ...string) *Engine {
	selected := "read_only"
	if len(profile) > 0 {
		value := strings.TrimSpace(profile[0])
		value = strings.NewReplacer("-", "_").Replace(strings.ToLower(value))
		if value != "" {
			selected = value
		}
	}

	return &Engine{profile: selected}
}

func (e *Engine) Decide(toolName string) Decision {
	if e == nil {
		return DecisionDeny
	}

	allowed := map[string]struct{}{
		"file_read":           {},
		"glob":                {},
		"grep":                {},
		"list_dir":            {},
		"request_permissions": {},
		"request_user_input":  {},
		"view_image":          {},
		"web_fetch":           {},
	}
	switch e.profile {
	case "workspace_write":
		allowed["apply_patch"] = struct{}{}
		allowed["file_write"] = struct{}{}
		allowed["file_edit"] = struct{}{}
		allowed["notebook_edit"] = struct{}{}
	case "trusted_local":
		allowed["apply_patch"] = struct{}{}
		allowed["file_write"] = struct{}{}
		allowed["file_edit"] = struct{}{}
		allowed["notebook_edit"] = struct{}{}
		allowed["shell_command"] = struct{}{}
		allowed["todo_write"] = struct{}{}
		allowed["task_create"] = struct{}{}
		allowed["task_get"] = struct{}{}
		allowed["task_list"] = struct{}{}
		allowed["task_output"] = struct{}{}
		allowed["task_stop"] = struct{}{}
		allowed["task_update"] = struct{}{}
		allowed["enter_worktree"] = struct{}{}
		allowed["exit_worktree"] = struct{}{}
		allowed["enter_plan_mode"] = struct{}{}
		allowed["exit_plan_mode"] = struct{}{}
	case "", "read_only":
		// Read-only profile intentionally keeps the base allow-list only.
	default:
		// Unknown profiles fall back to the safest profile.
	}

	if _, ok := allowed[toolName]; ok {
		return DecisionAllow
	}
	if e.profile == "trusted_local" {
		return DecisionAllow
	}
	return DecisionDeny
}
