package roles

import (
	"errors"
	"fmt"
	"strings"
)

type Diagnostic struct {
	RoleID string
	Path   string
	Error  string
}

func CanonicalRoleID(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", errors.New("role_id is required")
	}
	if trimmed != raw {
		return "", fmt.Errorf("invalid role_id: %s", raw)
	}
	if raw == "." || strings.HasPrefix(raw, ".") {
		return "", fmt.Errorf("invalid role_id: %s", raw)
	}
	if strings.Contains(raw, "/") || strings.Contains(raw, "\\") || strings.Contains(raw, "..") {
		return "", fmt.Errorf("invalid role_id: %s", raw)
	}
	return raw, nil
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
