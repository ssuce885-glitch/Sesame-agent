package skillcatalog

import "strings"

type SkillSpec struct {
	Name         string      `json:"name,omitempty" yaml:"name,omitempty"`
	Description  string      `json:"description,omitempty" yaml:"description,omitempty"`
	Path         string      `json:"path,omitempty" yaml:"path,omitempty"`
	Scope        string      `json:"scope,omitempty" yaml:"scope,omitempty"`
	Body         string      `json:"body,omitempty" yaml:"body,omitempty"`
	Triggers     []string    `json:"triggers,omitempty" yaml:"triggers,omitempty"`
	AllowedTools []string    `json:"allowed_tools,omitempty" yaml:"allowed_tools,omitempty"`
	Policy       SkillPolicy `json:"policy,omitempty" yaml:"policy,omitempty"`
	Agent        AgentSpec   `json:"agent,omitempty" yaml:"agent,omitempty"`
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

func (c Catalog) SkillNames() []string {
	if len(c.Skills) == 0 {
		return nil
	}
	names := make([]string, 0, len(c.Skills))
	for _, skill := range c.Skills {
		if trimmed := strings.TrimSpace(skill.Name); trimmed != "" {
			names = append(names, trimmed)
		}
	}
	return names
}
