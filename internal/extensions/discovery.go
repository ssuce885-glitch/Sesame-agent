package extensions

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var skillRefPattern = regexp.MustCompile(`\$([A-Za-z0-9._-]+)`)

type Skill struct {
	Name        string
	Description string
	Path        string
	Scope       string
	Body        string
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

	skillMap := make(map[string]Skill, len(systemSkills)+len(globalSkills)+len(workspaceSkills))
	for _, skill := range systemSkills {
		skillMap[strings.ToLower(skill.Name)] = skill
	}
	for _, skill := range globalSkills {
		skillMap[strings.ToLower(skill.Name)] = skill
	}
	for _, skill := range workspaceSkills {
		skillMap[strings.ToLower(skill.Name)] = skill
	}
	skills := make([]Skill, 0, len(skillMap))
	for _, skill := range skillMap {
		skills = append(skills, skill)
	}
	sort.Slice(skills, func(i, j int) bool {
		left := strings.ToLower(skills[i].Name)
		right := strings.ToLower(skills[j].Name)
		if left == right {
			return skills[i].Name < skills[j].Name
		}
		return left < right
	})

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

func BuildPromptSection(catalog Catalog, userMessage string) (string, []string) {
	parts := make([]string, 0, 2)
	notices := make([]string, 0, 4)
	if summary := skillsSummary(catalog); summary != "" {
		parts = append(parts, summary)
	}
	activated := activatedSkills(catalog.Skills, userMessage)
	if len(activated) > 0 {
		detail := make([]string, 0, len(activated))
		for _, skill := range activated {
			notices = append(notices, fmt.Sprintf("Activated local skill: %s", skill.Name))
			detail = append(detail, fmt.Sprintf("## %s (%s)\n%s", skill.Name, skill.Scope, strings.TrimSpace(skill.Body)))
		}
		parts = append(parts, "Activated local skills:\n\n"+strings.Join(detail, "\n\n"))
	}
	return strings.Join(parts, "\n\n"), notices
}

func activatedSkills(skills []Skill, userMessage string) []Skill {
	if len(skills) == 0 || strings.TrimSpace(userMessage) == "" {
		return nil
	}
	names := make(map[string]Skill, len(skills))
	for _, skill := range skills {
		names[strings.ToLower(skill.Name)] = skill
	}
	seen := make(map[string]struct{})
	out := make([]Skill, 0, 4)
	for _, match := range skillRefPattern.FindAllStringSubmatch(userMessage, -1) {
		if len(match) < 2 {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(match[1]))
		if key == "" {
			continue
		}
		skill, ok := names[key]
		if !ok {
			continue
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, skill)
	}
	return out
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
		skillPath := filepath.Join(root, entry.Name(), "SKILL.md")
		data, err := os.ReadFile(skillPath)
		if err != nil {
			continue
		}
		name, description, body := parseSkillDocument(entry.Name(), string(data))
		skills = append(skills, Skill{
			Name:        name,
			Description: description,
			Path:        filepath.Join(root, entry.Name()),
			Scope:       scope,
			Body:        body,
		})
	}
	return skills, nil
}

func parseSkillDocument(defaultName, raw string) (string, string, string) {
	name := strings.TrimSpace(defaultName)
	description := ""
	body := strings.TrimSpace(raw)
	if !strings.HasPrefix(raw, "---\n") {
		return name, description, body
	}
	rest := raw[len("---\n"):]
	end := strings.Index(rest, "\n---\n")
	if end < 0 {
		return name, description, body
	}
	frontmatter := rest[:end]
	body = strings.TrimSpace(rest[end+len("\n---\n"):])
	for _, line := range strings.Split(frontmatter, "\n") {
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key = strings.TrimSpace(strings.ToLower(key))
		value = strings.Trim(strings.TrimSpace(value), `"'`)
		switch key {
		case "name":
			if value != "" {
				name = value
			}
		case "description":
			if value != "" {
				description = value
			}
		}
	}
	return name, description, body
}
