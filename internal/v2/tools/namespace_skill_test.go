package tools

import (
	"context"
	"reflect"
	"testing"

	"go-agent/internal/skillcatalog"
	"go-agent/internal/v2/contracts"
)

func TestSkillUseCanonicalizesAliasActivation(t *testing.T) {
	tool := NewSkillUseTool(skillcatalog.Catalog{
		Skills: []skillcatalog.SkillSpec{
			{
				ID:   automationStandardBehaviorSkill,
				Name: "Automation Standard Behavior",
			},
		},
	})

	for _, input := range []string{
		automationStandardBehaviorSkill,
		"Automation Standard Behavior",
	} {
		result, err := tool.Execute(context.Background(), contracts.ToolCall{
			Name: "skill_use",
			Args: map[string]any{"skill": input},
		}, contracts.ExecContext{})
		if err != nil {
			t.Fatalf("Execute(%q): %v", input, err)
		}
		if result.IsError {
			t.Fatalf("Execute(%q) returned error result: %+v", input, result)
		}

		data, ok := result.Data.(skillUseResult)
		if !ok {
			t.Fatalf("result.Data type = %T, want skillUseResult", result.Data)
		}
		if got, want := data.ActivatedSkillNames(), []string{automationStandardBehaviorSkill}; !reflect.DeepEqual(got, want) {
			t.Fatalf("ActivatedSkillNames(%q) = %#v, want %#v", input, got, want)
		}
		if !hasActiveSkill(contracts.ExecContext{ActiveSkills: data.ActivatedSkillNames()}, automationStandardBehaviorSkill) {
			t.Fatalf("expected canonical activation to satisfy automation gate for %q", input)
		}
	}
}
