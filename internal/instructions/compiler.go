package instructions

import (
	"fmt"
	"slices"
	"strings"

	"go-agent/internal/skills"
)

type CompileInput struct {
	Catalog              skills.Catalog
	Active               []skills.ActivatedSkill
	VisibleTools         []string
	PreviousVisibleTools []string
}

type Bundle struct {
	sections []string
}

func Compile(in CompileInput) (Bundle, error) {
	sections := make([]string, 0, 2)
	if section := renderCatalogSection(in.Catalog); strings.TrimSpace(section) != "" {
		sections = append(sections, section)
	}

	if len(in.Active) == 0 {
		if len(sections) == 0 {
			return Bundle{}, nil
		}
		return Bundle{sections: sections}, nil
	}

	activeSection := renderActiveSkillSection(in.Active, in.VisibleTools, in.PreviousVisibleTools)
	if strings.TrimSpace(activeSection) != "" {
		sections = append(sections, activeSection)
	}
	if len(sections) == 0 {
		return Bundle{}, nil
	}
	return Bundle{sections: sections}, nil
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

func renderActiveSkillSection(active []skills.ActivatedSkill, visibleTools []string, previousVisibleTools []string) string {
	lines := make([]string, 0, len(active)*3+4)
	lines = append(lines, "Activated local skills:")

	for _, activated := range active {
		spec := activated.Skill
		lines = append(lines, "")
		lines = append(lines, fmt.Sprintf("## %s (%s)", spec.Name, spec.Scope))
		if strings.TrimSpace(activated.Body) != "" {
			lines = append(lines, activated.Body)
		}
	}

	newTools := newlyEnabledTools(active, visibleTools, previousVisibleTools)
	if len(newTools) > 0 {
		lines = append(lines, "")
		lines = append(lines, "Newly enabled tools:")
		for _, tool := range newTools {
			lines = append(lines, "- "+tool)
		}
	}

	return strings.Join(lines, "\n")
}

func newlyEnabledTools(active []skills.ActivatedSkill, visibleTools []string, previousVisibleTools []string) []string {
	previouslyVisible := make(map[string]struct{}, len(previousVisibleTools))
	for _, tool := range previousVisibleTools {
		tool = strings.TrimSpace(tool)
		if tool == "" {
			continue
		}
		previouslyVisible[tool] = struct{}{}
	}

	activeDependencies := make(map[string]struct{}, 8)
	for _, activated := range active {
		for _, tool := range activated.Skill.ToolDependencies {
			tool = strings.TrimSpace(tool)
			if tool == "" {
				continue
			}
			activeDependencies[tool] = struct{}{}
		}
	}

	newlyVisible := make(map[string]struct{}, len(visibleTools))
	for _, tool := range visibleTools {
		tool = strings.TrimSpace(tool)
		if tool == "" {
			continue
		}
		if _, wasVisible := previouslyVisible[tool]; wasVisible {
			continue
		}
		newlyVisible[tool] = struct{}{}
	}

	added := make([]string, 0, len(newlyVisible))
	for tool := range newlyVisible {
		if _, requiredByActiveSkill := activeDependencies[tool]; requiredByActiveSkill {
			added = append(added, tool)
		}
	}
	slices.Sort(added)
	return added
}
