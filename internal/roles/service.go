package roles

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type UpsertInput struct {
	RoleID      string   `json:"role_id"`
	DisplayName string   `json:"display_name"`
	Description string   `json:"description"`
	Prompt      string   `json:"prompt"`
	SkillNames  []string `json:"skills"`
}

type Service struct{}

func NewService() *Service { return &Service{} }

type ErrorKind string

const (
	ErrorKindInvalidInput ErrorKind = "invalid_input"
	ErrorKindNotFound     ErrorKind = "not_found"
	ErrorKindConflict     ErrorKind = "conflict"
	ErrorKindInternal     ErrorKind = "internal"
)

type ServiceError struct {
	kind  ErrorKind
	cause error
}

func (e *ServiceError) Error() string {
	switch e.kind {
	case ErrorKindInvalidInput:
		return "invalid input"
	case ErrorKindNotFound:
		return "not found"
	case ErrorKindConflict:
		return "conflict"
	default:
		return "internal error"
	}
}

func (e *ServiceError) Unwrap() error { return e.cause }

func KindOf(err error) ErrorKind {
	if err == nil {
		return ""
	}
	var serviceErr *ServiceError
	if errors.As(err, &serviceErr) {
		return serviceErr.kind
	}
	if errors.Is(err, os.ErrNotExist) {
		return ErrorKindNotFound
	}
	return ErrorKindInternal
}

func IsInvalidInput(err error) bool { return KindOf(err) == ErrorKindInvalidInput }

func IsNotFound(err error) bool { return KindOf(err) == ErrorKindNotFound }

func IsConflict(err error) bool { return KindOf(err) == ErrorKindConflict }

func IsInternal(err error) bool { return KindOf(err) == ErrorKindInternal }

func (s *Service) List(workspaceRoot string) (Catalog, error) {
	return LoadCatalog(strings.TrimSpace(workspaceRoot))
}

func (s *Service) Get(workspaceRoot, roleID string) (Spec, error) {
	workspaceRoot = strings.TrimSpace(workspaceRoot)
	roleID = strings.TrimSpace(roleID)
	if err := validateWorkspaceRoot(workspaceRoot); err != nil {
		return Spec{}, newServiceError(ErrorKindInvalidInput, err)
	}
	if err := validateRoleID(roleID); err != nil {
		return Spec{}, newServiceError(ErrorKindInvalidInput, err)
	}
	spec, err := loadRoleSpec(filepath.Join(workspaceRoot, "roles"), roleID)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Spec{}, newServiceError(ErrorKindNotFound, err)
		}
		return Spec{}, newServiceError(ErrorKindInternal, err)
	}
	return spec, nil
}

func (s *Service) Create(workspaceRoot string, in UpsertInput) (Spec, error) {
	workspaceRoot = strings.TrimSpace(workspaceRoot)
	normalized := normalizeUpsertInput(in)
	if err := validateWorkspaceRoot(workspaceRoot); err != nil {
		return Spec{}, newServiceError(ErrorKindInvalidInput, err)
	}
	if err := validateUpsertInput(normalized); err != nil {
		return Spec{}, newServiceError(ErrorKindInvalidInput, err)
	}
	rolesRoot := filepath.Join(workspaceRoot, "roles")
	roleDir := filepath.Join(rolesRoot, normalized.RoleID)
	if _, err := os.Stat(roleDir); err == nil {
		return Spec{}, newServiceError(ErrorKindConflict, fmt.Errorf("role already exists: %s", normalized.RoleID))
	} else if !errors.Is(err, os.ErrNotExist) {
		return Spec{}, newServiceError(ErrorKindInternal, err)
	}
	stagingDir, err := prepareRoleStagingDir(rolesRoot, normalized)
	if err != nil {
		return Spec{}, err
	}
	defer func() { _ = os.RemoveAll(stagingDir) }()
	if err := os.Rename(stagingDir, roleDir); err != nil {
		if errors.Is(err, os.ErrExist) {
			return Spec{}, newServiceError(ErrorKindConflict, err)
		}
		return Spec{}, newServiceError(ErrorKindInternal, err)
	}
	spec, err := loadRoleSpec(rolesRoot, normalized.RoleID)
	if err != nil {
		return Spec{}, newServiceError(ErrorKindInternal, err)
	}
	return spec, nil
}

