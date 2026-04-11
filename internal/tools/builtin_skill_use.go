package tools

import (
	"context"
	"fmt"
	"strings"

	"go-agent/internal/skills"
)

type skillUseTool struct{}

type SkillUseInput struct {
	Name string `json:"name"`
}

type SkillUseOutput struct {
	Name         string   `json:"name"`
	Scope        string   `json:"scope"`
	Path         string   `json:"path"`
	Description  string   `json:"description,omitempty"`
	GrantedTools []string `json:"granted_tools,omitempty"`
	BodyInjected bool     `json:"body_injected"`
}

func (skillUseTool) Definition() Definition {
	return Definition{
		Name:        "skill_use",
		Description: "Load a local skill by name, inject its instructions for the rest of the turn, and unlock any tools the skill grants.",
		InputSchema: objectSchema(map[string]any{
			"name": map[string]any{
				"type":        "string",
				"description": "Installed skill name to activate.",
			},
		}, "name"),
		OutputSchema: objectSchema(map[string]any{
			"name": map[string]any{"type": "string"},
			"scope": map[string]any{
				"type": "string",
			},
			"path": map[string]any{
				"type": "string",
			},
			"description": map[string]any{
				"type": "string",
			},
			"granted_tools": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "string",
				},
			},
			"body_injected": map[string]any{
				"type": "boolean",
			},
		}, "name", "scope", "path", "body_injected"),
	}
}

func (skillUseTool) IsConcurrencySafe() bool { return false }

func (skillUseTool) Decode(call Call) (DecodedCall, error) {
	input := SkillUseInput{
		Name: strings.TrimSpace(call.StringInput("name")),
	}
	if input.Name == "" {
		return DecodedCall{}, fmt.Errorf("name is required")
	}
	return DecodedCall{
		Call: Call{
			ID:   call.ID,
			Name: call.Name,
			Input: map[string]any{
				"name": input.Name,
			},
		},
		Input: input,
	}, nil
}

func (t skillUseTool) Execute(ctx context.Context, call Call, execCtx ExecContext) (Result, error) {
	decoded, err := t.Decode(call)
	if err != nil {
		return Result{}, err
	}
	output, err := t.ExecuteDecoded(ctx, decoded, execCtx)
	return output.Result, err
}

func (skillUseTool) ExecuteDecoded(_ context.Context, decoded DecodedCall, execCtx ExecContext) (ToolExecutionResult, error) {
	input, _ := decoded.Input.(SkillUseInput)
	catalog, err := skills.LoadCatalog(execCtx.GlobalConfigRoot, execCtx.WorkspaceRoot)
	if err != nil {
		return ToolExecutionResult{}, err
	}

	skill, ok := findSkillByName(catalog, input.Name)
	if !ok {
		return ToolExecutionResult{}, fmt.Errorf("installed skill %q not found", input.Name)
	}

	activated := skills.ActivatedSkill{
		Skill:       skill,
		Reason:      skills.ActivationReasonToolUse,
		MatchedText: input.Name,
	}
	grantedTools := skills.GrantedTools([]skills.ActivatedSkill{activated})
	injectedBody := strings.TrimSpace(skills.RenderActivatedSkillsInjection([]skills.ActivatedSkill{activated}))

	modelLines := []string{
		fmt.Sprintf("Activated skill: %s", skill.Name),
	}
	if description := strings.TrimSpace(skill.Description); description != "" {
		modelLines = append(modelLines, "Description: "+description)
	}
	if len(grantedTools) > 0 {
		modelLines = append(modelLines, "Granted tools for the rest of this turn: "+strings.Join(grantedTools, ", "))
	} else {
		modelLines = append(modelLines, "Granted tools for the rest of this turn: none")
	}
	if injectedBody != "" {
		modelLines = append(modelLines, injectedBody)
	} else if summary := strings.TrimSpace(skill.Body); summary != "" {
		modelLines = append(modelLines, "Skill summary:\n"+summary)
	}

	output := SkillUseOutput{
		Name:         skill.Name,
		Scope:        skill.Scope,
		Path:         skill.Path,
		Description:  skill.Description,
		GrantedTools: grantedTools,
		BodyInjected: injectedBody != "",
	}
	preview := fmt.Sprintf("Activated skill %s", skill.Name)
	if len(grantedTools) > 0 {
		preview += " (grants: " + strings.Join(grantedTools, ", ") + ")"
	}
	return ToolExecutionResult{
		Result: Result{
			Text:      preview,
			ModelText: strings.Join(modelLines, "\n\n"),
		},
		Data:        output,
		PreviewText: preview,
		Metadata: map[string]any{
			"activated_skill_names": []string{skill.Name},
			"granted_tools":         grantedTools,
			"skill_name":            skill.Name,
			"skill_scope":           skill.Scope,
			"skill_path":            skill.Path,
		},
	}, nil
}

func (skillUseTool) MapModelResult(output ToolExecutionResult) ModelToolResult {
	return defaultStructuredModelResult(output)
}

func findSkillByName(catalog skills.Catalog, name string) (skills.SkillSpec, bool) {
	wantExact := strings.ToLower(strings.TrimSpace(name))
	for _, skill := range catalog.Skills {
		if strings.ToLower(strings.TrimSpace(skill.Name)) == wantExact {
			return skill, true
		}
	}

	wantCanonical := canonicalSkillLookupName(name)
	for _, skill := range catalog.Skills {
		if canonicalSkillLookupName(skill.Name) == wantCanonical {
			return skill, true
		}
	}
	return skills.SkillSpec{}, false
}

func canonicalSkillLookupName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return ""
	}
	var builder strings.Builder
	builder.Grow(len(name))
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z':
			builder.WriteRune(r)
		case r >= '0' && r <= '9':
			builder.WriteRune(r)
		case r == '.':
			builder.WriteRune(r)
		}
	}
	return builder.String()
}
