package skills

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"go-agent/internal/extensions"
)

var skillRefPattern = regexp.MustCompile(`\$([A-Za-z0-9._-]+)`)

type SkillSpec struct {
	Name         string
	Description  string
	Path         string
	Scope        string
	Body         string
	Triggers     []string
	AllowedTools []string
	Policy       SkillPolicy
	Agent        AgentSpec
}

type SkillPolicy struct {
	AllowImplicitActivation bool
	AllowFullInjection      bool
	CapabilityTags          []string
	PreferredTools          []string
}

type AgentSpec struct {
	Type         string
	Description  string
	Instructions string
	Tools        []string
}

type ToolAsset struct {
	Name        string
	Path        string
	Scope       string
	Description string
}

type SkillDirectories struct {
	System    string
	Global    string
	Workspace string
}

type Catalog struct {
	Skills    []SkillSpec
	Tools     []ToolAsset
	SkillDirs SkillDirectories
}

func (c Catalog) SkillNames() []string {
	if len(c.Skills) == 0 {
		return nil
	}
	names := make([]string, 0, len(c.Skills))
	for _, skill := range c.Skills {
		if trimmed := strings.TrimSpace(skill.Name); trimmed != "" {
			names = append(names, trimmed)
		}
	}
	return names
}

type ActivationReason string

const (
	ActivationReasonExplicit  ActivationReason = "explicit"
	ActivationReasonName      ActivationReason = "name_match"
	ActivationReasonTrigger   ActivationReason = "trigger_match"
	ActivationReasonRetrieved ActivationReason = "retrieved"
	ActivationReasonProfile   ActivationReason = "profile_selected"
	ActivationReasonToolUse   ActivationReason = "tool_use"
	ActivationReasonInherited ActivationReason = "inherited"
)

type ActivatedSkill struct {
	Skill       SkillSpec
	Reason      ActivationReason
	MatchedText string
}

func LoadCatalog(globalRoot, workspaceRoot string) (Catalog, error) {
	catalog, err := extensions.LoadCatalog(globalRoot, workspaceRoot)
	if err != nil {
		return Catalog{}, err
	}
	return FromExtensionsCatalog(catalog), nil
}

func FromExtensionsCatalog(src extensions.Catalog) Catalog {
	out := Catalog{
		Skills: make([]SkillSpec, 0, len(src.Skills)),
		Tools:  make([]ToolAsset, 0, len(src.Tools)),
		SkillDirs: SkillDirectories{
			System:    src.SkillDirs.System,
			Global:    src.SkillDirs.Global,
			Workspace: src.SkillDirs.Workspace,
		},
	}
	for _, skill := range src.Skills {
		out.Skills = append(out.Skills, SkillSpec{
			Name:         skill.Name,
			Description:  skill.Description,
			Path:         skill.Path,
			Scope:        skill.Scope,
			Body:         skill.Body,
			Triggers:     append([]string(nil), skill.Triggers...),
			AllowedTools: append([]string(nil), skill.AllowedTools...),
			Policy: SkillPolicy{
				AllowImplicitActivation: skill.Policy.AllowImplicitActivation,
				AllowFullInjection:      skill.Policy.AllowFullInjection,
				CapabilityTags:          append([]string(nil), skill.Policy.CapabilityTags...),
				PreferredTools:          append([]string(nil), skill.Policy.PreferredTools...),
			},
			Agent: AgentSpec{
				Type:         skill.Agent.Type,
				Description:  skill.Agent.Description,
				Instructions: skill.Agent.Instructions,
				Tools:        append([]string(nil), skill.Agent.Tools...),
			},
		})
	}
	for _, tool := range src.Tools {
		out.Tools = append(out.Tools, ToolAsset{
			Name:        tool.Name,
			Path:        tool.Path,
			Scope:       tool.Scope,
			Description: tool.Description,
		})
	}
	return out
}

func Activate(catalog Catalog, userMessage string) []ActivatedSkill {
	if len(catalog.Skills) == 0 || strings.TrimSpace(userMessage) == "" {
		return nil
	}

	names := make(map[string]SkillSpec, len(catalog.Skills))
	for _, skill := range catalog.Skills {
		names[strings.ToLower(strings.TrimSpace(skill.Name))] = skill
	}

	seen := make(map[string]struct{})
	out := make([]ActivatedSkill, 0, 4)
	normalizedMessage := normalizeSkillMatchText(userMessage)

	for _, match := range skillRefPattern.FindAllStringSubmatch(userMessage, -1) {
		if len(match) < 2 {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(match[1]))
		if key == "" {
			continue
		}
		skill, ok := names[key]
		if !ok {
			continue
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, ActivatedSkill{
			Skill:       skill,
			Reason:      ActivationReasonExplicit,
			MatchedText: "$" + strings.TrimSpace(match[1]),
		})
	}

	for _, skill := range catalog.Skills {
		key := strings.ToLower(strings.TrimSpace(skill.Name))
		if key == "" {
			continue
		}
		if _, exists := seen[key]; exists {
			continue
		}
		if normalizedName := normalizeSkillMatchText(skill.Name); normalizedName != "" &&
			strings.Contains(normalizedMessage, normalizedName) {
			seen[key] = struct{}{}
			out = append(out, ActivatedSkill{
				Skill:       skill,
				Reason:      ActivationReasonName,
				MatchedText: skill.Name,
			})
			continue
		}
		for _, trigger := range skill.Triggers {
			if !skill.Policy.AllowImplicitActivation {
				continue
			}
			normalizedTrigger := normalizeSkillMatchText(trigger)
			if normalizedTrigger == "" || !strings.Contains(normalizedMessage, normalizedTrigger) {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, ActivatedSkill{
				Skill:       skill,
				Reason:      ActivationReasonTrigger,
				MatchedText: trigger,
			})
			break
		}
	}

	return out
}

