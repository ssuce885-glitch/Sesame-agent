package skills

import "strings"

type PromptInjection struct {
	ActiveContext string
}

func BuildPromptInjection(activated []ActivatedSkill) PromptInjection {
	return PromptInjection{
		ActiveContext: RenderActiveContext(activated),
	}
}

func RenderActivatedSkillsInjection(activated []ActivatedSkill) string {
	lines := make([]string, 0, len(activated))
	for _, item := range activated {
		if body := strings.TrimSpace(item.Skill.Body); body != "" {
			lines = append(lines, body)
			continue
		}
		if description := strings.TrimSpace(item.Skill.Description); description != "" {
			lines = append(lines, description)
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
		lines = append(lines, "- "+strings.TrimSpace(item.Skill.Name))
	}
	lines = append(lines, "Use `skill_use` with its name if you need the full local instructions.")
	return strings.Join(lines, "\n")
}
