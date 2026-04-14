package toolrouter

import (
	"sort"
	"strings"

	"go-agent/internal/intent"
	"go-agent/internal/skills"
	"go-agent/internal/tools"
)

type CapabilityProfile string

const (
	ProfileCodebaseEdit      CapabilityProfile = "codebase_edit"
	ProfileSystemInspect     CapabilityProfile = "system_inspect"
	ProfileWebLookup         CapabilityProfile = "web_lookup"
	ProfileBrowserAutomation CapabilityProfile = "browser_automation"
	ProfileAutomation        CapabilityProfile = "automation"
	ProfileScheduledReport   CapabilityProfile = "scheduled_report"
)

type PolicySummary struct {
	Profile          CapabilityProfile
	Guidance         []string
	PreferredTools   []string
	VisibleTools     []string
	HiddenTools      []string
	MaxSteps         int
	MaxFetches       int
	ForbiddenActions []string
	StopConditions   []string
	SkillTags        []string
}

type Decision struct {
	Summary    PolicySummary
	visibleSet map[string]struct{}
	hiddenSet  map[string]struct{}
}

type profileSpec struct {
	Guidance         []string
	VisibleTools     []string
	HiddenTools      []string
	BlockedTools     []string
	PreferredTools   []string
	MaxSteps         int
	MaxFetches       int
	ForbiddenActions []string
	StopConditions   []string
	SkillTags        []string
	ExposeSkillUse   bool
	AllowSkillGrants bool
}

var (
	metaTools = []string{
		"skill_use",
	}
	readOnlyTools = []string{
		"file_read",
		"glob",
		"grep",
		"list_dir",
		"request_permissions",
		"request_user_input",
		"view_image",
		"web_fetch",
	}
	editTools = []string{
		"apply_patch",
		"file_edit",
		"file_write",
		"notebook_edit",
		"todo_write",
	}
	taskTools = []string{
		"task_create",
		"task_get",
		"task_list",
		"task_output",
		"task_result",
		"task_stop",
		"task_update",
		"task_wait",
	}
	automationTools = []string{
		"automation_apply",
		"automation_control",
		"automation_get",
		"automation_list",
		"incident_ack",
		"incident_control",
		"incident_get",
		"incident_list",
	}
)

func Decide(input any, extra any) Decision {
	switch value := input.(type) {
	case intent.Plan:
		if resolution, ok := extra.(skills.Resolution); ok {
			return decideFromPlan(value, resolution)
		}
		return decideFromPlan(value, skills.Resolution{})
	default:
		return decideFromPlan(intent.Plan{Profile: intent.ProfileCodebaseEdit}, skills.Resolution{})
	}
}

func decideFromPlan(plan intent.Plan, resolution skills.Resolution) Decision {
	profile := CapabilityProfile(plan.Profile)
	if profile == "" {
		profile = ProfileCodebaseEdit
	}
	activated := append([]skills.ActivatedSkill(nil), resolution.Activated...)
	spec := profileFor(profile)
	visibleTools := normalizeToolList(spec.VisibleTools)
	if spec.ExposeSkillUse {
		visibleTools = normalizeToolList(append(append([]string(nil), visibleTools...), metaTools...))
	}
	preferredTools := normalizeToolList(append(append([]string(nil), spec.PreferredTools...), skills.PreferredTools(activated)...))
	hiddenTools := normalizeToolList(spec.HiddenTools)
	blockedTools := normalizeToolList(spec.BlockedTools)
	blockedSet := make(map[string]struct{}, len(blockedTools))
	for _, name := range blockedTools {
		blockedSet[strings.ToLower(strings.TrimSpace(name))] = struct{}{}
	}
	if spec.AllowSkillGrants {
		grantedTools := skills.GrantedTools(activated)
		allowedGrants := make([]string, 0, len(grantedTools))
		for _, name := range grantedTools {
			normalized := strings.ToLower(strings.TrimSpace(name))
			if _, blocked := blockedSet[normalized]; blocked {
				continue
			}
			allowedGrants = append(allowedGrants, name)
		}
		visibleTools = normalizeToolList(append(append([]string(nil), visibleTools...), allowedGrants...))
		hiddenTools = removeTools(hiddenTools, allowedGrants)
	}

	visibleSet := make(map[string]struct{}, len(visibleTools))
	for _, name := range visibleTools {
		visibleSet[strings.ToLower(strings.TrimSpace(name))] = struct{}{}
	}
	hiddenSet := make(map[string]struct{}, len(hiddenTools))
	for _, name := range hiddenTools {
		hiddenSet[strings.ToLower(strings.TrimSpace(name))] = struct{}{}
	}

	return Decision{
		Summary: PolicySummary{
			Profile:          profile,
			Guidance:         append([]string(nil), spec.Guidance...),
			PreferredTools:   preferredTools,
			VisibleTools:     visibleTools,
			HiddenTools:      hiddenTools,
			MaxSteps:         spec.MaxSteps,
			MaxFetches:       spec.MaxFetches,
			ForbiddenActions: append([]string(nil), spec.ForbiddenActions...),
			StopConditions:   append([]string(nil), spec.StopConditions...),
			SkillTags:        append([]string(nil), spec.SkillTags...),
		},
		visibleSet: visibleSet,
		hiddenSet:  hiddenSet,
	}
}

