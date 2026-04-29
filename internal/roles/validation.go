package roles

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

type Diagnostic struct {
	RoleID string
	Path   string
	Error  string
}

var canonicalRoleIDPattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,63}$`)

func CanonicalRoleID(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", errors.New("role_id is required")
	}
	if trimmed != raw {
		return "", fmt.Errorf("invalid role_id: %s", raw)
	}
	if !canonicalRoleIDPattern.MatchString(raw) {
		return "", fmt.Errorf("invalid role_id: %s", raw)
	}
	return raw, nil
}

func shouldIgnoreInternalRoleDir(name string) bool {
	name = strings.TrimSpace(name)
	return strings.HasPrefix(name, ".role-staging-") || strings.HasPrefix(name, ".role-update-backup-")
}

func normalizeUpsertInput(in UpsertInput) (UpsertInput, error) {
	roleID, err := CanonicalRoleID(in.RoleID)
	if err != nil {
		return UpsertInput{}, err
	}
	return UpsertInput{
		RoleID:      roleID,
		DisplayName: strings.TrimSpace(in.DisplayName),
		Description: strings.TrimSpace(in.Description),
		Prompt:      strings.TrimSpace(in.Prompt),
		SkillNames:  dedupeStrings(in.SkillNames),
		Policy:      normalizeRolePolicyConfig(in.Policy),
		Budget:      normalizeRoleBudgetConfig(in.Budget),
	}, nil
}

func validateUpsertInput(in UpsertInput) error {
	if _, err := CanonicalRoleID(in.RoleID); err != nil {
		return err
	}
	if strings.TrimSpace(in.Prompt) == "" {
		return errors.New("prompt is required")
	}
	return nil
}

func normalizeRolePolicyConfig(in *RolePolicyConfig) *RolePolicyConfig {
	if in == nil {
		return nil
	}
	out := cloneRolePolicyConfig(in)
	out.Model = strings.TrimSpace(out.Model)
	out.PermissionProfile = strings.TrimSpace(out.PermissionProfile)
	out.DeniedTools = dedupeStrings(out.DeniedTools)
	out.MemoryReadScope = strings.TrimSpace(out.MemoryReadScope)
	out.MemoryWriteScope = strings.TrimSpace(out.MemoryWriteScope)
	out.DefaultVisibility = strings.TrimSpace(out.DefaultVisibility)
	out.OutputSchema = strings.TrimSpace(out.OutputSchema)
	out.ReportAudience = dedupeStrings(out.ReportAudience)
	out.AutomationOwnership = dedupeStrings(out.AutomationOwnership)
	return out
}

func normalizeRoleBudgetConfig(in *RoleBudgetConfig) *RoleBudgetConfig {
	if in == nil {
		return nil
	}
	out := cloneRoleBudgetConfig(in)
	out.MaxRuntime = strings.TrimSpace(out.MaxRuntime)
	return out
}
