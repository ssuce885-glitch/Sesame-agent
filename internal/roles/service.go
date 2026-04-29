package roles

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"go-agent/internal/config"
	"go-agent/internal/skills"
	"go-agent/internal/types"

	"gopkg.in/yaml.v3"
)

type UpsertInput struct {
	RoleID      string            `json:"role_id"`
	DisplayName string            `json:"display_name"`
	Description string            `json:"description"`
	Prompt      string            `json:"prompt"`
	SkillNames  []string          `json:"skills"`
	Policy      *RolePolicyConfig `json:"policy,omitempty"`
	Budget      *RoleBudgetConfig `json:"budget,omitempty"`
}

type AutomationCleanupService interface {
	List(context.Context, types.AutomationListFilter) ([]types.AutomationSpec, error)
	Delete(context.Context, string) (bool, error)
}

type Service struct {
	globalRoot        string
	automationCleanup AutomationCleanupService
}

func NewService() *Service { return &Service{} }

func NewServiceWithGlobalRoot(globalRoot string) *Service {
	return &Service{globalRoot: strings.TrimSpace(globalRoot)}
}

func (s *Service) SetAutomationCleanupService(cleanup AutomationCleanupService) {
	if s != nil {
		s.automationCleanup = cleanup
	}
}

var renameRoleDir = os.Rename

var (
	ErrInvalidInput = errors.New("invalid input")
	ErrNotFound     = errors.New("not found")
	ErrConflict     = errors.New("conflict")
	ErrInternal     = errors.New("internal error")
)

func IsInvalidInput(err error) bool { return errors.Is(err, ErrInvalidInput) }

func IsNotFound(err error) bool {
	return errors.Is(err, ErrNotFound) || errors.Is(err, os.ErrNotExist)
}

func IsConflict(err error) bool { return errors.Is(err, ErrConflict) }

func IsInternal(err error) bool {
	if err == nil {
		return false
	}
	return errors.Is(err, ErrInternal) || (!IsInvalidInput(err) && !IsNotFound(err) && !IsConflict(err))
}

func (s *Service) List(workspaceRoot string) (Catalog, error) {
	workspaceRoot = strings.TrimSpace(workspaceRoot)
	if err := validateWorkspaceRoot(workspaceRoot); err != nil {
		return Catalog{}, wrapRoleError(ErrInvalidInput, err)
	}
	catalog, err := LoadCatalog(workspaceRoot)
	if err != nil {
		return Catalog{}, wrapRoleError(ErrInternal, err)
	}
	return catalog, nil
}

func (s *Service) Get(workspaceRoot, roleID string) (Spec, error) {
	workspaceRoot = strings.TrimSpace(workspaceRoot)
	if err := validateWorkspaceRoot(workspaceRoot); err != nil {
		return Spec{}, wrapRoleError(ErrInvalidInput, err)
	}
	roleID, err := CanonicalRoleID(roleID)
	if err != nil {
		return Spec{}, wrapRoleError(ErrInvalidInput, err)
	}
	catalog, err := LoadCatalog(workspaceRoot)
	if err != nil {
		return Spec{}, wrapRoleError(ErrInternal, err)
	}
	if spec, ok := catalog.ByID[roleID]; ok {
		return spec, nil
	}
	if diagnostic, ok := catalogDiagnosticByRoleID(catalog, roleID); ok {
		return Spec{}, wrapRoleError(ErrConflict, fmt.Errorf("%s: %s", diagnostic.Path, diagnostic.Error))
	}
	return Spec{}, wrapRoleError(ErrNotFound, os.ErrNotExist)
}

