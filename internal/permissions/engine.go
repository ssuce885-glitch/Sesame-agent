package permissions

import "strings"

type Decision string

const (
	DecisionAllow Decision = "allow"
	DecisionDeny  Decision = "deny"
)

const (
	ProfileReadOnly       = "read_only"
	ProfileWorkspaceWrite = "workspace_write"
	ProfileTrustedLocal   = "trusted_local"

	defaultProfile = ProfileTrustedLocal
)

type Engine struct {
	profile string
}

func NewEngine(profile ...string) *Engine {
	selected := defaultProfile
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
		return defaultProfile
	}
	return e.profile
}

type profileSpec struct {
	Allowed  map[string]struct{}
	Wildcard bool
}

func baseReadOnlyTools() map[string]struct{} {
	return map[string]struct{}{
		"file_read":          {},
		"glob":               {},
		"grep":               {},
		"list_dir":           {},
		"request_user_input": {},
		"skill_use":          {},
		"view_image":         {},
		"web_fetch":          {},
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
	ProfileReadOnly: {
		Allowed: baseReadOnlyTools(),
	},
	ProfileWorkspaceWrite: {
		Allowed: mergeAllowed(
			baseReadOnlyTools(),
			"apply_patch",
			"file_write",
			"file_edit",
			"notebook_edit",
		),
	},
	ProfileTrustedLocal: {
		Wildcard: true,
	},
}

func (e *Engine) AllowsAll() bool {
	if e == nil {
		return false
	}
	spec, ok := profileSpecs[e.Profile()]
	if !ok {
		spec = profileSpecs[defaultProfile]
	}
	return spec.Wildcard
}

func (e *Engine) Decide(toolName string) Decision {
	if e == nil {
		return DecisionDeny
	}

	spec, ok := profileSpecs[e.Profile()]
	if !ok {
		spec = profileSpecs[defaultProfile]
	}
	if _, ok := spec.Allowed[toolName]; ok || spec.Wildcard {
		return DecisionAllow
	}
	return DecisionDeny
}
