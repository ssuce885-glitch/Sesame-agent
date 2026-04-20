package roles

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go-agent/internal/config"
	"go-agent/internal/skills"

	"gopkg.in/yaml.v3"
)

type UpsertInput struct {
	RoleID      string   `json:"role_id"`
	DisplayName string   `json:"display_name"`
	Description string   `json:"description"`
	Prompt      string   `json:"prompt"`
	SkillNames  []string `json:"skills"`
}

type Service struct {
	globalRoot string
}

func NewService() *Service { return &Service{} }

func NewServiceWithGlobalRoot(globalRoot string) *Service {
	return &Service{globalRoot: strings.TrimSpace(globalRoot)}
}

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
	if err := s.validateSkillNames(workspaceRoot, normalized.SkillNames); err != nil {
		return Spec{}, newServiceError(ErrorKindInvalidInput, err)
	}
	rolesRoot := filepath.Join(workspaceRoot, "roles")
	roleDir := filepath.Join(rolesRoot, normalized.RoleID)
	if _, err := os.Lstat(roleDir); err == nil {
		return Spec{}, newServiceError(ErrorKindConflict, fmt.Errorf("role already exists: %s", normalized.RoleID))
	} else if !errors.Is(err, os.ErrNotExist) {
		return Spec{}, newServiceError(ErrorKindInternal, err)
	}
	stagingDir, err := prepareRoleStagingDir(workspaceRoot, rolesRoot, normalized)
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
	if err := s.validateSkillNames(workspaceRoot, normalized.SkillNames); err != nil {
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

	stagingDir, err := prepareRoleStagingDir(workspaceRoot, rolesRoot, normalized)
	if err != nil {
		return Spec{}, err
	}
	defer func() { _ = os.RemoveAll(stagingDir) }()

	if err := replaceRoleFilesFromStaging(workspaceRoot, roleDir, stagingDir); err != nil {
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

func prepareRoleStagingDir(workspaceRoot, rolesRoot string, in UpsertInput) (string, error) {
	if err := os.MkdirAll(rolesRoot, 0o755); err != nil {
		return "", newServiceError(ErrorKindInternal, err)
	}
	scratchRoot, err := roleScratchRoot(workspaceRoot)
	if err != nil {
		return "", newServiceError(ErrorKindInternal, err)
	}
	stagingDir, err := os.MkdirTemp(scratchRoot, ".role-staging-*")
	if err != nil {
		return "", newServiceError(ErrorKindInternal, err)
	}
	if err := writeRoleFilesToDir(stagingDir, in); err != nil {
		_ = os.RemoveAll(stagingDir)
		return "", newServiceError(ErrorKindInternal, err)
	}
	return stagingDir, nil
}

func replaceRoleFilesFromStaging(workspaceRoot, roleDir, stagingDir string) error {
	if err := validateManagedRoleFiles(roleDir); err != nil {
		return err
	}
	if err := copyPreservedRoleEntries(roleDir, stagingDir); err != nil {
		return err
	}
	scratchRoot, err := roleScratchRoot(workspaceRoot)
	if err != nil {
		return err
	}
	backupRoot, err := os.MkdirTemp(scratchRoot, ".role-update-backup-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(backupRoot) }()
	backupDir := filepath.Join(backupRoot, filepath.Base(roleDir))

	if err := renameRoleDir(roleDir, backupDir); err != nil {
		return err
	}
	if err := renameRoleDir(stagingDir, roleDir); err != nil {
		restoreErr := renameRoleDir(backupDir, roleDir)
		return errors.Join(err, restoreErr)
	}
	return nil
}

func validateManagedRoleFiles(roleDir string) error {
	for _, name := range []string{"role.yaml", "prompt.md"} {
		path := filepath.Join(roleDir, name)
		info, err := os.Stat(path)
		if err != nil {
			return err
		}
		if info.IsDir() {
			return fmt.Errorf("%s is a directory", path)
		}
	}
	return nil
}

func copyPreservedRoleEntries(srcDir, dstDir string) error {
	entries, err := os.ReadDir(srcDir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		name := entry.Name()
		if name == "role.yaml" || name == "prompt.md" {
			continue
		}
		if err := copyRoleEntry(filepath.Join(srcDir, name), filepath.Join(dstDir, name)); err != nil {
			return err
		}
	}
	return nil
}

func copyRoleEntry(src, dst string) error {
	info, err := os.Lstat(src)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%s is a symlink", src)
	}
	if info.IsDir() {
		if err := os.MkdirAll(dst, info.Mode().Perm()); err != nil {
			return err
		}
		entries, err := os.ReadDir(src)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if err := copyRoleEntry(filepath.Join(src, entry.Name()), filepath.Join(dst, entry.Name())); err != nil {
				return err
			}
		}
		return nil
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%s is not a regular file", src)
	}
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dst, data, info.Mode().Perm())
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

func (s *Service) validateSkillNames(workspaceRoot string, skillNames []string) error {
	if len(skillNames) == 0 {
		return nil
	}
	globalRoot, err := s.resolveGlobalRoot(workspaceRoot)
	if err != nil {
		return err
	}
	catalog, err := skills.LoadCatalog(globalRoot, workspaceRoot)
	if err != nil {
		return err
	}
	available := make(map[string]struct{}, len(catalog.Skills))
	for _, name := range catalog.SkillNames() {
		available[strings.ToLower(strings.TrimSpace(name))] = struct{}{}
	}
	var unknown []string
	for _, name := range dedupeStrings(skillNames) {
		if _, ok := available[strings.ToLower(strings.TrimSpace(name))]; ok {
			continue
		}
		unknown = append(unknown, name)
	}
	if len(unknown) > 0 {
		return fmt.Errorf("unknown skills: %s", strings.Join(unknown, ", "))
	}
	return nil
}

func (s *Service) resolveGlobalRoot(workspaceRoot string) (string, error) {
	if trimmed := strings.TrimSpace(s.globalRoot); trimmed != "" {
		return trimmed, nil
	}
	paths, err := config.ResolvePaths(workspaceRoot, "")
	if err != nil {
		return "", err
	}
	return paths.GlobalRoot, nil
}

func roleScratchRoot(workspaceRoot string) (string, error) {
	root := filepath.Join(strings.TrimSpace(workspaceRoot), config.DirName, "tmp", "roles")
	if err := os.MkdirAll(root, 0o755); err != nil {
		return "", err
	}
	return root, nil
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