func ActivationNotices(activated []ActivatedSkill) []string {
	if len(activated) == 0 {
		return nil
	}
	notices := make([]string, 0, len(activated))
	for _, skill := range activated {
		if skill.Reason == ActivationReasonProfile {
			continue
		}
		notices = append(notices, fmt.Sprintf("Activated local skill: %s", skill.Skill.Name))
	}
	return notices
}

func PreferredTools(activated []ActivatedSkill) []string {
	if len(activated) == 0 {
		return nil
	}
	seen := make(map[string]struct{})
	out := make([]string, 0, 8)
	for _, item := range activated {
		for _, name := range append(
			append(
				append([]string(nil), item.Skill.Policy.PreferredTools...),
				item.Skill.AllowedTools...,
			),
			item.Skill.Agent.Tools...,
		) {
			trimmed := strings.TrimSpace(name)
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

func GrantedTools(activated []ActivatedSkill) []string {
	if len(activated) == 0 {
		return nil
	}
	seen := make(map[string]struct{})
	out := make([]string, 0, 8)
	for _, item := range activated {
		names := append(
			append([]string(nil), item.Skill.AllowedTools...),
			item.Skill.Agent.Tools...,
		)
		if len(names) == 0 && shouldGrantLegacyExecutionTool(item) {
			names = append(names, "shell_command")
		}
		for _, name := range names {
			trimmed := strings.TrimSpace(name)
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

func shouldGrantLegacyExecutionTool(item ActivatedSkill) bool {
	switch item.Reason {
	case ActivationReasonExplicit, ActivationReasonRetrieved, ActivationReasonToolUse, ActivationReasonInherited:
		return true
	default:
		return false
	}
}

func normalizeSkillMatchText(text string) string {
	return strings.ToLower(strings.TrimSpace(text))
}

func ImplicitHintSkills(catalog Catalog) []SkillSpec {
	if len(catalog.Skills) == 0 {
		return nil
	}
	out := make([]SkillSpec, 0, len(catalog.Skills))
	for _, skill := range catalog.Skills {
		if !skill.Policy.AllowImplicitActivation {
			continue
		}
		out = append(out, skill)
	}
	return out
}

func SelectByCapabilityTags(catalog Catalog, tags []string) []ActivatedSkill {
	if len(catalog.Skills) == 0 || len(tags) == 0 {
		return nil
	}
	normalizedTags := make(map[string]string, len(tags))
	for _, tag := range tags {
		trimmed := strings.TrimSpace(tag)
		if trimmed == "" {
			continue
		}
		normalizedTags[strings.ToLower(trimmed)] = trimmed
	}
	if len(normalizedTags) == 0 {
		return nil
	}
	out := make([]ActivatedSkill, 0, len(catalog.Skills))
	for _, skill := range catalog.Skills {
		for _, tag := range skill.Policy.CapabilityTags {
			normalized := strings.ToLower(strings.TrimSpace(tag))
			matchedTag, ok := normalizedTags[normalized]
			if !ok {
				continue
			}
			out = append(out, ActivatedSkill{
				Skill:       skill,
				Reason:      ActivationReasonProfile,
				MatchedText: matchedTag,
			})
			break
		}
	}
	return out
}

func SelectByNames(catalog Catalog, names []string, reason ActivationReason) []ActivatedSkill {
	if len(catalog.Skills) == 0 || len(names) == 0 {
		return nil
	}
	want := make(map[string]string, len(names))
	for _, name := range names {
		trimmed := strings.TrimSpace(name)
		if trimmed == "" {
			continue
		}
		want[strings.ToLower(trimmed)] = trimmed
	}
	if len(want) == 0 {
		return nil
	}
	out := make([]ActivatedSkill, 0, len(want))
	for _, skill := range catalog.Skills {
		key := strings.ToLower(strings.TrimSpace(skill.Name))
		matched, ok := want[key]
		if !ok {
			continue
		}
		out = append(out, ActivatedSkill{
			Skill:       skill,
			Reason:      reason,
			MatchedText: matched,
		})
	}
	return out
}

func MergeActivatedSkills(groups ...[]ActivatedSkill) []ActivatedSkill {
	seen := make(map[string]struct{})
	out := make([]ActivatedSkill, 0, 8)
	for _, group := range groups {
		for _, item := range group {
			key := strings.ToLower(strings.TrimSpace(item.Skill.Name))
			if key == "" {
				continue
			}
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, item)
		}
	}
	return out
}
