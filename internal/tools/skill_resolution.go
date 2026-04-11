package tools

import (
	"strings"

	"go-agent/internal/skills"
)

func resolveChildTaskSkillNames(execCtx ExecContext, prompt string) ([]string, error) {
	base := normalizeSkillNameList(execCtx.ActiveSkillNames)
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return base, nil
	}

	catalog, err := skills.LoadCatalog(execCtx.GlobalConfigRoot, execCtx.WorkspaceRoot)
	if err != nil {
		return nil, err
	}

	activated := skills.Activate(catalog, prompt)
	if len(base) > 0 {
		activated = skills.MergeActivatedSkills(
			activated,
			skills.SelectByNames(catalog, base, skills.ActivationReasonInherited),
		)
	}
	retrieval := skills.RetrieveForExecution(catalog, prompt, activated)
	activated = skills.MergeActivatedSkills(activated, retrieval.Selected)
	return activatedSkillNames(activated), nil
}

func activatedSkillNames(activated []skills.ActivatedSkill) []string {
	if len(activated) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(activated))
	names := make([]string, 0, len(activated))
	for _, item := range activated {
		name := strings.TrimSpace(item.Skill.Name)
		if name == "" {
			continue
		}
		key := strings.ToLower(name)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		names = append(names, name)
	}
	return names
}

func normalizeSkillNameList(values []string) []string {
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
	return out
}
