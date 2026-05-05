package skillcatalog

import "strings"

type SkillSpec struct {
	ID               string         `json:"id,omitempty" yaml:"id,omitempty"`
	Name             string         `json:"name,omitempty" yaml:"name,omitempty"`
	Version          string         `json:"version,omitempty" yaml:"version,omitempty"`
	Description      string         `json:"description,omitempty" yaml:"description,omitempty"`
	Path             string         `json:"path,omitempty" yaml:"path,omitempty"`
	Scope            string         `json:"scope,omitempty" yaml:"scope,omitempty"`
	ManifestScope    string         `json:"manifest_scope,omitempty" yaml:"manifest_scope,omitempty"`
	Body             string         `json:"body,omitempty" yaml:"body,omitempty"`
	Triggers         []string       `json:"triggers,omitempty" yaml:"triggers,omitempty"`
	RequiresTools    []string       `json:"requires_tools,omitempty" yaml:"requires_tools,omitempty"`
	AllowedTools     []string       `json:"allowed_tools,omitempty" yaml:"allowed_tools,omitempty"`
	RiskLevel        string         `json:"risk_level,omitempty" yaml:"risk_level,omitempty"`
	ApprovalRequired bool           `json:"approval_required,omitempty" yaml:"approval_required,omitempty"`
	PromptFile       string         `json:"prompt_file,omitempty" yaml:"prompt_file,omitempty"`
	Examples         []string       `json:"examples,omitempty" yaml:"examples,omitempty"`
	Tests            []string       `json:"tests,omitempty" yaml:"tests,omitempty"`
	Permissions      map[string]any `json:"permissions,omitempty" yaml:"permissions,omitempty"`
	Policy           SkillPolicy    `json:"policy,omitempty" yaml:"policy,omitempty"`
	Agent            AgentSpec      `json:"agent,omitempty" yaml:"agent,omitempty"`
}

type SkillPolicy struct {
	AllowImplicitActivation bool     `json:"allow_implicit_activation,omitempty" yaml:"allow_implicit_activation,omitempty"`
	AllowFullInjection      bool     `json:"allow_full_injection,omitempty" yaml:"allow_full_injection,omitempty"`
	CapabilityTags          []string `json:"capability_tags,omitempty" yaml:"capability_tags,omitempty"`
	PreferredTools          []string `json:"preferred_tools,omitempty" yaml:"preferred_tools,omitempty"`
}

type AgentSpec struct {
	Type         string   `json:"type,omitempty" yaml:"type,omitempty"`
	Description  string   `json:"description,omitempty" yaml:"description,omitempty"`
	Instructions string   `json:"instructions,omitempty" yaml:"instructions,omitempty"`
	Tools        []string `json:"tools,omitempty" yaml:"tools,omitempty"`
}

type ToolAsset struct {
	Name        string `json:"name,omitempty" yaml:"name,omitempty"`
	Path        string `json:"path,omitempty" yaml:"path,omitempty"`
	Scope       string `json:"scope,omitempty" yaml:"scope,omitempty"`
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
}

type SkillDirectories struct {
	System    string `json:"system,omitempty" yaml:"system,omitempty"`
	Global    string `json:"global,omitempty" yaml:"global,omitempty"`
	Workspace string `json:"workspace,omitempty" yaml:"workspace,omitempty"`
}

type Catalog struct {
	Skills    []SkillSpec      `json:"skills,omitempty" yaml:"skills,omitempty"`
	Tools     []ToolAsset      `json:"tools,omitempty" yaml:"tools,omitempty"`
	SkillDirs SkillDirectories `json:"skill_dirs,omitempty" yaml:"skill_dirs,omitempty"`
}

func (s SkillSpec) Identifier() string {
	if trimmed := strings.TrimSpace(s.ID); trimmed != "" {
		return trimmed
	}
	return strings.TrimSpace(s.Name)
}

func (s SkillSpec) DisplayName() string {
	if trimmed := strings.TrimSpace(s.Name); trimmed != "" {
		return trimmed
	}
	return strings.TrimSpace(s.ID)
}

func (s SkillSpec) Aliases() []string {
	out := make([]string, 0, 2)
	seen := make(map[string]struct{}, 2)
	for _, value := range []string{s.ID, s.Name} {
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
	return out
}

func (c Catalog) SkillNames() []string {
	if len(c.Skills) == 0 {
		return nil
	}
	names := make([]string, 0, len(c.Skills))
	for _, skill := range c.Skills {
		if trimmed := skill.DisplayName(); trimmed != "" {
			names = append(names, trimmed)
		}
	}
	return names
}

type LintFinding struct {
	Severity string `json:"severity,omitempty" yaml:"severity,omitempty"`
	Field    string `json:"field,omitempty" yaml:"field,omitempty"`
	Message  string `json:"message,omitempty" yaml:"message,omitempty"`
}

const (
	LintSeverityError   = "error"
	LintSeverityWarning = "warning"
)
