package skillcatalog

import "strings"

type SkillSpec struct {
	Name         string
	Description  string
	Path         string
	Scope        string
	Body         string
	Triggers     []string
	AllowedTools []string
	Policy       SkillPolicy
	Agent        AgentSpec
}

type SkillPolicy struct {
	AllowImplicitActivation bool
	AllowFullInjection      bool
	CapabilityTags          []string
	PreferredTools          []string
}

type AgentSpec struct {
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
	Skills    []SkillSpec
	Tools     []ToolAsset
	SkillDirs SkillDirectories
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
