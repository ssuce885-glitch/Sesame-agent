package roles

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

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

var renameRoleDir = os.Rename

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
	workspaceRoot = strings.TrimSpace(workspaceRoot)
	if err := validateWorkspaceRoot(workspaceRoot); err != nil {
		return Catalog{}, newServiceError(ErrorKindInvalidInput, err)
	}
	catalog, err := LoadCatalog(workspaceRoot)
	if err != nil {
		return Catalog{}, newServiceError(ErrorKindInternal, err)
	}
	return catalog, nil
}

func (s *Service) Get(workspaceRoot, roleID string) (Spec, error) {
	workspaceRoot = strings.TrimSpace(workspaceRoot)
	if err := validateWorkspaceRoot(workspaceRoot); err != nil {
		return Spec{}, newServiceError(ErrorKindInvalidInput, err)
	}
	roleID, err := CanonicalRoleID(roleID)
	if err != nil {
		return Spec{}, newServiceError(ErrorKindInvalidInput, err)
	}
	catalog, err := LoadCatalog(workspaceRoot)
	if err != nil {
		return Spec{}, newServiceError(ErrorKindInternal, err)
	}
	if spec, ok := catalog.ByID[roleID]; ok {
		return spec, nil
	}
	if diagnostic, ok := catalogDiagnosticByRoleID(catalog, roleID); ok {
		return Spec{}, newServiceError(ErrorKindConflict, fmt.Errorf("%s: %s", diagnostic.Path, diagnostic.Error))
	}
	return Spec{}, newServiceError(ErrorKindNotFound, os.ErrNotExist)
}

func (s *Service) Create(workspaceRoot string, in UpsertInput) (Spec, error) {
	workspaceRoot = strings.TrimSpace(workspaceRoot)
	normalized, err := normalizeUpsertInput(in)
	if err != nil {
		return Spec{}, newServiceError(ErrorKindInvalidInput, err)
	}
	if err := validateWorkspaceRoot(workspaceRoot); err != nil {
		return Spec{}, newServiceError(ErrorKindInvalidInput, err)
	}
	if err := validateUpsertInput(normalized); err != nil {
		return Spec{}, newServiceError(ErrorKindInvalidInput, err)
	}
	rolesRoot := filepath.Join(workspaceRoot, "roles")
	roleDir := filepath.Join(rolesRoot, normalized.RoleID)
	if _, err := os.Lstat(roleDir); err == nil {
		return Spec{}, newServiceError(ErrorKindConflict, fmt.Errorf("role already exists: %s", normalized.RoleID))
	} else if !errors.Is(err, os.ErrNotExist) {
		return Spec{}, newServiceError(ErrorKindInternal, err)
	}
	stagingDir, err := prepareRoleStagingDir(rolesRoot, normalized)
	if err != nil {
		return Spec{}, err
	}
	defer func() { _ = os.RemoveAll(stagingDir) }()
	if err := renameRoleDir(stagingDir, roleDir); err != nil {
		if isCreateDestinationConflict(roleDir, err) {
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

func isCreateDestinationConflict(roleDir string, renameErr error) bool {
	if errors.Is(renameErr, os.ErrExist) {
		return true
	}
	_, err := os.Lstat(roleDir)
	return err == nil
}

func (s *Service) Update(workspaceRoot string, in UpsertInput) (Spec, error) {
	workspaceRoot = strings.TrimSpace(workspaceRoot)
	normalized, err := normalizeUpsertInput(in)
	if err != nil {
		return Spec{}, newServiceError(ErrorKindInvalidInput, err)
	}
	if err := validateWorkspaceRoot(workspaceRoot); err != nil {
		return Spec{}, newServiceError(ErrorKindInvalidInput, err)
	}
	if err := validateUpsertInput(normalized); err != nil {
		return Spec{}, newServiceError(ErrorKindInvalidInput, err)
	}
	rolesRoot := filepath.Join(workspaceRoot, "roles")
	roleDir := filepath.Join(rolesRoot, normalized.RoleID)
	if err := ensureConcreteRoleDir(roleDir); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			catalog, loadErr := LoadCatalog(workspaceRoot)
			if loadErr != nil {
				return Spec{}, newServiceError(ErrorKindInternal, loadErr)
			}
			if diagnostic, ok := catalogDiagnosticByRoleID(catalog, normalized.RoleID); ok {
				return Spec{}, newServiceError(ErrorKindConflict, fmt.Errorf("%s: %s", diagnostic.Path, diagnostic.Error))
			}
			return Spec{}, newServiceError(ErrorKindNotFound, err)
		}
		return Spec{}, newServiceError(ErrorKindInternal, err)
	}

	stagingDir, err := prepareRoleStagingDir(rolesRoot, normalized)
	if err != nil {
		return Spec{}, err
	}
	defer func() { _ = os.RemoveAll(stagingDir) }()

	if err := replaceRoleFilesFromStaging(roleDir, stagingDir); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Spec{}, newServiceError(ErrorKindNotFound, err)
		}
		return Spec{}, newServiceError(ErrorKindInternal, err)
	}

	spec, err := loadRoleSpec(rolesRoot, normalized.RoleID)
	if err != nil {
		return Spec{}, newServiceError(ErrorKindInternal, err)
	}
	return spec, nil
}

