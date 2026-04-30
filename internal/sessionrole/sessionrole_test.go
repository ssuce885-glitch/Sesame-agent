package sessionrole

import (
	"strings"
	"testing"

	"go-agent/internal/roles"
	"go-agent/internal/types"
)

func TestMainParentPromptRequiresSkillBeforeAutomationControl(t *testing.T) {
	prompt := DefaultSystemPrompt(types.SessionRoleMainParent)
	required := []string{
		"Before creating, modifying, pausing, or resuming automations",
		"activate the relevant automation skills",
		"automation-standard-behavior",
		"automation-normalizer",
	}
	for _, text := range required {
		if !strings.Contains(prompt, text) {
			t.Fatalf("main parent prompt missing %q:\n%s", text, prompt)
		}
	}
}

func TestMainParentPromptUsesPersonalAssistantIdentity(t *testing.T) {
	prompt := DefaultSystemPrompt(types.SessionRoleMainParent)
	required := []string{
		"local personal assistant",
		"Do not default to a software-engineering or coding-assistant identity",
	}
	for _, text := range required {
		if !strings.Contains(prompt, text) {
			t.Fatalf("main parent prompt missing %q:\n%s", text, prompt)
		}
	}
}

func TestShouldRefreshDefaultSystemPromptRefreshesLegacyMainParent(t *testing.T) {
	legacy := `# Main Parent Role
You are the main parent session for this workspace.
You are the primary user-facing persona of Sesame-agent.`

	if !ShouldRefreshDefaultSystemPrompt(types.SessionRoleMainParent, legacy) {
		t.Fatal("expected legacy main parent prompt to refresh")
	}
	if ShouldRefreshDefaultSystemPrompt(types.SessionRoleMainParent, "custom prompt") {
		t.Fatal("custom main parent prompt should not refresh")
	}
}

func TestSpecialistSystemPromptMentionsMemoryAndArchiveTools(t *testing.T) {
	prompt := SpecialistSystemPrompt(roles.Spec{RoleID: "analyst", DisplayName: "Analyst"})
	for _, required := range []string{
		"# Memory and archive",
		"recall_archive",
		"load_context",
		"memory_write",
	} {
		if !strings.Contains(prompt, required) {
			t.Fatalf("specialist prompt missing %q:\n%s", required, prompt)
		}
	}
}

func TestSpecialistSystemPromptIncludesOutputSchema(t *testing.T) {
	schema := `{"type":"object","required":["summary"]}`
	prompt := SpecialistSystemPrompt(roles.Spec{
		RoleID: "analyst",
		Policy: &roles.RolePolicyConfig{
			OutputSchema: schema,
		},
	})
	for _, required := range []string{
		"# Output format",
		"valid JSON matching this schema",
		schema,
	} {
		if !strings.Contains(prompt, required) {
			t.Fatalf("specialist prompt missing %q:\n%s", required, prompt)
		}
	}
}
