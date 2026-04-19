package roles

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

type Spec struct {
	RoleID      string
	DisplayName string
	Description string
	Prompt      string
	SkillNames  []string
}

type Catalog struct {
	Roles []Spec
	ByID  map[string]Spec
}

type roleConfig struct {
	DisplayName string   `yaml:"display_name"`
	Description string   `yaml:"description"`
	Skills      []string `yaml:"skills"`
}

func LoadCatalog(workspaceRoot string) (Catalog, error) {
	root := filepath.Join(strings.TrimSpace(workspaceRoot), "roles")
	entries, err := os.ReadDir(root)
	if os.IsNotExist(err) {
		return Catalog{ByID: map[string]Spec{}}, nil
	}
	if err != nil {
		return Catalog{}, err
	}

	out := Catalog{ByID: map[string]Spec{}}
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		spec, err := loadRoleSpec(root, entry.Name())
		if err != nil {
			return Catalog{}, err
		}
		out.Roles = append(out.Roles, spec)
		out.ByID[spec.RoleID] = spec
	}

	sort.Slice(out.Roles, func(i, j int) bool {
		return out.Roles[i].RoleID < out.Roles[j].RoleID
	})
	return out, nil
}

func RenderRegistrySummary(catalog Catalog) string {
	if len(catalog.Roles) == 0 {
		return ""
	}
	lines := []string{"# Installed Specialist Roles"}
	for _, role := range catalog.Roles {
		line := fmt.Sprintf("- %s: %s", role.RoleID, firstNonEmpty(role.Description, role.DisplayName))
		if len(role.SkillNames) > 0 {
			line += ". Skills: " + strings.Join(role.SkillNames, ", ")
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func loadRoleSpec(root, roleID string) (Spec, error) {
	rolePath := filepath.Join(root, roleID)
	roleData, err := os.ReadFile(filepath.Join(rolePath, "role.yaml"))
	if err != nil {
		return Spec{}, err
	}
	promptData, err := os.ReadFile(filepath.Join(rolePath, "prompt.md"))
	if err != nil {
		return Spec{}, err
	}

	var cfg roleConfig
	if err := yaml.Unmarshal(roleData, &cfg); err != nil {
		return Spec{}, err
	}

	return Spec{
		RoleID:      strings.TrimSpace(roleID),
		DisplayName: strings.TrimSpace(cfg.DisplayName),
		Description: strings.TrimSpace(cfg.Description),
		Prompt:      strings.TrimSpace(string(promptData)),
		SkillNames:  normalizeSkillNames(cfg.Skills),
	}, nil
}

func normalizeSkillNames(skills []string) []string {
	if len(skills) == 0 {
		return nil
	}
	out := make([]string, 0, len(skills))
	for _, skill := range skills {
		if trimmed := strings.TrimSpace(skill); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}
