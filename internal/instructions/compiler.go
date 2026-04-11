package instructions

import (
	"fmt"
	"slices"
	"strings"

	"go-agent/internal/skills"
)

type CompileInput struct {
	Catalog         skills.Catalog
	Active          []skills.ActivatedSkill
	NewlyActivated  []skills.ActivatedSkill
	PreviouslyTools []string
}

type Bundle struct {
	sections []string
}

func Compile(in CompileInput) (Bundle, error) {
	if len(in.Active) == 0 {
		section := renderCatalogSection(in.Catalog)
		if strings.TrimSpace(section) == "" {
			return Bundle{}, nil
		}
		return Bundle{sections: []string{section}}, nil
	}

	activeSection, err := renderActiveSkillSection(in.Catalog, in.Active, in.NewlyActivated, in.PreviouslyTools)
	if err != nil {
		return Bundle{}, err
	}
	if strings.TrimSpace(activeSection) == "" {
		return Bundle{}, nil
	}
	return Bundle{sections: []string{activeSection}}, nil
}

func (b Bundle) Render() string {
	return strings.Join(b.sections, "\n\n")
}

func renderCatalogSection(catalog skills.Catalog) string {
	lines := make([]string, 0, len(catalog.Skills)+6)
	lines = append(lines, "Installed local skills:")
	if len(catalog.Skills) == 0 {
		lines = append(lines, "- (none installed)")
	} else {
		for _, skill := range catalog.Skills {
			line := fmt.Sprintf("- %s [%s]", skill.Name, skill.Scope)
			if desc := strings.TrimSpace(skill.Description); desc != "" {
				line += ": " + desc
			}
			lines = append(lines, line)
		}
	}
	lines = append(lines, "Use the `skill_use` tool call with the exact installed skill name to activate a skill.")
	return strings.Join(lines, "\n")
}

func renderActiveSkillSection(catalog skills.Catalog, active []skills.ActivatedSkill, newlyActivated []skills.ActivatedSkill, previouslyTools []string) (string, error) {
	lines := make([]string, 0, len(active)*3+4)
	lines = append(lines, "Activated local skills:")

	for _, activated := range active {
		spec := activated.Skill
		body, err := catalog.ReadBody(spec)
		if err != nil {
			return "", err
		}
		lines = append(lines, "")
		lines = append(lines, fmt.Sprintf("## %s (%s)", spec.Name, spec.Scope))
		if strings.TrimSpace(body) != "" {
			lines = append(lines, body)
		}
	}

	newTools := newlyEnabledTools(newlyActivated, previouslyTools)
	if len(newTools) > 0 {
		lines = append(lines, "")
		lines = append(lines, "Newly enabled tools:")
		for _, tool := range newTools {
			lines = append(lines, "- "+tool)
		}
	}

	return strings.Join(lines, "\n"), nil
}

func newlyEnabledTools(newlyActivated []skills.ActivatedSkill, previouslyTools []string) []string {
	seen := make(map[string]struct{}, len(previouslyTools))
	for _, tool := range previouslyTools {
		tool = strings.TrimSpace(tool)
		if tool == "" {
			continue
		}
		seen[tool] = struct{}{}
	}

	added := make([]string, 0, 8)
	appendTool := func(tool string) {
		tool = strings.TrimSpace(tool)
		if tool == "" {
			return
		}
		if _, ok := seen[tool]; ok {
			return
		}
		seen[tool] = struct{}{}
		added = append(added, tool)
	}

	for _, activated := range newlyActivated {
		for _, tool := range activated.Skill.ToolDependencies {
			appendTool(tool)
		}
		for _, tool := range activated.Skill.PreferredTools {
			appendTool(tool)
		}
	}
	slices.Sort(added)
	return added
}
