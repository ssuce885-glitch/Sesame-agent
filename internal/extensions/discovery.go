package extensions

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type Skill struct {
	Name             string
	Description      string
	Path             string
	Scope            string
	WhenToUse        string
	ToolDependencies []string
	PreferredTools   []string
	ExecutionMode    string
	AgentType        string
	EnvDependencies  []string
	Enabled          bool
}

type ToolAsset struct {
	Name        string
	Path        string
	Scope       string
	Description string
}

type SkillDirectories struct {
	System    string
	Global    string
	Workspace string
}

type Catalog struct {
	Skills    []Skill
	Tools     []ToolAsset
	SkillDirs SkillDirectories
}

func Discover(globalRoot, workspaceRoot string) (Catalog, error) {
	paths, err := resolveExtensionPaths(globalRoot, workspaceRoot)
	if err != nil {
		return Catalog{}, err
	}

	systemSkills, err := readSkillsDir(filepath.Join(paths.GlobalSkillsDir, systemSkillsDirName), ScopeSystem)
	if err != nil {
		return Catalog{}, err
	}
	globalSkills, err := readSkillsDir(paths.GlobalSkillsDir, ScopeGlobal)
	if err != nil {
		return Catalog{}, err
	}
	workspaceSkills, err := readSkillsDir(paths.WorkspaceSkillsDir, ScopeWorkspace)
	if err != nil {
		return Catalog{}, err
	}
	toolSpecs, err := DiscoverToolSpecs(globalRoot, workspaceRoot)
	if err != nil {
		return Catalog{}, err
	}

	skills, err := mergeDiscoveredSkills(systemSkills, globalSkills, workspaceSkills)
	if err != nil {
		return Catalog{}, err
	}

	tools := make([]ToolAsset, 0, len(toolSpecs))
	for _, spec := range toolSpecs {
		tools = append(tools, ToolAsset{
			Name:        spec.Name,
			Path:        spec.Path,
			Scope:       spec.Scope,
			Description: spec.Description,
		})
	}

	return Catalog{
		Skills: skills,
		Tools:  tools,
		SkillDirs: SkillDirectories{
			System:    filepath.Join(paths.GlobalSkillsDir, systemSkillsDirName),
			Global:    paths.GlobalSkillsDir,
			Workspace: paths.WorkspaceSkillsDir,
		},
	}, nil
}

func skillsSummary(catalog Catalog) string {
	lines := make([]string, 0, len(catalog.Skills)+8)
	if catalog.SkillDirs.Global != "" || catalog.SkillDirs.Workspace != "" || catalog.SkillDirs.System != "" {
		lines = append(lines, "Sesame skill directories:")
		if catalog.SkillDirs.Global != "" {
			lines = append(lines, "- global install/load dir: "+catalog.SkillDirs.Global)
		}
		if catalog.SkillDirs.Workspace != "" {
			lines = append(lines, "- workspace install/load dir: "+catalog.SkillDirs.Workspace)
		}
		if catalog.SkillDirs.System != "" {
			lines = append(lines, "- bundled system skills dir: "+catalog.SkillDirs.System)
		}
		lines = append(lines, "Install Sesame skills only into the Sesame directories above, never into .claude/.codex/.cursor or other external tool folders.")
		lines = append(lines, "Repository paths (for example GitHub `--path` values) are source candidates only and do not change the Sesame install destination.")
		lines = append(lines, "Treat the installed-skills list below as the source of truth for what this Sesame session currently has loaded; do not infer extra installed skills by scanning repo folders like .claude/skills or .codex/skills.")
		lines = append(lines, "")
	}
	lines = append(lines, "Available local skills:")
	if len(catalog.Skills) == 0 {
		lines = append(lines, "- (none installed)")
		return strings.Join(lines, "\n")
	}
	for _, skill := range catalog.Skills {
		line := fmt.Sprintf("- %s [%s]", skill.Name, skill.Scope)
		if strings.TrimSpace(skill.Description) != "" {
			line += ": " + strings.TrimSpace(skill.Description)
		}
		lines = append(lines, line)
	}
	lines = append(lines, "If a skill is needed, reference it explicitly as `$skill-name` in the user message.")
	return strings.Join(lines, "\n")
}

func readSkillsDir(root string, scope string) ([]Skill, error) {
	if strings.TrimSpace(root) == "" {
		return nil, nil
	}
	entries, err := os.ReadDir(root)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	skills := make([]Skill, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		dir := filepath.Join(root, entry.Name())
		meta, err := loadSkillMetadata(filepath.Join(dir, "SKILL.json"))
		if err != nil {
			continue
		}
		if _, err := os.Stat(filepath.Join(dir, "SKILL.md")); err != nil {
			continue
		}
		enabled := meta.Enabled == nil || *meta.Enabled
		if !enabled {
			continue
		}
		skills = append(skills, Skill{
			Name:             firstNonEmptyString(meta.Name, entry.Name()),
			Description:      meta.Description,
			Path:             dir,
			Scope:            scope,
			WhenToUse:        strings.TrimSpace(meta.WhenToUse),
			ToolDependencies: append([]string(nil), meta.ToolDependencies...),
			PreferredTools:   append([]string(nil), meta.PreferredTools...),
			ExecutionMode:    strings.TrimSpace(meta.ExecutionMode),
			AgentType:        strings.TrimSpace(meta.AgentType),
			EnvDependencies:  append([]string(nil), meta.EnvDependencies...),
			Enabled:          enabled,
		})
	}
	return skills, nil
}

func mergeDiscoveredSkills(groups ...[]Skill) ([]Skill, error) {
	total := 0
	for _, group := range groups {
		total += len(group)
	}

	merged := make([]Skill, 0, total)
	seenByFoldedName := make(map[string]Skill, total)
	for _, group := range groups {
		for _, skill := range group {
			folded := strings.ToLower(skill.Name)
			if existing, ok := seenByFoldedName[folded]; ok {
				return nil, fmt.Errorf(
					"ambiguous skill name collision: %q (%s, %s) conflicts with %q (%s, %s)",
					existing.Name,
					existing.Scope,
					existing.Path,
					skill.Name,
					skill.Scope,
					skill.Path,
				)
			}
			seenByFoldedName[folded] = skill
			merged = append(merged, skill)
		}
	}

	sort.Slice(merged, func(i, j int) bool {
		left := strings.ToLower(merged[i].Name)
		right := strings.ToLower(merged[j].Name)
		if left == right {
			return merged[i].Name < merged[j].Name
		}
		return left < right
	})

	return merged, nil
}
