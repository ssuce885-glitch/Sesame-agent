package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"go-agent/internal/skillcatalog"
	"go-agent/internal/v2/contracts"
)

type skillUseTool struct {
	catalog skillcatalog.Catalog
}

type skillUseResult struct {
	ActivatedSkills []string `json:"activated_skill_names"`
}

func (r skillUseResult) ActivatedSkillNames() []string {
	return append([]string(nil), r.ActivatedSkills...)
}

func NewSkillUseTool(catalog skillcatalog.Catalog) contracts.Tool {
	return &skillUseTool{catalog: catalog}
}

func (t *skillUseTool) Definition() contracts.ToolDefinition {
	return contracts.ToolDefinition{
		Name:        "skill_use",
		Namespace:   contracts.NamespaceSkill,
		Description: "Activate a skill to gain domain knowledge and optional tool access.",
		Capabilities: []string{
			string(contracts.CapabilityMutateRuntime),
		},
		Risk: "low",
		Parameters: objectSchema(map[string]any{
			"skill": map[string]any{"type": "string", "description": "Skill name to activate"},
		}, "skill"),
	}
}

func (t *skillUseTool) Execute(ctx context.Context, call contracts.ToolCall, execCtx contracts.ExecContext) (contracts.ToolResult, error) {
	_ = ctx
	_ = execCtx

	skillName, _ := call.Args["skill"].(string)
	skillName = strings.TrimSpace(skillName)
	if skillName == "" {
		return contracts.ToolResult{Output: "skill is required", IsError: true}, nil
	}

	names := t.catalog.SkillNames()
	available := make(map[string]string, len(names)*2)
	for _, skill := range t.catalog.Skills {
		canonical := skill.Identifier()
		if canonical == "" {
			continue
		}
		for _, alias := range skill.Aliases() {
			available[alias] = canonical
		}
	}
	canonical, ok := available[skillName]
	if !ok {
		sort.Strings(names)
		msg := fmt.Sprintf("skill %q not found", skillName)
		if len(names) > 0 {
			msg += "; available skills: " + strings.Join(names, ", ")
		}
		return contracts.ToolResult{Output: msg, IsError: true}, nil
	}

	result := skillUseResult{ActivatedSkills: []string{canonical}}
	raw, err := json.Marshal(result)
	if err != nil {
		return contracts.ToolResult{}, err
	}
	return contracts.ToolResult{Output: string(raw), Data: result}, nil
}
