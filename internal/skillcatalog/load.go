package skillcatalog

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// LoadCatalog loads skills from the standard global and workspace skill
// directories. Missing directories are treated as an empty catalog.
func LoadCatalog(globalRoot, workspaceRoot string) (Catalog, error) {
	catalog := Catalog{
		SkillDirs: SkillDirectories{
			System:    joinIfBase(globalRoot, "system", "skills"),
			Global:    joinIfBase(globalRoot, "skills"),
			Workspace: joinIfBase(workspaceRoot, "skills"),
		},
	}

	var errs []error
	for _, dir := range []struct {
		path  string
		scope string
	}{
		{path: catalog.SkillDirs.System, scope: "system"},
		{path: catalog.SkillDirs.Global, scope: "global"},
		{path: catalog.SkillDirs.Workspace, scope: "workspace"},
	} {
		skills, err := loadSkillDir(dir.path, dir.scope)
		if err != nil {
			errs = append(errs, err)
		}
		catalog.Skills = append(catalog.Skills, skills...)
	}

	sort.SliceStable(catalog.Skills, func(i, j int) bool {
		if catalog.Skills[i].Name == catalog.Skills[j].Name {
			return catalog.Skills[i].Scope < catalog.Skills[j].Scope
		}
		return catalog.Skills[i].Name < catalog.Skills[j].Name
	})

	return catalog, errors.Join(errs...)
}

func joinIfBase(base string, elems ...string) string {
	if strings.TrimSpace(base) == "" {
		return ""
	}
	parts := append([]string{base}, elems...)
	return filepath.Join(parts...)
}

func loadSkillDir(dir, scope string) ([]SkillSpec, error) {
	if strings.TrimSpace(dir) == "" {
		return nil, nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read skill dir %s: %w", dir, err)
	}

	var skills []SkillSpec
	var errs []error
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		if entry.IsDir() {
			path = filepath.Join(path, "SKILL.md")
		} else if !strings.EqualFold(filepath.Ext(entry.Name()), ".md") {
			continue
		}

		spec, err := loadSkillFile(path, scope)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			errs = append(errs, err)
			continue
		}
		if strings.TrimSpace(spec.Name) != "" {
			skills = append(skills, spec)
		}
	}
	return skills, errors.Join(errs...)
}

func loadSkillFile(path, scope string) (SkillSpec, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return SkillSpec{}, err
	}
	meta, body, err := splitSkillFrontMatter(raw)
	if err != nil {
		return SkillSpec{}, fmt.Errorf("parse skill %s: %w", path, err)
	}

	name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	if strings.EqualFold(name, "SKILL") {
		name = filepath.Base(filepath.Dir(path))
	}
	spec := SkillSpec{
		Name:  name,
		Path:  path,
		Scope: scope,
		Body:  strings.TrimSpace(body),
	}
	if len(meta) > 0 {
		var front skillFrontMatter
		if err := yaml.Unmarshal(meta, &front); err != nil {
			return SkillSpec{}, fmt.Errorf("parse front matter: %w", err)
		}
		if strings.TrimSpace(front.Name) != "" {
			spec.Name = strings.TrimSpace(front.Name)
		}
		spec.Description = strings.TrimSpace(front.Description)
		spec.Triggers = appendStringLists(front.Triggers, front.WhenToUse, front.WhenToUseHyphen)
		spec.AllowedTools = appendStringLists(front.AllowedTools, front.AllowedToolsSnake)
		spec.Policy = front.Policy
		spec.Agent = front.Agent
	}
	return spec, nil
}

func splitSkillFrontMatter(raw []byte) ([]byte, string, error) {
	raw = bytes.TrimPrefix(raw, []byte("\xef\xbb\xbf"))
	if !bytes.HasPrefix(raw, []byte("---\n")) && !bytes.HasPrefix(raw, []byte("---\r\n")) {
		return nil, string(raw), nil
	}
	lines := bytes.SplitAfter(raw, []byte("\n"))
	if len(lines) == 0 {
		return nil, string(raw), nil
	}
	offset := len(lines[0])
	for i := 1; i < len(lines); i++ {
		line := strings.TrimSpace(string(lines[i]))
		if line != "---" {
			offset += len(lines[i])
			continue
		}
		metaStart := len(lines[0])
		metaEnd := offset
		bodyStart := offset + len(lines[i])
		return raw[metaStart:metaEnd], string(raw[bodyStart:]), nil
	}
	return nil, string(raw), fmt.Errorf("unterminated front matter")
}

func appendStringLists(values ...[]string) []string {
	var out []string
	seen := make(map[string]struct{})
	for _, list := range values {
		for _, value := range list {
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
	}
	return out
}

type skillFrontMatter struct {
	Name              string      `yaml:"name"`
	Description       string      `yaml:"description"`
	AllowedTools      stringList  `yaml:"allowed-tools"`
	AllowedToolsSnake stringList  `yaml:"allowed_tools"`
	Triggers          stringList  `yaml:"triggers"`
	WhenToUse         stringList  `yaml:"when_to_use"`
	WhenToUseHyphen   stringList  `yaml:"when-to-use"`
	Policy            SkillPolicy `yaml:"policy"`
	Agent             AgentSpec   `yaml:"agent"`
}

type stringList []string

func (l *stringList) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.SequenceNode:
		out := make([]string, 0, len(node.Content))
		for _, item := range node.Content {
			out = append(out, strings.TrimSpace(item.Value))
		}
		*l = out
		return nil
	case yaml.ScalarNode:
		value := strings.TrimSpace(node.Value)
		if value == "" {
			*l = nil
			return nil
		}
		parts := strings.Split(value, ",")
		out := make([]string, 0, len(parts))
		for _, part := range parts {
			if trimmed := strings.TrimSpace(part); trimmed != "" {
				out = append(out, trimmed)
			}
		}
		*l = out
		return nil
	default:
		return fmt.Errorf("expected string or list, got yaml kind %d", node.Kind)
	}
}
