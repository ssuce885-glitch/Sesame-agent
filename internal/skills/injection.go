package skills

import "strings"

type PromptInjection struct {
	ActiveContext string
}

func BuildPromptInjection(activated []ActivatedSkill, suggested []SuggestedSkill, activeSkillTokenBudget int) PromptInjection {
	return PromptInjection{
		ActiveContext: RenderActiveContext(activated, suggested, activeSkillTokenBudget),
	}
}

func RenderActivatedSkillsInjection(activated []ActivatedSkill) string {
	return RenderActiveContext(activated, nil, 0)
}

func RenderActiveContext(activated []ActivatedSkill, suggested []SuggestedSkill, activeSkillTokenBudget int) string {
	lines := make([]string, 0, len(activated)+len(suggested)+4)
	if len(activated) > 0 {
		lines = append(lines, "Active skills:")
		for _, item := range activated {
			skill := item.Skill
			line := "- " + strings.TrimSpace(skill.Name)
			if description := strings.TrimSpace(skill.Description); description != "" {
				line += ": " + description
			}
			lines = append(lines, line)
			if skill.Policy.AllowFullInjection &&
				strings.TrimSpace(skill.Body) != "" &&
				(activeSkillTokenBudget <= 0 || len(skill.Body) <= activeSkillTokenBudget) {
				lines = append(lines, skill.Body)
			} else if strings.TrimSpace(skill.Body) != "" {
				lines = append(lines, "Use `skill_use` with its name if you need the full local instructions.")
			}
			if granted := GrantedTools([]ActivatedSkill{item}); len(granted) > 0 {
				lines = append(lines, "Granted tools: "+strings.Join(granted, ", "))
			}
		}
	}
	if len(suggested) > 0 {
		lines = append(lines, "Suggested skills:")
		for _, item := range suggested {
			line := "- " + strings.TrimSpace(item.Name)
			if description := strings.TrimSpace(item.Description); description != "" {
				line += ": " + description
			}
			if len(item.Reasons) > 0 {
				line += " (match: " + strings.Join(item.Reasons, ", ") + ")"
			}
			if len(item.Grants) > 0 {
				line += " (grants: " + strings.Join(item.Grants, ", ") + ")"
			}
			lines = append(lines, line)
		}
	}
	return strings.Join(lines, "\n")
}