func (s *Service) Create(workspaceRoot string, in UpsertInput) (Spec, error) {
	workspaceRoot = strings.TrimSpace(workspaceRoot)
	normalized, err := normalizeUpsertInput(in)
	if err != nil {
		return Spec{}, wrapRoleError(ErrInvalidInput, err)
	}
	if err := validateWorkspaceRoot(workspaceRoot); err != nil {
		return Spec{}, wrapRoleError(ErrInvalidInput, err)
	}
	if err := validateUpsertInput(normalized); err != nil {
		return Spec{}, wrapRoleError(ErrInvalidInput, err)
	}
	if err := s.validateSkillNames(workspaceRoot, normalized.SkillNames); err != nil {
		return Spec{}, wrapRoleError(ErrInvalidInput, err)
	}
	rolesRoot := filepath.Join(workspaceRoot, "roles")
	roleDir := filepath.Join(rolesRoot, normalized.RoleID)
	if _, err := os.Lstat(roleDir); err == nil {
		return Spec{}, wrapRoleError(ErrConflict, fmt.Errorf("role already exists: %s", normalized.RoleID))
	} else if !errors.Is(err, os.ErrNotExist) {
		return Spec{}, wrapRoleError(ErrInternal, err)
	}
	stagingDir, err := prepareRoleStagingDir(workspaceRoot, rolesRoot, normalized, 1)
	if err != nil {
		return Spec{}, err
	}
	defer func() { _ = os.RemoveAll(stagingDir) }()
	createdSpec := specFromUpsertInput(normalized, 1)
	if err := writeRoleSnapshotToDir(stagingDir, createdSpec); err != nil {
		return Spec{}, wrapRoleError(ErrInternal, err)
	}
	if err := renameRoleDir(stagingDir, roleDir); err != nil {
		if isCreateDestinationConflict(roleDir, err) {
			return Spec{}, wrapRoleError(ErrConflict, err)
		}
		return Spec{}, wrapRoleError(ErrInternal, err)
	}
	spec, err := loadRoleSpec(rolesRoot, normalized.RoleID)
	if err != nil {
		return Spec{}, wrapRoleError(ErrInternal, err)
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
		return Spec{}, wrapRoleError(ErrInvalidInput, err)
	}
	if err := validateWorkspaceRoot(workspaceRoot); err != nil {
		return Spec{}, wrapRoleError(ErrInvalidInput, err)
	}
	if err := validateUpsertInput(normalized); err != nil {
		return Spec{}, wrapRoleError(ErrInvalidInput, err)
	}
	if err := s.validateSkillNames(workspaceRoot, normalized.SkillNames); err != nil {
		return Spec{}, wrapRoleError(ErrInvalidInput, err)
	}
	rolesRoot := filepath.Join(workspaceRoot, "roles")
	roleDir := filepath.Join(rolesRoot, normalized.RoleID)
	if err := ensureConcreteRoleDir(roleDir); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			catalog, loadErr := LoadCatalog(workspaceRoot)
			if loadErr != nil {
				return Spec{}, wrapRoleError(ErrInternal, loadErr)
			}
			if diagnostic, ok := catalogDiagnosticByRoleID(catalog, normalized.RoleID); ok {
				return Spec{}, wrapRoleError(ErrConflict, fmt.Errorf("%s: %s", diagnostic.Path, diagnostic.Error))
			}
			return Spec{}, wrapRoleError(ErrNotFound, err)
		}
		return Spec{}, wrapRoleError(ErrInternal, err)
	}

	currentVersion := readRoleVersion(roleDir)
	nextVersion := currentVersion + 1
	stagingDir, err := prepareRoleStagingDir(workspaceRoot, rolesRoot, normalized, nextVersion)
	if err != nil {
		return Spec{}, err
	}
	defer func() { _ = os.RemoveAll(stagingDir) }()
	updatedSpec := specFromUpsertInput(normalized, nextVersion)
	if err := writeRoleSnapshotToDir(stagingDir, updatedSpec); err != nil {
		return Spec{}, wrapRoleError(ErrInternal, err)
	}

	if err := replaceRoleFilesFromStaging(workspaceRoot, roleDir, stagingDir); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Spec{}, wrapRoleError(ErrNotFound, err)
		}
		return Spec{}, wrapRoleError(ErrInternal, err)
	}

	spec, err := loadRoleSpec(rolesRoot, normalized.RoleID)
	if err != nil {
		return Spec{}, wrapRoleError(ErrInternal, err)
	}
	return spec, nil
}

func (s *Service) ListVersions(workspaceRoot, roleID string) ([]Spec, error) {
	workspaceRoot = strings.TrimSpace(workspaceRoot)
	if err := validateWorkspaceRoot(workspaceRoot); err != nil {
		return nil, wrapRoleError(ErrInvalidInput, err)
	}
	roleID, err := CanonicalRoleID(roleID)
	if err != nil {
		return nil, wrapRoleError(ErrInvalidInput, err)
	}
	roleDir := filepath.Join(workspaceRoot, "roles", roleID)
	if err := ensureConcreteRoleDir(roleDir); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, wrapRoleError(ErrNotFound, err)
		}
		return nil, wrapRoleError(ErrInternal, err)
	}
	versionsDir := filepath.Join(roleDir, ".role-versions")
	entries, err := os.ReadDir(versionsDir)
	if errors.Is(err, os.ErrNotExist) {
		spec, getErr := s.Get(workspaceRoot, roleID)
		if getErr != nil {
			return nil, getErr
		}
		return []Spec{spec}, nil
	}
	if err != nil {
		return nil, wrapRoleError(ErrInternal, err)
	}
	versions := make([]Spec, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}
		spec, loadErr := loadRoleVersionSnapshot(filepath.Join(versionsDir, entry.Name()))
		if loadErr != nil {
			return nil, wrapRoleError(ErrInternal, loadErr)
		}
		versions = append(versions, spec)
	}
	if len(versions) == 0 {
		spec, getErr := s.Get(workspaceRoot, roleID)
		if getErr != nil {
			return nil, getErr
		}
		return []Spec{spec}, nil
	}
	sort.Slice(versions, func(i, j int) bool {
		return versions[i].Version < versions[j].Version
	})
	return versions, nil
}

