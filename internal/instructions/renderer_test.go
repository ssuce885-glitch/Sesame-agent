package instructions

import (
	"strings"
	"testing"

	"go-agent/internal/skills"
)

func TestRenderIncludesActiveSkillDescriptionsAndFullInstructions(t *testing.T) {
	bundle := Render(RenderInput{
		BaseText: "base",
		Catalog: skills.Catalog{
			Skills: []skills.SkillSpec{
				{
					Name:        "automation-standard-behavior",
					Description: "Use when a user is defining or managing a long-running simple-chain automation.",
					Scope:       "system",
					Policy: skills.SkillPolicy{
						AllowFullInjection: true,
					},
				},
				{
					Name:        "agent-browser",
					Description: "Browser automation helper for websites and forms.",
					Scope:       "workspace",
					Policy: skills.SkillPolicy{
						AllowFullInjection: false,
					},
				},
			},
		},
		ActiveSkills: []skills.ActivatedSkill{
			{
				Skill: skills.SkillSpec{
					Name:        "automation-standard-behavior",
					Description: "Use when a user is defining or managing a long-running simple-chain automation.",
					Body:        "Owner-task mode executes the business action defined by automation_goal.",
					Policy: skills.SkillPolicy{
						AllowFullInjection: true,
					},
				},
			},
		},
	})

	rendered := bundle.Render()
	if !strings.Contains(rendered, "## Installed skills") {
		t.Fatalf("Render() missing installed skills section:\n%s", rendered)
	}
	if !strings.Contains(rendered, "- agent-browser [scope=workspace | description=Browser automation helper for websites and forms.]") {
		t.Fatalf("Render() missing inactive installed skill card:\n%s", rendered)
	}
	if strings.Contains(rendered, "- automation-standard-behavior [") {
		t.Fatalf("Render() should not include active skill in installed skills section:\n%s", rendered)
	}
	if !strings.Contains(rendered, "## Active context") {
		t.Fatalf("Render() missing active context section:\n%s", rendered)
	}
	if !strings.Contains(rendered, "- automation-standard-behavior: Use when a user is defining or managing a long-running simple-chain automation.") {
		t.Fatalf("Render() missing active skill summary:\n%s", rendered)
	}
	if !strings.Contains(rendered, "## Active skill instructions") {
		t.Fatalf("Render() missing active skill instructions section:\n%s", rendered)
	}
	if !strings.Contains(rendered, "Skill: automation-standard-behavior\nOwner-task mode executes the business action defined by automation_goal.") {
		t.Fatalf("Render() missing full injected skill guidance:\n%s", rendered)
	}
	if strings.Contains(rendered, "This body should not be injected in normal compile output.") {
		t.Fatalf("Render() injected a skill body with AllowFullInjection disabled:\n%s", rendered)
	}
}