func (s *Service) Update(workspaceRoot string, in UpsertInput) (Spec, error) {
	workspaceRoot = strings.TrimSpace(workspaceRoot)
	normalized := normalizeUpsertInput(in)
	if err := validateWorkspaceRoot(workspaceRoot); err != nil {
		return Spec{}, newServiceError(ErrorKindInvalidInput, err)
	}
	if err := validateUpsertInput(normalized); err != nil {
		return Spec{}, newServiceError(ErrorKindInvalidInput, err)
	}
	rolesRoot := filepath.Join(workspaceRoot, "roles")
	roleDir := filepath.Join(rolesRoot, normalized.RoleID)
	if _, err := os.Stat(roleDir); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Spec{}, newServiceError(ErrorKindNotFound, err)
		}
		return Spec{}, newServiceError(ErrorKindInternal, err)
	}

	stagingDir, err := prepareRoleStagingDir(rolesRoot, normalized)
	if err != nil {
		return Spec{}, err
	}
	defer func() { _ = os.RemoveAll(stagingDir) }()

	backupDir := filepath.Join(rolesRoot, fmt.Sprintf(".%s.backup.%d", normalized.RoleID, time.Now().UnixNano()))
	if err := os.Rename(roleDir, backupDir); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Spec{}, newServiceError(ErrorKindNotFound, err)
		}
		return Spec{}, newServiceError(ErrorKindInternal, err)
	}

	if err := os.Rename(stagingDir, roleDir); err != nil {
		if rollbackErr := os.Rename(backupDir, roleDir); rollbackErr != nil {
			return Spec{}, newServiceError(ErrorKindInternal, fmt.Errorf("apply update: %w; rollback failed: %v", err, rollbackErr))
		}
		return Spec{}, newServiceError(ErrorKindInternal, err)
	}
	_ = os.RemoveAll(backupDir)

	spec, err := loadRoleSpec(rolesRoot, normalized.RoleID)
	if err != nil {
		return Spec{}, newServiceError(ErrorKindInternal, err)
	}
	return spec, nil
}

func (s *Service) Delete(workspaceRoot, roleID string) error {
	workspaceRoot = strings.TrimSpace(workspaceRoot)
	roleID = strings.TrimSpace(roleID)
	if err := validateWorkspaceRoot(workspaceRoot); err != nil {
		return newServiceError(ErrorKindInvalidInput, err)
	}
	if err := validateRoleID(roleID); err != nil {
		return newServiceError(ErrorKindInvalidInput, err)
	}
	roleDir := filepath.Join(workspaceRoot, "roles", roleID)
	if _, err := os.Stat(roleDir); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return newServiceError(ErrorKindNotFound, err)
		}
		return newServiceError(ErrorKindInternal, err)
	}
	if err := os.RemoveAll(roleDir); err != nil {
		return newServiceError(ErrorKindInternal, err)
	}
	return nil
}

func prepareRoleStagingDir(rolesRoot string, in UpsertInput) (string, error) {
	if err := os.MkdirAll(rolesRoot, 0o755); err != nil {
		return "", newServiceError(ErrorKindInternal, err)
	}
	stagingDir, err := os.MkdirTemp(rolesRoot, ".role-staging-*")
	if err != nil {
		return "", newServiceError(ErrorKindInternal, err)
	}
	if err := writeRoleFilesToDir(stagingDir, in); err != nil {
		_ = os.RemoveAll(stagingDir)
		return "", newServiceError(ErrorKindInternal, err)
	}
	return stagingDir, nil
}

func writeRoleFilesToDir(roleDir string, in UpsertInput) error {
	raw, err := yaml.Marshal(map[string]any{
		"display_name": strings.TrimSpace(in.DisplayName),
		"description":  strings.TrimSpace(in.Description),
		"skills":       dedupeStrings(in.SkillNames),
	})
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(roleDir, "role.yaml"), raw, 0o644); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(roleDir, "prompt.md"), []byte(in.Prompt+"\n"), 0o644)
}

func dedupeStrings(values []string) []string {
	return normalizeSkillNames(values)
}

func validateWorkspaceRoot(workspaceRoot string) error {
	if strings.TrimSpace(workspaceRoot) == "" {
		return errors.New("workspace root is required")
	}
	return nil
}

func validateRoleID(roleID string) error {
	roleID = strings.TrimSpace(roleID)
	if roleID == "" {
		return errors.New("role_id is required")
	}
	if strings.HasPrefix(roleID, ".") {
		return fmt.Errorf("invalid role_id: %s", roleID)
	}
	if strings.Contains(roleID, "/") || strings.Contains(roleID, "\\") || strings.Contains(roleID, "..") {
		return fmt.Errorf("invalid role_id: %s", roleID)
	}
	return nil
}

func validateUpsertInput(in UpsertInput) error {
	if err := validateRoleID(in.RoleID); err != nil {
		return err
	}
	if strings.TrimSpace(in.Prompt) == "" {
		return errors.New("prompt is required")
	}
	return nil
}

func normalizeUpsertInput(in UpsertInput) UpsertInput {
	return UpsertInput{
		RoleID:      strings.TrimSpace(in.RoleID),
		DisplayName: strings.TrimSpace(in.DisplayName),
		Description: strings.TrimSpace(in.Description),
		Prompt:      strings.TrimSpace(in.Prompt),
		SkillNames:  dedupeStrings(in.SkillNames),
	}
}

func newServiceError(kind ErrorKind, cause error) error {
	return &ServiceError{kind: kind, cause: cause}
}
