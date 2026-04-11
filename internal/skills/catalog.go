package skills

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go-agent/internal/extensions"
)

type SkillSpec struct {
	Name             string
	Description      string
	Path             string
	Scope            string
	WhenToUse        string
	ToolDependencies []string
	PreferredTools   []string
	ExecutionMode    string
	AgentType        string
	EnvDependencies  []string
}

type ActivatedSkill struct {
	Skill SkillSpec
	Body  string
}

type SkillDirectories struct {
	System    string
	Global    string
	Workspace string
}

type Catalog struct {
	Skills    []SkillSpec
	SkillDirs SkillDirectories
}

func LoadCatalog(globalRoot, workspaceRoot string) (Catalog, error) {
	raw, err := extensions.LoadCatalog(globalRoot, workspaceRoot)
	if err != nil {
		return Catalog{}, err
	}

	skills := make([]SkillSpec, 0, len(raw.Skills))
	for _, skill := range raw.Skills {
		skills = append(skills, SkillSpec{
			Name:             skill.Name,
			Description:      skill.Description,
			Path:             skill.Path,
			Scope:            skill.Scope,
			WhenToUse:        skill.WhenToUse,
			ToolDependencies: append([]string(nil), skill.ToolDependencies...),
			PreferredTools:   append([]string(nil), skill.PreferredTools...),
			ExecutionMode:    skill.ExecutionMode,
			AgentType:        skill.AgentType,
			EnvDependencies:  append([]string(nil), skill.EnvDependencies...),
		})
	}

	return Catalog{
		Skills: skills,
		SkillDirs: SkillDirectories{
			System:    raw.SkillDirs.System,
			Global:    raw.SkillDirs.Global,
			Workspace: raw.SkillDirs.Workspace,
		},
	}, nil
}

func (c Catalog) FindByName(name string) (SkillSpec, bool) {
	for _, skill := range c.Skills {
		if skill.Name == name {
			return skill, true
		}
	}
	return SkillSpec{}, false
}

func (c Catalog) ReadBody(skill SkillSpec) (string, error) {
	if strings.TrimSpace(skill.Path) == "" {
		return "", fmt.Errorf("skill path is empty for %q", skill.Name)
	}
	data, err := os.ReadFile(filepath.Join(skill.Path, "SKILL.md"))
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

func MergeActive(existing []ActivatedSkill, incoming ...ActivatedSkill) []ActivatedSkill {
	if len(incoming) == 0 {
		return append([]ActivatedSkill(nil), existing...)
	}

	merged := make([]ActivatedSkill, 0, len(existing)+len(incoming))
	seen := make(map[string]struct{}, len(existing)+len(incoming))

	for _, skill := range existing {
		name := skill.Skill.Name
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		merged = append(merged, skill)
	}
	for _, skill := range incoming {
		name := skill.Skill.Name
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		merged = append(merged, skill)
	}
	return merged
}

func ActiveSkillNames(active []ActivatedSkill) []string {
	names := make([]string, 0, len(active))
	for _, skill := range active {
		names = append(names, skill.Skill.Name)
	}
	return names
}

func ActivateByNames(catalog Catalog, names []string) ([]ActivatedSkill, error) {
	if len(names) == 0 {
		return nil, nil
	}
	activated := make([]ActivatedSkill, 0, len(names))
	for _, name := range names {
		spec, ok := catalog.FindByName(name)
		if !ok {
			return nil, fmt.Errorf("skill %q not found", name)
		}
		body, err := catalog.ReadBody(spec)
		if err != nil {
			return nil, fmt.Errorf("read skill body for %q: %w", spec.Name, err)
		}
		activated = append(activated, ActivatedSkill{
			Skill: spec,
			Body:  body,
		})
	}
	return MergeActive(nil, activated...), nil
}
