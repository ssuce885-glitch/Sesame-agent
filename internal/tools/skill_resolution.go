package tools

import "strings"

func resolveChildTaskSkillNames(execCtx ExecContext, prompt string) ([]string, error) {
	base := normalizeSkillNameList(execCtx.ActiveSkillNames)
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return base, nil
	}
	return base, nil
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
