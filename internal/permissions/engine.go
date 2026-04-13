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

func (e *Engine) Profile() string {
	if e == nil || strings.TrimSpace(e.profile) == "" {
		return "read_only"
	}
	return e.profile
}

type profileSpec struct {
	Allowed  map[string]struct{}
	Wildcard bool
}

func baseReadOnlyTools() map[string]struct{} {
	return map[string]struct{}{
		"file_read":           {},
		"glob":                {},
		"grep":                {},
		"list_dir":            {},
		"request_permissions": {},
		"request_user_input":  {},
		"skill_use":           {},
		"view_image":          {},
		"web_fetch":           {},
	}
}

func mergeAllowed(base map[string]struct{}, tools ...string) map[string]struct{} {
	merged := make(map[string]struct{}, len(base)+len(tools))
	for key := range base {
		merged[key] = struct{}{}
	}
	for _, tool := range tools {
		merged[tool] = struct{}{}
	}
	return merged
}

var profileSpecs = map[string]profileSpec{
	"read_only": {
		Allowed: baseReadOnlyTools(),
	},
	"workspace_write": {
		Allowed: mergeAllowed(
			baseReadOnlyTools(),
			"apply_patch",
			"file_write",
			"file_edit",
			"notebook_edit",
		),
	},
	"trusted_local": {
		Allowed: mergeAllowed(
			baseReadOnlyTools(),
			"apply_patch",
			"file_write",
			"file_edit",
			"notebook_edit",
			"shell_command",
			"todo_write",
			"task_create",
			"task_get",
			"task_list",
			"task_output",
			"task_result",
			"task_wait",
			"task_stop",
			"task_update",
			"enter_worktree",
			"exit_worktree",
			"enter_plan_mode",
			"exit_plan_mode",
		),
		Wildcard: true,
	},
}

func (e *Engine) Decide(toolName string) Decision {
	if e == nil {
		return DecisionDeny
	}

	spec, ok := profileSpecs[e.Profile()]
	if !ok {
		spec = profileSpecs["read_only"]
	}
	if _, ok := spec.Allowed[toolName]; ok || spec.Wildcard {
		return DecisionAllow
	}
	return DecisionDeny
}
