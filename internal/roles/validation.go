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
		Policy:      normalizePolicyMap(in.Policy),
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

func normalizePolicyMap(policy map[string]any) map[string]any {
	if len(policy) == 0 {
		return map[string]any{}
	}
	out := make(map[string]any, len(policy))
	for key, value := range policy {
		out[key] = value
	}
	return out
}
