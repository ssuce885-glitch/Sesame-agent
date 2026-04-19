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

func (s *Service) List(workspaceRoot string) (Catalog, error) {
	return LoadCatalog(strings.TrimSpace(workspaceRoot))
}

func (s *Service) Get(workspaceRoot, roleID string) (Spec, error) {
	workspaceRoot = strings.TrimSpace(workspaceRoot)
	roleID = strings.TrimSpace(roleID)
	if err := validateWorkspaceRoot(workspaceRoot); err != nil {
		return Spec{}, err
	}
	if err := validateRoleID(roleID); err != nil {
		return Spec{}, err
	}
	return loadRoleSpec(filepath.Join(workspaceRoot, "roles"), roleID)
}

func (s *Service) Create(workspaceRoot string, in UpsertInput) (Spec, error) {
	workspaceRoot = strings.TrimSpace(workspaceRoot)
	roleID := strings.TrimSpace(in.RoleID)
	if err := validateWorkspaceRoot(workspaceRoot); err != nil {
		return Spec{}, err
	}
	if err := validateRoleID(roleID); err != nil {
		return Spec{}, err
	}
	if err := writeRoleFiles(workspaceRoot, roleID, in); err != nil {
		return Spec{}, err
	}
	return loadRoleSpec(filepath.Join(workspaceRoot, "roles"), roleID)
}

func (s *Service) Update(workspaceRoot string, in UpsertInput) (Spec, error) {
	workspaceRoot = strings.TrimSpace(workspaceRoot)
	roleID := strings.TrimSpace(in.RoleID)
	if err := validateWorkspaceRoot(workspaceRoot); err != nil {
		return Spec{}, err
	}
	if err := validateRoleID(roleID); err != nil {
		return Spec{}, err
	}
	roleDir := filepath.Join(workspaceRoot, "roles", roleID)
	if _, err := os.Stat(roleDir); err != nil {
		return Spec{}, err
	}
	if err := writeRoleFiles(workspaceRoot, roleID, in); err != nil {
		return Spec{}, err
	}
	return loadRoleSpec(filepath.Join(workspaceRoot, "roles"), roleID)
}

func (s *Service) Delete(workspaceRoot, roleID string) error {
	workspaceRoot = strings.TrimSpace(workspaceRoot)
	roleID = strings.TrimSpace(roleID)
	if err := validateWorkspaceRoot(workspaceRoot); err != nil {
		return err
	}
	if err := validateRoleID(roleID); err != nil {
		return err
	}
	roleDir := filepath.Join(workspaceRoot, "roles", roleID)
	if _, err := os.Stat(roleDir); err != nil {
		return err
	}
	return os.RemoveAll(roleDir)
}

func writeRoleFiles(workspaceRoot, roleID string, in UpsertInput) error {
	roleDir := filepath.Join(workspaceRoot, "roles", roleID)
	if err := os.MkdirAll(roleDir, 0o755); err != nil {
		return err
	}
	prompt := strings.TrimSpace(in.Prompt)
	if prompt == "" {
		return errors.New("prompt is required")
	}
	if err := os.WriteFile(filepath.Join(roleDir, "prompt.md"), []byte(prompt+"\n"), 0o644); err != nil {
		return err
	}
	raw, err := yaml.Marshal(map[string]any{
		"display_name": strings.TrimSpace(in.DisplayName),
		"description":  strings.TrimSpace(in.Description),
		"skills":       dedupeStrings(in.SkillNames),
	})
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(roleDir, "role.yaml"), raw, 0o644)
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
	if strings.Contains(roleID, "/") || strings.Contains(roleID, "\\") || strings.Contains(roleID, "..") {
		return fmt.Errorf("invalid role_id: %s", roleID)
	}
	return nil
}
