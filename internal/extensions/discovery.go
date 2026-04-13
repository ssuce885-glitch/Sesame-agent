package extensions

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

type Skill struct {
	Name         string
	Description  string
	Path         string
	Scope        string
	Body         string
	Triggers     []string
	AllowedTools []string
	Policy       SkillPolicy
	Agent        SkillAgent
}

type SkillPolicy struct {
	AllowImplicitActivation bool
	AllowFullInjection      bool
	CapabilityTags          []string
	PreferredTools          []string
}

type SkillAgent struct {
	Type         string
	Description  string
	Instructions string
	Tools        []string
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

type stringList []string

type skillMetadata struct {
	Name         string             `yaml:"name"`
	Description  string             `yaml:"description"`
	Triggers     stringList         `yaml:"triggers"`
	AllowedTools stringList         `yaml:"allowed-tools"`
	Policy       skillPolicySection `yaml:"policy"`
	Agent        skillAgentSection  `yaml:"agent"`
}

type skillPolicySection struct {
	AllowImplicitActivation *bool      `yaml:"allow_implicit_activation"`
	AllowFullInjection      *bool      `yaml:"allow_full_injection"`
	AllowedTools            stringList `yaml:"allowed-tools"`
	CapabilityTags          stringList `yaml:"capability_tags"`
	PreferredTools          stringList `yaml:"preferred_tools"`
}

type skillAgentSection struct {
	Type         string     `yaml:"type"`
	Description  string     `yaml:"description"`
	Instructions string     `yaml:"instructions"`
	Tools        stringList `yaml:"tools"`
}

type parsedSkillDocument struct {
	Name         string
	Description  string
	Body         string
	Triggers     []string
	AllowedTools []string
	Policy       SkillPolicy
	Agent        SkillAgent
}

func (l *stringList) UnmarshalYAML(node *yaml.Node) error {
	if l == nil || node == nil {
		return nil
	}
	switch node.Kind {
	case yaml.SequenceNode:
		out := make([]string, 0, len(node.Content))
		for _, item := range node.Content {
			value := strings.TrimSpace(item.Value)
			if value != "" {
				out = append(out, value)
			}
		}
		*l = out
		return nil
	case yaml.ScalarNode:
		raw := strings.TrimSpace(node.Value)
		if raw == "" {
			*l = nil
			return nil
		}
		parts := strings.Split(raw, ",")
		out := make([]string, 0, len(parts))
		for _, part := range parts {
			if trimmed := strings.TrimSpace(part); trimmed != "" {
				out = append(out, trimmed)
			}
		}
		*l = out
		return nil
	default:
		return nil
	}
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
		parsed := parseSkillDocument(entry.Name(), string(data))
		skills = append(skills, Skill{
			Name:         parsed.Name,
			Description:  parsed.Description,
			Path:         filepath.Join(root, entry.Name()),
			Scope:        scope,
			Body:         parsed.Body,
			Triggers:     append([]string(nil), parsed.Triggers...),
			AllowedTools: append([]string(nil), parsed.AllowedTools...),
			Policy:       cloneSkillPolicy(parsed.Policy),
			Agent:        cloneSkillAgent(parsed.Agent),
		})
	}
	return skills, nil
}

func parseSkillDocument(defaultName, raw string) parsedSkillDocument {
	name := strings.TrimSpace(defaultName)
	body := strings.TrimSpace(raw)

	if strings.HasPrefix(raw, "---\n") {
		if parsed, ok := parseFrontmatterSkillDocument(defaultName, raw); ok {
			return parsed
		}
	}
	if parsed, ok := parseStructuredSkillDocument(defaultName, raw); ok {
		return parsed
	}

	return parsedSkillDocument{
		Name: name,
		Body: body,
		Policy: SkillPolicy{
			AllowFullInjection: true,
		},
	}
}

func parseFrontmatterSkillDocument(defaultName, raw string) (parsedSkillDocument, bool) {
	rest := raw[len("---\n"):]
	end := strings.Index(rest, "\n---\n")
	if end < 0 {
		return parsedSkillDocument{}, false
	}
	meta, err := decodeSkillMetadata(rest[:end])
	if err != nil {
		return parsedSkillDocument{}, false
	}
	body := strings.TrimSpace(rest[end+len("\n---\n"):])
	return buildParsedSkillDocument(defaultName, meta, body), true
}

func parseStructuredSkillDocument(defaultName, raw string) (parsedSkillDocument, bool) {
	meta, err := decodeSkillMetadata(raw)
	if err != nil {
		return parsedSkillDocument{}, false
	}
	if strings.TrimSpace(meta.Name) == "" &&
		strings.TrimSpace(meta.Description) == "" &&
		len(meta.Triggers) == 0 &&
		meta.Policy.AllowImplicitActivation == nil &&
		meta.Policy.AllowFullInjection == nil &&
		len(meta.Policy.CapabilityTags) == 0 &&
		len(meta.Policy.PreferredTools) == 0 &&
		strings.TrimSpace(meta.Agent.Instructions) == "" &&
		len(meta.Agent.Tools) == 0 &&
		len(meta.AllowedTools) == 0 {
		return parsedSkillDocument{}, false
	}
	return buildParsedSkillDocument(defaultName, meta, renderStructuredSkillBody(meta)), true
}

