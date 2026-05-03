package roles

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

var (
	ErrInvalidRole  = errors.New("invalid role")
	ErrRoleExists   = errors.New("role already exists")
	ErrRoleNotFound = errors.New("role not found")
)

var roleIDPattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,63}$`)

// RoleSpec is a parsed role definition from disk.
type RoleSpec struct {
	ID                string   `json:"id"`
	Name              string   `json:"name"`
	Description       string   `json:"description"`
	SystemPrompt      string   `json:"system_prompt"`
	PermissionProfile string   `json:"permission_profile"`
	Model             string   `json:"model"`
	MaxToolCalls      int      `json:"max_tool_calls"`
	MaxRuntime        int      `json:"max_runtime"` // seconds
	MaxContextTokens  int      `json:"max_context_tokens,omitempty"`
	SkillNames        []string `json:"skill_names,omitempty"`
	DeniedTools       []string `json:"denied_tools,omitempty"`
	AllowedTools      []string `json:"allowed_tools,omitempty"`
	DeniedPaths       []string `json:"denied_paths,omitempty"`
	AllowedPaths      []string `json:"allowed_paths,omitempty"`
	CanDelegate       bool     `json:"can_delegate"`
	AutomationOwners  []string `json:"automation_ownership,omitempty"`
	Version           int      `json:"version,omitempty"`
}

// SaveInput is the full editable role payload accepted by create/update paths.
type SaveInput struct {
	ID                string   `json:"id"`
	Name              string   `json:"name"`
	Description       string   `json:"description"`
	SystemPrompt      string   `json:"system_prompt"`
	PermissionProfile string   `json:"permission_profile"`
	Model             string   `json:"model"`
	MaxToolCalls      int      `json:"max_tool_calls"`
	MaxRuntime        int      `json:"max_runtime"`
	MaxContextTokens  int      `json:"max_context_tokens,omitempty"`
	SkillNames        []string `json:"skill_names,omitempty"`
	DeniedTools       []string `json:"denied_tools,omitempty"`
	AllowedTools      []string `json:"allowed_tools,omitempty"`
	DeniedPaths       []string `json:"denied_paths,omitempty"`
	AllowedPaths      []string `json:"allowed_paths,omitempty"`
	CanDelegate       bool     `json:"can_delegate"`
	AutomationOwners  []string `json:"automation_ownership,omitempty"`
}

// Service reads role specs from the filesystem.
type Service struct {
	workspaceRoot string
}

func NewService(workspaceRoot string) *Service { return &Service{workspaceRoot: workspaceRoot} }

// List returns all installed roles.
func (s *Service) List() ([]RoleSpec, error) {
	rolesDir := filepath.Join(s.workspaceRoot, "roles")
	entries, err := os.ReadDir(rolesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read roles dir: %w", err)
	}

	var specs []RoleSpec
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		spec, err := s.readRole(entry.Name())
		if err != nil {
			return nil, err
		}
		specs = append(specs, spec)
	}
	sort.Slice(specs, func(i, j int) bool { return specs[i].ID < specs[j].ID })
	return specs, nil
}

// Get returns a single role by ID.
func (s *Service) Get(id string) (RoleSpec, bool, error) {
	id = strings.TrimSpace(id)
	if id == "" || !isSafeRoleID(id) {
		return RoleSpec{}, false, nil
	}
	spec, err := s.readRole(id)
	if err != nil {
		if os.IsNotExist(err) {
			return RoleSpec{}, false, nil
		}
		return RoleSpec{}, false, err
	}
	return spec, true, nil
}

// Create writes a new role directory under roles/<id>/.
func (s *Service) Create(ctx context.Context, input SaveInput) (RoleSpec, error) {
	input = normalizeInput(input)
	if err := validateInput(input); err != nil {
		return RoleSpec{}, err
	}
	roleDir := s.roleDir(input.ID)
	if _, err := os.Stat(roleDir); err == nil {
		return RoleSpec{}, fmt.Errorf("%w: %s", ErrRoleExists, input.ID)
	} else if !os.IsNotExist(err) {
		return RoleSpec{}, fmt.Errorf("stat role: %w", err)
	}
	if err := s.writeRole(ctx, input, 1); err != nil {
		return RoleSpec{}, err
	}
	spec, _, err := s.Get(input.ID)
	return spec, err
}

// Update replaces an existing role definition while preserving role assets.
func (s *Service) Update(ctx context.Context, id string, input SaveInput) (RoleSpec, error) {
	id = strings.TrimSpace(id)
	if input.ID == "" {
		input.ID = id
	}
	input.ID = strings.TrimSpace(input.ID)
	if id == "" || input.ID != id {
		return RoleSpec{}, fmt.Errorf("%w: role id mismatch", ErrInvalidRole)
	}
	input = normalizeInput(input)
	if err := validateInput(input); err != nil {
		return RoleSpec{}, err
	}
	current, ok, err := s.Get(id)
	if err != nil {
		return RoleSpec{}, err
	}
	if !ok {
		return RoleSpec{}, fmt.Errorf("%w: %s", ErrRoleNotFound, id)
	}
	version := current.Version + 1
	if version <= 1 {
		version = nextSnapshotVersion(filepath.Join(s.roleDir(id), ".role-versions"))
	}
	if err := s.writeRole(ctx, input, version); err != nil {
		return RoleSpec{}, err
	}
	spec, _, err := s.Get(input.ID)
	return spec, err
}

// InstallRoleFromPath copies a role directory into the workspace's roles/ dir.
func (s *Service) InstallRoleFromPath(ctx context.Context, sourcePath string) error {
	sourcePath = filepath.Clean(sourcePath)
	info, err := os.Stat(sourcePath)
	if err != nil {
		return fmt.Errorf("stat source role: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("source role path is not a directory: %s", sourcePath)
	}
	roleID := filepath.Base(sourcePath)
	if roleID == "." || roleID == string(filepath.Separator) {
		return fmt.Errorf("invalid source role path: %s", sourcePath)
	}
	dest := filepath.Join(s.workspaceRoot, "roles", roleID)
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return fmt.Errorf("create roles dir: %w", err)
	}
	return copyDir(ctx, sourcePath, dest)
}

func (s *Service) roleDir(id string) string {
	return filepath.Join(s.workspaceRoot, "roles", id)
}

func (s *Service) readRole(id string) (RoleSpec, error) {
	roleDir := s.roleDir(id)
	metaPath := filepath.Join(roleDir, "role.yaml")
	promptPath := filepath.Join(roleDir, "prompt.md")

	rawMeta, err := os.ReadFile(metaPath)
	if err != nil {
		return RoleSpec{}, err
	}
	rawPrompt, err := os.ReadFile(promptPath)
	if err != nil {
		return RoleSpec{}, err
	}

	var meta roleYAML
	if err := yaml.Unmarshal(rawMeta, &meta); err != nil {
		return RoleSpec{}, fmt.Errorf("parse %s: %w", metaPath, err)
	}
	name := firstNonEmpty(meta.Name, meta.DisplayName, id)
	model := firstNonEmpty(meta.Model, meta.Policy.Model)
	permissionProfile := firstNonEmpty(meta.PermissionProfile, meta.Policy.PermissionProfile)
	skillNames := append(append([]string(nil), meta.SkillNames...), meta.Skills...)
	return RoleSpec{
		ID:                id,
		Name:              name,
		Description:       strings.TrimSpace(meta.Description),
		SystemPrompt:      string(rawPrompt),
		PermissionProfile: permissionProfile,
		Model:             model,
		MaxToolCalls:      meta.Budget.MaxToolCalls,
		MaxRuntime:        meta.Budget.MaxRuntime.Seconds,
		MaxContextTokens:  meta.Budget.MaxContextTokens,
		SkillNames:        cleanStringList(skillNames),
		DeniedTools:       cleanStringList(meta.DeniedTools),
		AllowedTools:      cleanStringList(meta.AllowedTools),
		DeniedPaths:       cleanStringList(meta.DeniedPaths),
		AllowedPaths:      cleanStringList(meta.AllowedPaths),
		CanDelegate:       meta.Policy.CanDelegate,
		AutomationOwners:  cleanStringList(meta.Policy.AutomationOwners),
		Version:           meta.Version,
	}, nil
}

func (s *Service) writeRole(ctx context.Context, input SaveInput, version int) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	roleDir := s.roleDir(input.ID)
	if err := os.MkdirAll(roleDir, 0o755); err != nil {
		return fmt.Errorf("create role dir: %w", err)
	}
	meta := roleYAML{
		RoleID:      input.ID,
		DisplayName: input.Name,
		Description: input.Description,
		Version:     version,
		Policy: rolePolicyYAML{
			Model:             input.Model,
			PermissionProfile: input.PermissionProfile,
			CanDelegate:       input.CanDelegate,
			AutomationOwners:  input.AutomationOwners,
		},
		Budget: roleBudgetYAML{
			MaxToolCalls:     input.MaxToolCalls,
			MaxRuntime:       durationSeconds{Seconds: input.MaxRuntime},
			MaxContextTokens: input.MaxContextTokens,
		},
		Skills:       input.SkillNames,
		DeniedTools:  input.DeniedTools,
		AllowedTools: input.AllowedTools,
		DeniedPaths:  input.DeniedPaths,
		AllowedPaths: input.AllowedPaths,
	}
	metaRaw, err := yaml.Marshal(meta)
	if err != nil {
		return fmt.Errorf("marshal role yaml: %w", err)
	}
	if err := writeFileAtomic(filepath.Join(roleDir, "role.yaml"), metaRaw, 0o644); err != nil {
		return fmt.Errorf("write role yaml: %w", err)
	}
	if err := writeFileAtomic(filepath.Join(roleDir, "prompt.md"), []byte(input.SystemPrompt), 0o644); err != nil {
		return fmt.Errorf("write prompt: %w", err)
	}
	if err := s.writeSnapshot(input, meta, version); err != nil {
		return err
	}
	return nil
}

func (s *Service) writeSnapshot(input SaveInput, meta roleYAML, version int) error {
	snapshotDir := filepath.Join(s.roleDir(input.ID), ".role-versions")
	if err := os.MkdirAll(snapshotDir, 0o755); err != nil {
		return fmt.Errorf("create role snapshot dir: %w", err)
	}
	snapshot := roleSnapshotYAML{
		RoleID:       input.ID,
		DisplayName:  input.Name,
		Description:  input.Description,
		Prompt:       input.SystemPrompt,
		Skills:       input.SkillNames,
		Version:      version,
		Policy:       meta.Policy,
		Budget:       meta.Budget,
		DeniedTools:  meta.DeniedTools,
		AllowedTools: meta.AllowedTools,
		DeniedPaths:  meta.DeniedPaths,
		AllowedPaths: meta.AllowedPaths,
	}
	raw, err := yaml.Marshal(snapshot)
	if err != nil {
		return fmt.Errorf("marshal role snapshot: %w", err)
	}
	path := filepath.Join(snapshotDir, fmt.Sprintf("%06d.yaml", version))
	if err := writeFileAtomic(path, raw, 0o644); err != nil {
		return fmt.Errorf("write role snapshot: %w", err)
	}
	return nil
}

type roleYAML struct {
	RoleID            string         `yaml:"role_id,omitempty"`
	Name              string         `yaml:"name,omitempty"`
	DisplayName       string         `yaml:"display_name,omitempty"`
	Description       string         `yaml:"description,omitempty"`
	Version           int            `yaml:"version,omitempty"`
	Model             string         `yaml:"model,omitempty"`
	PermissionProfile string         `yaml:"permission_profile,omitempty"`
	Policy            rolePolicyYAML `yaml:"policy,omitempty"`
	Budget            roleBudgetYAML `yaml:"budget,omitempty"`
	SkillNames        []string       `yaml:"skill_names,omitempty"`
	Skills            []string       `yaml:"skills,omitempty"`
	DeniedTools       []string       `yaml:"denied_tools,omitempty"`
	AllowedTools      []string       `yaml:"allowed_tools,omitempty"`
	DeniedPaths       []string       `yaml:"denied_paths,omitempty"`
	AllowedPaths      []string       `yaml:"allowed_paths,omitempty"`
}

type rolePolicyYAML struct {
	Model             string   `yaml:"model,omitempty"`
	PermissionProfile string   `yaml:"permission_profile,omitempty"`
	CanDelegate       bool     `yaml:"can_delegate,omitempty"`
	AutomationOwners  []string `yaml:"automation_ownership,omitempty"`
}

type roleBudgetYAML struct {
	MaxToolCalls     int             `yaml:"max_tool_calls,omitempty"`
	MaxRuntime       durationSeconds `yaml:"max_runtime,omitempty"`
	MaxContextTokens int             `yaml:"max_context_tokens,omitempty"`
}

type roleSnapshotYAML struct {
	RoleID       string         `yaml:"role_id"`
	DisplayName  string         `yaml:"display_name"`
	Description  string         `yaml:"description,omitempty"`
	Prompt       string         `yaml:"prompt"`
	Skills       []string       `yaml:"skills,omitempty"`
	Version      int            `yaml:"version"`
	Policy       rolePolicyYAML `yaml:"policy,omitempty"`
	Budget       roleBudgetYAML `yaml:"budget,omitempty"`
	DeniedTools  []string       `yaml:"denied_tools,omitempty"`
	AllowedTools []string       `yaml:"allowed_tools,omitempty"`
	DeniedPaths  []string       `yaml:"denied_paths,omitempty"`
	AllowedPaths []string       `yaml:"allowed_paths,omitempty"`
}

type durationSeconds struct {
	Seconds int
}

func (d *durationSeconds) UnmarshalYAML(node *yaml.Node) error {
	if node == nil || node.Value == "" {
		return nil
	}
	if node.Tag == "!!int" {
		v, err := strconv.Atoi(node.Value)
		if err != nil {
			return err
		}
		d.Seconds = v
		return nil
	}
	if v, err := strconv.Atoi(node.Value); err == nil {
		d.Seconds = v
		return nil
	}
	parsed, err := time.ParseDuration(node.Value)
	if err != nil {
		return err
	}
	d.Seconds = int(parsed.Seconds())
	return nil
}

func (d durationSeconds) MarshalYAML() (any, error) {
	if d.Seconds == 0 {
		return nil, nil
	}
	return d.Seconds, nil
}

func (d durationSeconds) IsZero() bool {
	return d.Seconds == 0
}

func (p rolePolicyYAML) IsZero() bool {
	return strings.TrimSpace(p.Model) == "" && strings.TrimSpace(p.PermissionProfile) == "" && !p.CanDelegate && len(p.AutomationOwners) == 0
}

func (b roleBudgetYAML) IsZero() bool {
	return b.MaxToolCalls == 0 && b.MaxRuntime.Seconds == 0 && b.MaxContextTokens == 0
}

func normalizeInput(input SaveInput) SaveInput {
	input.ID = strings.TrimSpace(input.ID)
	input.Name = strings.TrimSpace(input.Name)
	input.Description = strings.TrimSpace(input.Description)
	input.SystemPrompt = strings.TrimSpace(input.SystemPrompt)
	input.PermissionProfile = strings.TrimSpace(input.PermissionProfile)
	input.Model = strings.TrimSpace(input.Model)
	input.AutomationOwners = cleanStringList(input.AutomationOwners)
	input.SkillNames = cleanStringList(input.SkillNames)
	input.DeniedTools = cleanStringList(input.DeniedTools)
	input.AllowedTools = cleanStringList(input.AllowedTools)
	input.DeniedPaths = cleanStringList(input.DeniedPaths)
	input.AllowedPaths = cleanStringList(input.AllowedPaths)
	return input
}

func validateInput(input SaveInput) error {
	if !isSafeRoleID(input.ID) {
		return fmt.Errorf("%w: role id must match %s", ErrInvalidRole, roleIDPattern.String())
	}
	if input.Name == "" {
		return fmt.Errorf("%w: name is required", ErrInvalidRole)
	}
	if input.SystemPrompt == "" {
		return fmt.Errorf("%w: system_prompt is required", ErrInvalidRole)
	}
	if input.MaxToolCalls < 0 {
		return fmt.Errorf("%w: max_tool_calls must be non-negative", ErrInvalidRole)
	}
	if input.MaxRuntime < 0 {
		return fmt.Errorf("%w: max_runtime must be non-negative", ErrInvalidRole)
	}
	if input.MaxContextTokens < 0 {
		return fmt.Errorf("%w: max_context_tokens must be non-negative", ErrInvalidRole)
	}
	for _, list := range [][]string{input.SkillNames, input.DeniedTools, input.AllowedTools, input.DeniedPaths, input.AllowedPaths, input.AutomationOwners} {
		for _, value := range list {
			if strings.ContainsAny(value, "\x00\r\n") {
				return fmt.Errorf("%w: list values must be single-line strings", ErrInvalidRole)
			}
		}
	}
	return nil
}

func isSafeRoleID(id string) bool {
	id = strings.TrimSpace(id)
	return roleIDPattern.MatchString(id) && !strings.Contains(id, string(filepath.Separator))
}

func nextSnapshotVersion(snapshotDir string) int {
	entries, err := os.ReadDir(snapshotDir)
	if err != nil {
		return 1
	}
	maxVersion := 0
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
		version, err := strconv.Atoi(name)
		if err != nil {
			continue
		}
		if version > maxVersion {
			maxVersion = version
		}
	}
	return maxVersion + 1
}

func writeFileAtomic(path string, content []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }()
	if _, err := tmp.Write(content); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

func copyDir(ctx context.Context, source, dest string) error {
	if err := os.RemoveAll(dest); err != nil {
		return fmt.Errorf("remove existing role: %w", err)
	}
	return filepath.WalkDir(source, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dest, rel)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		return copyFile(path, target, info.Mode())
	})
}

func copyFile(source, dest string, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func cleanStringList(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
