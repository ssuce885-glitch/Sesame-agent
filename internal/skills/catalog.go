package skills

import (
	"fmt"
	"sort"
	"strings"

	"go-agent/internal/extensions"
	skillcatalog "go-agent/internal/skillcatalog"
)

type SkillSpec = skillcatalog.SkillSpec
type SkillPolicy = skillcatalog.SkillPolicy
type AgentSpec = skillcatalog.AgentSpec
type ToolAsset = skillcatalog.ToolAsset
type SkillDirectories = skillcatalog.SkillDirectories
type Catalog = skillcatalog.Catalog

type ActivationReason string

const (
	ActivationReasonToolUse   ActivationReason = "tool_use"
	ActivationReasonInherited ActivationReason = "inherited"
)

type ActivatedSkill struct {
	Skill       SkillSpec
	Reason      ActivationReason
	MatchedText string
}

func LoadCatalog(globalRoot, workspaceRoot string) (Catalog, error) {
	return extensions.LoadCatalog(globalRoot, workspaceRoot)
}

func ActivationNotices(activated []ActivatedSkill) []string {
	if len(activated) == 0 {
		return nil
	}
	notices := make([]string, 0, len(activated))
	for _, skill := range activated {
		notices = append(notices, fmt.Sprintf("Activated local skill: %s", skill.Skill.Name))
	}
	return notices
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
	case ActivationReasonToolUse, ActivationReasonInherited:
		return true
	default:
		return false
	}
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
