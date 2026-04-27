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
	Version     int
}

type Catalog struct {
	Roles       []Spec
	ByID        map[string]Spec
	Diagnostics []Diagnostic
}

type roleConfig struct {
	DisplayName string   `yaml:"display_name"`
	Description string   `yaml:"description"`
	Skills      []string `yaml:"skills"`
	Version     int      `yaml:"version"`
}

type roleSnapshot struct {
	RoleID      string   `yaml:"role_id"`
	DisplayName string   `yaml:"display_name"`
	Description string   `yaml:"description"`
	Prompt      string   `yaml:"prompt"`
	SkillNames  []string `yaml:"skills"`
	Version     int      `yaml:"version"`
}

func LoadCatalog(workspaceRoot string) (Catalog, error) {
	workspaceRoot = strings.TrimSpace(workspaceRoot)
	if workspaceRoot == "" {
		return Catalog{ByID: map[string]Spec{}}, nil
	}

	root := filepath.Join(workspaceRoot, "roles")
	entries, err := os.ReadDir(root)
	if os.IsNotExist(err) {
		return Catalog{ByID: map[string]Spec{}}, nil
	}
	if err != nil {
		return Catalog{}, err
	}

	out := Catalog{ByID: map[string]Spec{}}
	for _, entry := range entries {
		if shouldIgnoreInternalRoleDir(entry.Name()) {
			continue
		}
		roleID, err := CanonicalRoleID(entry.Name())
		if err != nil {
			out.Diagnostics = append(out.Diagnostics, Diagnostic{
				RoleID: entry.Name(),
				Path:   filepath.Join(root, entry.Name()),
				Error:  err.Error(),
			})
			continue
		}
		spec, err := loadRoleSpec(root, roleID)
		if err != nil {
			out.Diagnostics = append(out.Diagnostics, Diagnostic{
				RoleID: roleID,
				Path:   filepath.Join(root, roleID),
				Error:  err.Error(),
			})
			continue
		}
		out.Roles = append(out.Roles, spec)
		out.ByID[spec.RoleID] = spec
	}

	sort.Slice(out.Roles, func(i, j int) bool {
		return out.Roles[i].RoleID < out.Roles[j].RoleID
	})
	sort.Slice(out.Diagnostics, func(i, j int) bool {
		if out.Diagnostics[i].RoleID != out.Diagnostics[j].RoleID {
			return out.Diagnostics[i].RoleID < out.Diagnostics[j].RoleID
		}
		if out.Diagnostics[i].Path != out.Diagnostics[j].Path {
			return out.Diagnostics[i].Path < out.Diagnostics[j].Path
		}
		return out.Diagnostics[i].Error < out.Diagnostics[j].Error
	})
	return out, nil
}

func RenderRegistrySummary(catalog Catalog) string {
	lines := []string{
		"# Installed Specialist Roles",
		"- Source: workspace roles/ directory",
	}
	if len(catalog.Roles) == 0 {
		lines = append(lines, "- Installed specialist roles: none")
	} else {
		for _, role := range catalog.Roles {
			line := fmt.Sprintf("- %s: %s", role.RoleID, firstNonEmpty(role.Description, role.DisplayName, role.RoleID))
			if len(role.SkillNames) > 0 {
				line += ". Skills: " + strings.Join(role.SkillNames, ", ")
			}
			lines = append(lines, line)
		}
	}
	if len(catalog.Diagnostics) > 0 {
		lines = append(lines, "# Invalid Specialist Roles")
		for _, diagnostic := range catalog.Diagnostics {
			lines = append(lines, fmt.Sprintf("- %s: invalid (%s)", diagnostic.RoleID, diagnostic.Error))
		}
	}
	return strings.Join(lines, "\n")
}

func loadRoleSpec(root, roleID string) (Spec, error) {
	roleID, err := CanonicalRoleID(roleID)
	if err != nil {
		return Spec{}, err
	}
	rolePath := filepath.Join(root, roleID)
	if err := ensureConcreteRoleDir(rolePath); err != nil {
		return Spec{}, err
	}
	roleData, err := readConcreteRoleFile(filepath.Join(rolePath, "role.yaml"))
	if err != nil {
		return Spec{}, err
	}
	promptData, err := readConcreteRoleFile(filepath.Join(rolePath, "prompt.md"))
	if err != nil {
		return Spec{}, err
	}

	var node yaml.Node
	if err := yaml.Unmarshal(roleData, &node); err != nil {
		return Spec{}, err
	}
	if err := rejectInvalidRoleYAML(node.Content); err != nil {
		return Spec{}, err
	}

	var cfg roleConfig
	if err := node.Decode(&cfg); err != nil {
		return Spec{}, err
	}
	prompt := strings.TrimSpace(string(promptData))
	if prompt == "" {
		return Spec{}, fmt.Errorf("role prompt is required")
	}

	return Spec{
		RoleID:      roleID,
		DisplayName: strings.TrimSpace(cfg.DisplayName),
		Description: strings.TrimSpace(cfg.Description),
		Prompt:      prompt,
		SkillNames:  normalizeSkillNames(cfg.Skills),
		Version:     normalizeRoleVersion(cfg.Version),
	}, nil
}

func loadRoleVersionSnapshot(path string) (Spec, error) {
	snapshotData, err := readConcreteRoleFile(path)
	if err != nil {
		return Spec{}, err
	}
	var snapshot roleSnapshot
	if err := yaml.Unmarshal(snapshotData, &snapshot); err != nil {
		return Spec{}, err
	}
	roleID, err := CanonicalRoleID(snapshot.RoleID)
	if err != nil {
		return Spec{}, err
	}
	return Spec{
		RoleID:      roleID,
		DisplayName: strings.TrimSpace(snapshot.DisplayName),
		Description: strings.TrimSpace(snapshot.Description),
		Prompt:      strings.TrimSpace(snapshot.Prompt),
		SkillNames:  normalizeSkillNames(snapshot.SkillNames),
		Version:     normalizeRoleVersion(snapshot.Version),
	}, nil
}

func readConcreteRoleFile(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("role file %s is a symlink: %w", path, os.ErrNotExist)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("role file %s is not a regular file", path)
	}
	return os.ReadFile(path)
}

func normalizeSkillNames(skills []string) []string {
	if len(skills) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(skills))
	out := make([]string, 0, len(skills))
	for _, skill := range skills {
		if trimmed := strings.TrimSpace(skill); trimmed != "" {
			if _, exists := seen[trimmed]; exists {
				continue
			}
			seen[trimmed] = struct{}{}
			out = append(out, trimmed)
		}
	}
	return out
}

func normalizeRoleVersion(version int) int {
	if version <= 0 {
		return 1
	}
	return version
}

func rejectInvalidRoleYAML(content []*yaml.Node) error {
	for _, node := range content {
		if node == nil {
			continue
		}
		if node.Kind != yaml.MappingNode {
			continue
		}
		for i := 0; i+1 < len(node.Content); i += 2 {
			keyNode := node.Content[i]
			valNode := node.Content[i+1]
			if keyNode == nil || valNode == nil {
				continue
			}
			if strings.TrimSpace(keyNode.Value) != "skills" {
				continue
			}
			if valNode.Kind != yaml.SequenceNode {
				return fmt.Errorf("skills must be a sequence")
			}
			for _, item := range valNode.Content {
				if item == nil || item.Kind != yaml.ScalarNode {
					return fmt.Errorf("skills entries must be scalar strings")
				}
			}
		}
	}
	return nil
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
