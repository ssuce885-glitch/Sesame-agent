package skills

import "strings"

type PromptInjection struct {
	InstalledSkills     string
	ActiveContext       string
	ActiveSkillGuidance string
}

func BuildPromptInjection(catalog Catalog, activated []ActivatedSkill) PromptInjection {
	return PromptInjection{
		InstalledSkills:     RenderInstalledSkillCatalog(catalog, activated),
		ActiveContext:       RenderActiveContext(activated),
		ActiveSkillGuidance: RenderActivatedSkillsInjection(activated),
	}
}

func RenderInstalledSkillCatalog(catalog Catalog, activated []ActivatedSkill) string {
	if len(catalog.Skills) == 0 {
		return ""
	}
	active := make(map[string]struct{}, len(activated))
	for _, item := range activated {
		key := strings.ToLower(strings.TrimSpace(item.Skill.Name))
		if key != "" {
			active[key] = struct{}{}
		}
	}

	lines := []string{
		"Before performing domain-specific work that matches an installed skill's capabilities, activate that skill with `skill_use` first. Installed skills define the correct workflow and constraints for their domain — do not skip them. Multiple skills may be activated in the same turn when needed.",
	}
	count := 0
	for _, skill := range catalog.Skills {
		key := strings.ToLower(strings.TrimSpace(skill.Name))
		if key == "" {
			continue
		}
		if _, ok := active[key]; ok {
			continue
		}
		line := "- " + strings.TrimSpace(skill.Name)
		details := make([]string, 0, 5)
		if scope := strings.TrimSpace(skill.Scope); scope != "" {
			details = append(details, "scope="+scope)
		}
		if description := strings.TrimSpace(skill.Description); description != "" {
			details = append(details, "description="+description)
		}
		if len(skill.Policy.CapabilityTags) > 0 {
			details = append(details, "capabilities="+strings.Join(skill.Policy.CapabilityTags, ", "))
		}
		tools := GrantedTools([]ActivatedSkill{{Skill: skill}})
		if len(tools) > 0 {
			details = append(details, "tools="+strings.Join(tools, ", "))
		}
		if len(details) > 0 {
			line += " [" + strings.Join(details, " | ") + "]"
		}
		lines = append(lines, line)
		count++
	}
	if count == 0 {
		lines = append(lines, "- No additional installed skills are available to activate in this turn.")
	}
	return strings.Join(lines, "\n")
}

func RenderActivatedSkillsInjection(activated []ActivatedSkill) string {
	lines := make([]string, 0, len(activated))
	for _, item := range activated {
		if !item.Skill.Policy.AllowFullInjection {
			continue
		}
		if body := strings.TrimSpace(item.Skill.Body); body != "" {
			lines = append(lines, "Skill: "+strings.TrimSpace(item.Skill.Name)+"\n"+body)
			continue
		}
		if description := strings.TrimSpace(item.Skill.Description); description != "" {
			lines = append(lines, "Skill: "+strings.TrimSpace(item.Skill.Name)+"\n"+description)
		}
	}
	return strings.Join(lines, "\n\n")
}

func RenderActiveContext(activated []ActivatedSkill) string {
	if len(activated) == 0 {
		return ""
	}
	lines := make([]string, 0, len(activated)+2)
	lines = append(lines, "Active skills:")
	for _, item := range activated {
		line := "- " + strings.TrimSpace(item.Skill.Name)
		if description := strings.TrimSpace(item.Skill.Description); description != "" {
			line += ": " + description
		}
		lines = append(lines, line)
	}
	lines = append(lines, "Skills with full-injection enabled may include their instructions below. Use `skill_use` with a skill name when you need to load another skill explicitly for this turn.")
	return strings.Join(lines, "\n")
}