func decodeSkillMetadata(raw string) (skillMetadata, error) {
	var meta skillMetadata
	if err := yaml.Unmarshal([]byte(raw), &meta); err != nil {
		return skillMetadata{}, err
	}
	meta.Name = strings.TrimSpace(meta.Name)
	meta.Description = strings.TrimSpace(meta.Description)
	meta.Agent.Type = strings.TrimSpace(meta.Agent.Type)
	meta.Agent.Description = strings.TrimSpace(meta.Agent.Description)
	meta.Agent.Instructions = strings.TrimSpace(meta.Agent.Instructions)
	meta.Triggers = normalizeSkillStringList(meta.Triggers)
	meta.Policy.AllowedTools = normalizeSkillStringList(meta.Policy.AllowedTools)
	meta.AllowedTools = normalizeSkillStringList(append(meta.AllowedTools, meta.Policy.AllowedTools...))
	meta.Policy.CapabilityTags = normalizeSkillStringList(meta.Policy.CapabilityTags)
	meta.Policy.PreferredTools = normalizeSkillStringList(meta.Policy.PreferredTools)
	meta.Agent.Tools = normalizeSkillStringList(meta.Agent.Tools)
	return meta, nil
}

func buildParsedSkillDocument(defaultName string, meta skillMetadata, body string) parsedSkillDocument {
	name := strings.TrimSpace(defaultName)
	if meta.Name != "" {
		name = meta.Name
	}
	if strings.TrimSpace(body) == "" {
		body = renderStructuredSkillBody(meta)
	}
	return parsedSkillDocument{
		Name:         name,
		Description:  meta.Description,
		Body:         strings.TrimSpace(body),
		Triggers:     append([]string(nil), meta.Triggers...),
		AllowedTools: append([]string(nil), meta.AllowedTools...),
		Policy:       normalizeSkillPolicy(meta.Policy),
		Agent: SkillAgent{
			Type:         meta.Agent.Type,
			Description:  meta.Agent.Description,
			Instructions: meta.Agent.Instructions,
			Tools:        append([]string(nil), meta.Agent.Tools...),
		},
	}
}

func renderStructuredSkillBody(meta skillMetadata) string {
	lines := make([]string, 0, 8)
	if meta.Description != "" {
		lines = append(lines, "Use this skill when: "+meta.Description)
	}
	if len(meta.Triggers) > 0 {
		lines = append(lines, "Trigger phrases: "+strings.Join(meta.Triggers, ", "))
	}
	if meta.Agent.Type != "" || meta.Agent.Description != "" {
		agentLine := strings.TrimSpace(strings.Join([]string{
			firstNonEmptySkillString(meta.Agent.Type, ""),
			meta.Agent.Description,
		}, " "))
		agentLine = strings.TrimSpace(agentLine)
		if agentLine != "" {
			lines = append(lines, "Preferred execution: "+agentLine)
		}
	}
	if tags := normalizeSkillStringList(meta.Policy.CapabilityTags); len(tags) > 0 {
		lines = append(lines, "Capability tags: "+strings.Join(tags, ", "))
	}
	tools := normalizeSkillStringList(
		append(
			append(
				append(stringList{}, meta.Policy.PreferredTools...),
				meta.AllowedTools...,
			),
			meta.Agent.Tools...,
		),
	)
	if len(tools) > 0 {
		lines = append(lines, "Preferred tools: "+strings.Join(tools, ", "))
	}
	if meta.Agent.Instructions != "" {
		lines = append(lines, "Instructions:\n"+meta.Agent.Instructions)
	}
	return strings.TrimSpace(strings.Join(lines, "\n\n"))
}

func normalizeSkillStringList(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		key := strings.ToLower(trimmed)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, trimmed)
	}
	return out
}

func firstNonEmptySkillString(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func cloneSkillAgent(agent SkillAgent) SkillAgent {
	return SkillAgent{
		Type:         strings.TrimSpace(agent.Type),
		Description:  strings.TrimSpace(agent.Description),
		Instructions: strings.TrimSpace(agent.Instructions),
		Tools:        append([]string(nil), agent.Tools...),
	}
}

func normalizeSkillPolicy(section skillPolicySection) SkillPolicy {
	policy := SkillPolicy{
		AllowFullInjection: true,
		CapabilityTags:     append([]string(nil), section.CapabilityTags...),
		PreferredTools:     append([]string(nil), section.PreferredTools...),
	}
	if section.AllowImplicitActivation != nil {
		policy.AllowImplicitActivation = *section.AllowImplicitActivation
	}
	if section.AllowFullInjection != nil {
		policy.AllowFullInjection = *section.AllowFullInjection
	}
	return policy
}

func cloneSkillPolicy(policy SkillPolicy) SkillPolicy {
	return SkillPolicy{
		AllowImplicitActivation: policy.AllowImplicitActivation,
		AllowFullInjection:      policy.AllowFullInjection,
		CapabilityTags:          append([]string(nil), policy.CapabilityTags...),
		PreferredTools:          append([]string(nil), policy.PreferredTools...),
	}
}
