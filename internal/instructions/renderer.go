package instructions

import (
	"fmt"
	"strings"

	"go-agent/internal/skills"
)

type Section struct {
	Title string
	Body  string
}

type Bundle struct {
	BaseText     string
	Sections     []Section
	Notices      []string
	ActiveSkills []skills.ActivatedSkill
}

type RenderInput struct {
	BaseText     string
	Catalog      skills.Catalog
	Message      string
	ActiveSkills []skills.ActivatedSkill
}

func Render(input RenderInput) Bundle {
	activated := append([]skills.ActivatedSkill(nil), input.ActiveSkills...)
	notices := skills.ActivationNotices(activated)
	sections := make([]Section, 0, 3)
	injection := skills.BuildPromptInjection(input.Catalog, activated)

	if installedSkills := strings.TrimSpace(injection.InstalledSkills); installedSkills != "" {
		sections = append(sections, Section{
			Title: "Installed skills",
			Body:  installedSkills,
		})
	}
	if activeContext := strings.TrimSpace(injection.ActiveContext); activeContext != "" {
		sections = append(sections, Section{
			Title: "Active context",
			Body:  activeContext,
		})
	}
	if guidance := strings.TrimSpace(injection.ActiveSkillGuidance); guidance != "" {
		sections = append(sections, Section{
			Title: "Active skill instructions",
			Body:  guidance,
		})
	}
	if catalogSnapshot := renderCatalogSnapshotIfRequested(input.Catalog, input.Message); strings.TrimSpace(catalogSnapshot) != "" {
		sections = append(sections, Section{
			Title: "Catalog snapshot",
			Body:  catalogSnapshot,
		})
	}
	return Bundle{
		BaseText:     strings.TrimSpace(input.BaseText),
		Sections:     sections,
		Notices:      notices,
		ActiveSkills: activated,
	}
}

func (b Bundle) Render() string {
	parts := make([]string, 0, len(b.Sections)+1)
	if strings.TrimSpace(b.BaseText) != "" {
		parts = append(parts, strings.TrimSpace(b.BaseText))
	}
	for _, section := range b.Sections {
		body := strings.TrimSpace(section.Body)
		if body == "" {
			continue
		}
		if title := strings.TrimSpace(section.Title); title != "" {
			parts = append(parts, "## "+title+"\n"+body)
			continue
		}
		parts = append(parts, body)
	}
	return strings.Join(parts, "\n\n")
}

func renderCatalogSnapshotIfRequested(catalog skills.Catalog, message string) string {
	message = strings.ToLower(strings.TrimSpace(message))
	if message == "" {
		return ""
	}

	includeSkills := isCatalogSkillQuery(message)
	includeTools := isCatalogToolQuery(message)
	if !includeSkills && !includeTools {
		return ""
	}

	lines := make([]string, 0, 16)
	lines = append(lines, "- Installed catalog is separate from turn-visible tools for this request.")

	if includeSkills {
		lines = append(lines, renderSkillCatalogSnapshot(catalog)...)
	}
	if includeTools {
		lines = append(lines, renderToolCatalogSnapshot(catalog)...)
	}
	return strings.Join(lines, "\n")
}

func isCatalogSkillQuery(message string) bool {
	for _, needle := range []string{
		"/skills",
		"skills folder",
		"skill folder",
		"skill list",
		"installed skill",
		"available skill",
		"what skills",
		"which skills",
		"你的skills",
		"skills文件夹",
		"skill文件夹",
		"技能",
		"skill",
		"skills",
	} {
		if strings.Contains(message, needle) {
			return true
		}
	}
	return false
}

func isCatalogToolQuery(message string) bool {
	for _, needle := range []string{
		"/tools",
		"tool list",
		"installed tool",
		"available tool",
		"what tools",
		"which tools",
		"你的tools",
		"tools文件夹",
		"tool文件夹",
		"工具列表",
		"工具",
		"tools",
	} {
		if strings.Contains(message, needle) {
			return true
		}
	}
	return false
}

func renderSkillCatalogSnapshot(catalog skills.Catalog) []string {
	lines := []string{
		"- Skill directories:",
	}
	if dir := strings.TrimSpace(catalog.SkillDirs.System); dir != "" {
		lines = append(lines, "  system: "+dir)
	}
	if dir := strings.TrimSpace(catalog.SkillDirs.Global); dir != "" {
		lines = append(lines, "  global: "+dir)
	}
	if dir := strings.TrimSpace(catalog.SkillDirs.Workspace); dir != "" {
		lines = append(lines, "  workspace: "+dir)
	}
	if len(catalog.Skills) == 0 {
		lines = append(lines, "- Installed skills: none")
		return lines
	}
	lines = append(lines, "- Installed skills:")
	for _, skill := range catalog.Skills {
		line := fmt.Sprintf("  - %s [%s]", skill.Name, skill.Scope)
		if description := strings.TrimSpace(skill.Description); description != "" {
			line += ": " + description
		}
		if path := strings.TrimSpace(skill.Path); path != "" {
			line += " (" + path + ")"
		}
		lines = append(lines, line)
	}
	return lines
}

func renderToolCatalogSnapshot(catalog skills.Catalog) []string {
	if len(catalog.Tools) == 0 {
		return []string{"- Installed local tool assets: none"}
	}
	lines := []string{"- Installed local tool assets:"}
	for _, tool := range catalog.Tools {
		line := fmt.Sprintf("  - %s [%s]", tool.Name, tool.Scope)
		if description := strings.TrimSpace(tool.Description); description != "" {
			line += ": " + description
		}
		if path := strings.TrimSpace(tool.Path); path != "" {
			line += " (" + path + ")"
		}
		lines = append(lines, line)
	}
	return lines
}
