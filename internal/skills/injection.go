package skills

import (
	"fmt"
	"strings"
)

type PromptInjection struct {
	ImplicitHints   string
	RelevantSkills  string
	ActivatedSkills string
}

func BuildPromptInjection(activated []ActivatedSkill, suggested []RetrievalCandidate, includeSuggestedSkills bool) PromptInjection {
	relevantSkills := ""
	if includeSuggestedSkills {
		relevantSkills = RenderRelevantSkills(suggested)
	}
	return PromptInjection{
		ImplicitHints:   RenderImplicitSkillHints(suggested),
		RelevantSkills:  relevantSkills,
		ActivatedSkills: RenderActivatedSkillsInjection(activated),
	}
}

func RenderImplicitSkillHints(suggested []RetrievalCandidate) string {
	hints := make([]RetrievalCandidate, 0, len(suggested))
	for _, candidate := range suggested {
		if !candidate.Skill.Policy.AllowImplicitActivation {
			continue
		}
		hints = append(hints, candidate)
	}
	if len(hints) == 0 {
		return ""
	}
	lines := make([]string, 0, len(hints))
	for _, candidate := range hints {
		skill := candidate.Skill
		line := "- " + strings.TrimSpace(skill.Name)
		if description := strings.TrimSpace(skill.Description); description != "" {
			line += ": " + description
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func RenderActivatedSkillsInjection(activated []ActivatedSkill) string {
	if len(activated) == 0 {
		return ""
	}
	detail := make([]string, 0, len(activated))
	for _, item := range activated {
		skill := item.Skill
		lines := []string{fmt.Sprintf("## %s (%s)", skill.Name, skill.Scope)}
		if skill.Policy.AllowFullInjection {
			if body := strings.TrimSpace(skill.Body); body != "" {
				lines = append(lines, body)
			}
		} else {
			if description := strings.TrimSpace(skill.Description); description != "" {
				lines = append(lines, "Summary: "+description)
			}
			lines = append(lines, "This skill is active as a runtime capability. Use `skill_use` with its name if you need the full local instructions.")
		}
		if granted := GrantedTools([]ActivatedSkill{item}); len(granted) > 0 {
			lines = append(lines, "Granted tools: "+strings.Join(granted, ", "))
		} else if preferred := PreferredTools([]ActivatedSkill{item}); len(preferred) > 0 {
			lines = append(lines, "Preferred tools: "+strings.Join(preferred, ", "))
		}
		detail = append(detail, strings.Join(lines, "\n"))
	}
	if len(detail) == 0 {
		return ""
	}
	return "Activated local skills:\n\n" + strings.Join(detail, "\n\n")
}

func RenderRelevantSkills(suggested []RetrievalCandidate) string {
	relevant := make([]RetrievalCandidate, 0, len(suggested))
	for _, candidate := range suggested {
		if candidate.Skill.Policy.AllowImplicitActivation {
			continue
		}
		relevant = append(relevant, candidate)
	}
	if len(relevant) == 0 {
		return ""
	}
	lines := []string{
		"Relevant installed skills for this task. Call `skill_use` only if you need their full local instructions:",
	}
	for _, candidate := range relevant {
		skill := candidate.Skill
		line := "- " + strings.TrimSpace(skill.Name)
		if description := strings.TrimSpace(skill.Description); description != "" {
			line += ": " + description
		}
		if len(candidate.Reasons) > 0 {
			line += " (match: " + strings.Join(candidate.Reasons, ", ") + ")"
		}
		if granted := GrantedTools([]ActivatedSkill{{
			Skill:  skill,
			Reason: ActivationReasonToolUse,
		}}); len(granted) > 0 {
			line += " (grants: " + strings.Join(granted, ", ") + ")"
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}
