package tools

import (
	"fmt"
	"strings"
)

func requireActiveSkills(execCtx ExecContext, names ...string) error {
	missing := missingActiveSkills(execCtx.ActiveSkillNames, names)
	if len(missing) == 0 {
		return nil
	}
	return fmt.Errorf("activate required skills with skill_use before continuing: %s", strings.Join(missing, ", "))
}

func hasActiveSkills(execCtx ExecContext, names ...string) bool {
	return len(missingActiveSkills(execCtx.ActiveSkillNames, names)) == 0
}

func missingActiveSkills(active []string, required []string) []string {
	if len(required) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(active))
	for _, name := range active {
		key := strings.ToLower(strings.TrimSpace(name))
		if key != "" {
			seen[key] = struct{}{}
		}
	}
	missing := make([]string, 0, len(required))
	for _, name := range required {
		trimmed := strings.TrimSpace(name)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[strings.ToLower(trimmed)]; ok {
			continue
		}
		missing = append(missing, trimmed)
	}
	return missing
}