func (s *Service) Delete(workspaceRoot, roleID string) error {
	workspaceRoot = strings.TrimSpace(workspaceRoot)
	if err := validateWorkspaceRoot(workspaceRoot); err != nil {
		return wrapRoleError(ErrInvalidInput, err)
	}
	roleID, err := CanonicalRoleID(roleID)
	if err != nil {
		return wrapRoleError(ErrInvalidInput, err)
	}
	roleDir := filepath.Join(workspaceRoot, "roles", roleID)
	if _, err := os.Lstat(roleDir); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return wrapRoleError(ErrNotFound, err)
		}
		return wrapRoleError(ErrInternal, err)
	}
	if err := s.deleteOwnedAutomations(context.Background(), workspaceRoot, roleID); err != nil {
		return wrapRoleError(ErrInternal, err)
	}
	if err := os.RemoveAll(roleDir); err != nil {
		return wrapRoleError(ErrInternal, err)
	}
	return nil
}

func (s *Service) deleteOwnedAutomations(ctx context.Context, workspaceRoot, roleID string) error {
	if s == nil || s.automationCleanup == nil {
		return nil
	}
	specs, err := s.automationCleanup.List(ctx, types.AutomationListFilter{WorkspaceRoot: workspaceRoot})
	if err != nil {
		return err
	}
	wantOwner := "role:" + strings.TrimSpace(roleID)
	for _, spec := range specs {
		if types.NormalizeRoleAutomationOwner(spec.Owner) != wantOwner {
			continue
		}
		if _, err := s.automationCleanup.Delete(ctx, spec.ID); err != nil {
			return err
		}
	}
	return nil
}

func prepareRoleStagingDir(workspaceRoot, rolesRoot string, in UpsertInput, version int) (string, error) {
	if err := os.MkdirAll(rolesRoot, 0o755); err != nil {
		return "", wrapRoleError(ErrInternal, err)
	}
	scratchRoot, err := roleScratchRoot(workspaceRoot)
	if err != nil {
		return "", wrapRoleError(ErrInternal, err)
	}
	stagingDir, err := os.MkdirTemp(scratchRoot, ".role-staging-*")
	if err != nil {
		return "", wrapRoleError(ErrInternal, err)
	}
	if err := writeRoleFilesToDir(stagingDir, in, version); err != nil {
		_ = os.RemoveAll(stagingDir)
		return "", wrapRoleError(ErrInternal, err)
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

func writeRoleFilesToDir(roleDir string, in UpsertInput, version int) error {
	payload := map[string]any{
		"display_name": strings.TrimSpace(in.DisplayName),
		"description":  strings.TrimSpace(in.Description),
		"skills":       dedupeStrings(in.SkillNames),
		"version":      normalizeRoleVersion(version),
	}
	if in.Policy != nil {
		payload["policy"] = cloneRolePolicyConfig(in.Policy)
	}
	if in.Budget != nil {
		payload["budget"] = cloneRoleBudgetConfig(in.Budget)
	}
	raw, err := yaml.Marshal(payload)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(roleDir, "role.yaml"), raw, 0o644); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(roleDir, "prompt.md"), []byte(in.Prompt+"\n"), 0o644)
}

func writeRoleSnapshotToDir(roleDir string, spec Spec) error {
	snapshotDir := filepath.Join(roleDir, ".role-versions")
	if err := os.MkdirAll(snapshotDir, 0o755); err != nil {
		return err
	}
	raw, err := yaml.Marshal(roleSnapshot{
		RoleID:      spec.RoleID,
		DisplayName: spec.DisplayName,
		Description: spec.Description,
		Prompt:      spec.Prompt,
		SkillNames:  normalizeSkillNames(spec.SkillNames),
		Version:     normalizeRoleVersion(spec.Version),
		Policy:      cloneRolePolicyConfig(spec.Policy),
		Budget:      cloneRoleBudgetConfig(spec.Budget),
	})
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(snapshotDir, roleVersionSnapshotFileName(spec.Version)), raw, 0o644)
}

func roleVersionSnapshotFileName(version int) string {
	return fmt.Sprintf("%06d.yaml", normalizeRoleVersion(version))
}

func specFromUpsertInput(in UpsertInput, version int) Spec {
	return Spec{
		RoleID:      in.RoleID,
		DisplayName: strings.TrimSpace(in.DisplayName),
		Description: strings.TrimSpace(in.Description),
		Prompt:      strings.TrimSpace(in.Prompt),
		SkillNames:  normalizeSkillNames(in.SkillNames),
		Version:     normalizeRoleVersion(version),
		Policy:      cloneRolePolicyConfig(in.Policy),
		Budget:      cloneRoleBudgetConfig(in.Budget),
	}
}

func readRoleVersion(roleDir string) int {
	roleData, err := readConcreteRoleFile(filepath.Join(roleDir, "role.yaml"))
	if err != nil {
		return 1
	}
	var cfg roleConfig
	if err := yaml.Unmarshal(roleData, &cfg); err != nil {
		return 1
	}
	return normalizeRoleVersion(cfg.Version)
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

func wrapRoleError(kind error, cause error) error {
	if cause == nil {
		return kind
	}
	return fmt.Errorf("%w: %w", kind, cause)
}

func catalogDiagnosticByRoleID(catalog Catalog, roleID string) (Diagnostic, bool) {
	for _, diagnostic := range catalog.Diagnostics {
		if diagnostic.RoleID == roleID {
			return diagnostic, true
		}
	}
	return Diagnostic{}, false
}
