package tools

func explicitActiveSkillNames(execCtx ExecContext) []string {
	if len(execCtx.ActiveSkillNames) == 0 {
		return nil
	}

	seen := make(map[string]struct{}, len(execCtx.ActiveSkillNames))
	names := make([]string, 0, len(execCtx.ActiveSkillNames))
	for _, name := range execCtx.ActiveSkillNames {
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		names = append(names, name)
	}
	if len(names) == 0 {
		return nil
	}
	return names
}
