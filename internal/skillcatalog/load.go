package skillcatalog

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
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
		left := catalog.Skills[i].DisplayName()
		right := catalog.Skills[j].DisplayName()
		if left == right {
			return catalog.Skills[i].Scope < catalog.Skills[j].Scope
		}
		return left < right
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
		if strings.TrimSpace(spec.DisplayName()) != "" {
			skills = append(skills, spec)
		}
	}
	return skills, errors.Join(errs...)
}

func loadSkillFile(path, scope string) (SkillSpec, error) {
	parsed, err := parseSkillFile(path, scope)
	if err != nil {
		return SkillSpec{}, err
	}
	return parsed.Spec, nil
}

type parsedSkillFile struct {
	Path  string
	Raw   []byte
	Meta  []byte
	Body  string
	Front skillFrontMatter
	Spec  SkillSpec
}

func parseSkillFile(path, scope string) (parsedSkillFile, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return parsedSkillFile{}, err
	}
	meta, body, err := splitSkillFrontMatter(raw)
	if err != nil {
		return parsedSkillFile{}, fmt.Errorf("parse skill %s: %w", path, err)
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
	parsed := parsedSkillFile{
		Path: path,
		Raw:  raw,
		Meta: meta,
		Body: body,
		Spec: spec,
	}
	if len(meta) > 0 {
		if err := yaml.Unmarshal(meta, &parsed.Front); err != nil {
			return parsedSkillFile{}, fmt.Errorf("parse front matter: %w", err)
		}
	}
	if trimmed := strings.TrimSpace(parsed.Front.ID); trimmed != "" {
		parsed.Spec.ID = trimmed
	}
	if trimmed := strings.TrimSpace(parsed.Front.Name); trimmed != "" {
		parsed.Spec.Name = trimmed
	} else if trimmed := strings.TrimSpace(parsed.Spec.ID); trimmed != "" {
		parsed.Spec.Name = trimmed
	}
	parsed.Spec.Version = strings.TrimSpace(parsed.Front.Version)
	parsed.Spec.Description = strings.TrimSpace(parsed.Front.Description)
	parsed.Spec.ManifestScope = strings.TrimSpace(parsed.Front.Scope)
	parsed.Spec.Triggers = appendStringLists(parsed.Front.Triggers, parsed.Front.WhenToUse, parsed.Front.WhenToUseHyphen)
	parsed.Spec.RequiresTools = appendStringLists(parsed.Front.RequiresTools, parsed.Front.AllowedTools, parsed.Front.AllowedToolsSnake)
	parsed.Spec.AllowedTools = appendStringLists(parsed.Spec.RequiresTools)
	parsed.Spec.RiskLevel = strings.TrimSpace(parsed.Front.RiskLevel)
	parsed.Spec.ApprovalRequired = parsed.Front.ApprovalRequired
	parsed.Spec.PromptFile = strings.TrimSpace(parsed.Front.PromptFile)
	parsed.Spec.Examples = appendStringLists(parsed.Front.Examples)
	parsed.Spec.Tests = appendStringLists(parsed.Front.Tests)
	parsed.Spec.Permissions = normalizeStringBoolMap(parsed.Front.Permissions)
	parsed.Spec.Policy = parsed.Front.Policy
	parsed.Spec.Agent = parsed.Front.Agent
	return parsed, nil
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
	ID                string        `yaml:"id"`
	Name              string        `yaml:"name"`
	Version           string        `yaml:"version"`
	Description       string        `yaml:"description"`
	Scope             string        `yaml:"scope"`
	RequiresTools     stringList    `yaml:"requires_tools"`
	AllowedTools      stringList    `yaml:"allowed-tools"`
	AllowedToolsSnake stringList    `yaml:"allowed_tools"`
	RiskLevel         string        `yaml:"risk_level"`
	ApprovalRequired  bool          `yaml:"approval_required"`
	PromptFile        string        `yaml:"prompt_file"`
	Examples          stringList    `yaml:"examples"`
	Tests             stringList    `yaml:"tests"`
	Permissions       stringBoolMap `yaml:"permissions"`
	Triggers          stringList    `yaml:"triggers"`
	WhenToUse         stringList    `yaml:"when_to_use"`
	WhenToUseHyphen   stringList    `yaml:"when-to-use"`
	Policy            SkillPolicy   `yaml:"policy"`
	Agent             AgentSpec     `yaml:"agent"`
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

type stringBoolMap map[string]any

func (m *stringBoolMap) UnmarshalYAML(node *yaml.Node) error {
	if node == nil {
		*m = nil
		return nil
	}
	if node.Kind == yaml.ScalarNode && strings.TrimSpace(node.Value) == "" {
		*m = nil
		return nil
	}
	if node.Kind != yaml.MappingNode {
		return fmt.Errorf("expected mapping, got yaml kind %d", node.Kind)
	}

	out := make(map[string]any, len(node.Content)/2)
	for i := 0; i+1 < len(node.Content); i += 2 {
		key := strings.TrimSpace(node.Content[i].Value)
		if key == "" {
			continue
		}
		valueNode := node.Content[i+1]
		switch valueNode.Kind {
		case yaml.ScalarNode:
			value := strings.TrimSpace(valueNode.Value)
			if parsed, err := strconv.ParseBool(value); err == nil {
				out[key] = parsed
			} else {
				out[key] = value
			}
		default:
			return fmt.Errorf("expected string or bool for permissions.%s", key)
		}
	}
	*m = out
	return nil
}

func normalizeStringBoolMap(in map[string]any) map[string]any {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]any, len(in))
	for key, value := range in {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		switch typed := value.(type) {
		case string:
			out[key] = strings.TrimSpace(typed)
		case bool:
			out[key] = typed
		default:
			out[key] = typed
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