func (s *Service) Delete(workspaceRoot, roleID string) error {
	workspaceRoot = strings.TrimSpace(workspaceRoot)
	if err := validateWorkspaceRoot(workspaceRoot); err != nil {
		return newServiceError(ErrorKindInvalidInput, err)
	}
	roleID, err := CanonicalRoleID(roleID)
	if err != nil {
		return newServiceError(ErrorKindInvalidInput, err)
	}
	roleDir := filepath.Join(workspaceRoot, "roles", roleID)
	if _, err := os.Lstat(roleDir); err != nil {
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

type roleFileBackup struct {
	dst    string
	backup string
}

func replaceRoleFilesFromStaging(roleDir, stagingDir string) error {
	backupDir, err := os.MkdirTemp(roleDir, ".role-update-backup-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(backupDir) }()

	replaced := make([]roleFileBackup, 0, 2)

	for _, name := range []string{"role.yaml", "prompt.md"} {
		src := filepath.Join(stagingDir, name)
		dst := filepath.Join(roleDir, name)
		backup := filepath.Join(backupDir, name)

		if err := moveFileToBackup(dst, backup); err != nil {
			if rollbackErr := rollbackRoleFiles(replaced); rollbackErr != nil {
				return errors.Join(err, rollbackErr)
			}
			return err
		}

		if err := os.Rename(src, dst); err != nil {
			restoreErr := os.Rename(backup, dst)
			rollbackErr := rollbackRoleFiles(replaced)
			return errors.Join(err, restoreErr, rollbackErr)
		}

		replaced = append(replaced, roleFileBackup{
			dst:    dst,
			backup: backup,
		})
	}
	return nil
}

func moveFileToBackup(src, backup string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return fmt.Errorf("%s is a directory", src)
	}
	return os.Rename(src, backup)
}

func rollbackRoleFiles(replaced []roleFileBackup) error {
	var rollbackErr error
	for i := len(replaced) - 1; i >= 0; i-- {
		entry := replaced[i]
		if err := os.Remove(entry.dst); err != nil && !errors.Is(err, os.ErrNotExist) {
			rollbackErr = errors.Join(rollbackErr, err)
		}
		if err := os.Rename(entry.backup, entry.dst); err != nil {
			rollbackErr = errors.Join(rollbackErr, err)
		}
	}
	return rollbackErr
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

func ensureConcreteRoleDir(roleDir string) error {
	info, err := os.Lstat(roleDir)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("role directory %s is a symlink: %w", roleDir, os.ErrNotExist)
	}
	if !info.IsDir() {
		return fmt.Errorf("role path %s is not a directory", roleDir)
	}
	return nil
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

func newServiceError(kind ErrorKind, cause error) error {
	return &ServiceError{kind: kind, cause: cause}
}

func catalogDiagnosticByRoleID(catalog Catalog, roleID string) (Diagnostic, bool) {
	for _, diagnostic := range catalog.Diagnostics {
		if diagnostic.RoleID == roleID {
			return diagnostic, true
		}
	}
	return Diagnostic{}, false
}