func (d Decision) FilterDefinitions(defs []tools.Definition) []tools.Definition {
	if len(defs) == 0 {
		return nil
	}
	out := make([]tools.Definition, 0, len(defs))
	for _, def := range defs {
		name := strings.ToLower(strings.TrimSpace(def.Name))
		if len(d.visibleSet) > 0 {
			if _, ok := d.visibleSet[name]; !ok {
				continue
			}
		}
		if _, hidden := d.hiddenSet[name]; hidden {
			continue
		}
		out = append(out, def)
	}
	return out
}

func profileFor(profile CapabilityProfile) profileSpec {
	switch profile {
	case ProfileAutomation:
		return profileSpec{
			Guidance: []string{
				"This request is automation setup or automation management. Keep the turn in the automation compile-and-manage path.",
				"Draft the script-backed automation first, then summarize assumptions, then wait for explicit user confirmation before automation_apply.",
				"Use only the automation tools for apply, lookup, control, and incident inspection.",
				"Manual automation testing must emit a synthetic trigger; never launch child agents, shell loops, or watcher runners directly from this profile.",
				"Do not fall back to schedule_report, task_create, shell_command, or ad hoc long-running loops for automation creation.",
			},
			VisibleTools:     normalizeToolList(append(append([]string(nil), automationTools...), readOnlyTools...)),
			HiddenTools:      normalizeToolList(append(append([]string(nil), editTools...), append(append([]string(nil), taskTools...), "schedule_report", "shell_command")...)),
			BlockedTools:     normalizeToolList(append(append([]string(nil), editTools...), append(append([]string(nil), taskTools...), "schedule_report", "shell_command")...)),
			PreferredTools:   []string{"automation_apply", "automation_get", "automation_list", "automation_control", "incident_ack", "incident_control", "incident_get", "incident_list"},
			MaxSteps:         8,
			SkillTags:        []string{"automation_standard_behavior"},
			ExposeSkillUse:   false,
			AllowSkillGrants: false,
			ForbiddenActions: []string{
				"Do not fake automation with background shell loops, direct task launches, or delayed reports.",
				"Do not use non-automation tools during an automation compile turn unless the runtime changes profiles later.",
				"Do not launch child agents, watcher runners, shell loops, or tasks directly during automation compile and manage turns.",
			},
			StopConditions: []string{
				"Stop after the automation definition is drafted, normalized, applied, or the missing automation fields are clarified.",
			},
		}
	case ProfileSystemInspect:
		return profileSpec{
			Guidance: []string{
				"Inspect the local environment directly when the user asks about the current system, binaries, versions, or runtime state.",
			},
			VisibleTools: append(append([]string(nil), readOnlyTools...), "shell_command"),
			HiddenTools:  append(append([]string(nil), editTools...), append(append([]string(nil), taskTools...), "schedule_report")...),
			BlockedTools: append(append([]string(nil), taskTools...), "schedule_report"),
			PreferredTools: []string{
				"shell_command",
			},
			MaxSteps:         6,
			ExposeSkillUse:   true,
			AllowSkillGrants: true,
			ForbiddenActions: []string{
				"Do not modify files unless the user explicitly asks for changes.",
			},
			StopConditions: []string{
				"Stop after the requested system facts are verified.",
			},
		}
	case ProfileWebLookup:
		return profileSpec{
			Guidance: []string{
				"Use direct web fetching for public web pages, news, weather, and webpage summaries.",
				"Prefer answering directly from fetched pages instead of probing the local environment.",
			},
			VisibleTools: append([]string(nil), readOnlyTools...),
			HiddenTools:  normalizeToolList(append(append([]string(nil), editTools...), append(append([]string(nil), taskTools...), "schedule_report", "shell_command")...)),
			BlockedTools: normalizeToolList(append(append([]string(nil), taskTools...), "schedule_report")),
			PreferredTools: []string{
				"web_fetch",
			},
			MaxSteps:         4,
			MaxFetches:       3,
			ExposeSkillUse:   true,
			AllowSkillGrants: true,
			ForbiddenActions: []string{
				"Do not use shell_command for environment probes during ordinary web lookup.",
				"Do not use task_create or schedule_report for a direct lookup request.",
			},
			StopConditions: []string{
				"Stop once fetched pages are sufficient to answer the request.",
			},
		}
	case ProfileBrowserAutomation:
		return profileSpec{
			Guidance: []string{
				"Use browser-oriented skills only when the user explicitly asks for webpage interaction, login, clicking, screenshots, or form submission.",
			},
			VisibleTools: append([]string(nil), readOnlyTools...),
			HiddenTools:  normalizeToolList(append(append([]string(nil), editTools...), append(append([]string(nil), taskTools...), "schedule_report", "shell_command")...)),
			BlockedTools: normalizeToolList(append(append([]string(nil), taskTools...), "schedule_report")),
			PreferredTools: []string{
				"web_fetch",
			},
			MaxSteps:         6,
			MaxFetches:       2,
			ExposeSkillUse:   true,
			AllowSkillGrants: true,
			ForbiddenActions: []string{
				"Do not fall back to local environment probing for browser tasks.",
			},
			StopConditions: []string{
				"Stop after the requested webpage interaction path is identified or the limitation is clear.",
			},
			SkillTags: []string{
				"browser_automation",
			},
		}
	case ProfileScheduledReport:
		return profileSpec{
			Guidance: []string{
				"This request is a delayed or recurring report. Use schedule_report.",
				"Do not fake delayed reporting with task_create.",
				"Do not pre-create a background task for the future run; the scheduled job will create its own execution when it fires.",
				"Do not fetch web data, inspect the local environment, or gather report contents now unless the user explicitly asks for an immediate preview in addition to the scheduled run.",
			},
			VisibleTools: []string{"schedule_report"},
			HiddenTools:  normalizeToolList(append(append(append([]string(nil), readOnlyTools...), editTools...), append(append([]string(nil), taskTools...), "shell_command")...)),
			BlockedTools: normalizeToolList(append(append(append([]string(nil), readOnlyTools...), editTools...), append(append([]string(nil), taskTools...), "shell_command")...)),
			PreferredTools: []string{
				"schedule_report",
			},
			MaxSteps: 4,
			ForbiddenActions: []string{
				"Do not combine task_create and schedule_report for the same delayed objective.",
				"Do not fetch report data during scheduling unless the user explicitly asked for both schedule creation and an immediate answer.",
			},
			StopConditions: []string{
				"Stop after the scheduled job is created or the missing scheduling details are clarified.",
			},
		}
	default:
		return profileSpec{
			Guidance: []string{
				"Treat this as a codebase task: inspect relevant files before editing or answering.",
			},
			VisibleTools: normalizeToolList(append(append(append([]string(nil), readOnlyTools...), editTools...), append(append([]string(nil), taskTools...), "schedule_report", "shell_command")...)),
			HiddenTools:  nil,
			PreferredTools: []string{
				"file_read",
				"grep",
				"glob",
				"apply_patch",
			},
			MaxSteps:         12,
			ExposeSkillUse:   true,
			AllowSkillGrants: true,
			StopConditions: []string{
				"Stop after the requested code or workspace change is verified.",
			},
		}
	}
}

func normalizeToolList(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		key := strings.ToLower(trimmed)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, trimmed)
	}
	sort.Slice(out, func(i, j int) bool {
		left := strings.ToLower(out[i])
		right := strings.ToLower(out[j])
		if left == right {
			return out[i] < out[j]
		}
		return left < right
	})
	return out
}

func removeTools(values []string, remove []string) []string {
	if len(values) == 0 || len(remove) == 0 {
		return values
	}
	blocked := make(map[string]struct{}, len(remove))
	for _, value := range remove {
		key := strings.ToLower(strings.TrimSpace(value))
		if key == "" {
			continue
		}
		blocked[key] = struct{}{}
	}
	if len(blocked) == 0 {
		return values
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		key := strings.ToLower(strings.TrimSpace(value))
		if _, ok := blocked[key]; ok {
			continue
		}
		out = append(out, value)
	}
	return normalizeToolList(out)
}
