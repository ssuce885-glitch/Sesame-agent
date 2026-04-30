package tools

import (
	"testing"

	"go-agent/internal/roles"
)

func TestAutomationPublicSurfaceSimpleChainOnly(t *testing.T) {
	_, execCtx := newSimpleBuilderTestHarness(t)
	execCtx.RoleSpec = &roles.Spec{RoleID: "doc_cleanup_operator"}
	enableAutomationCreateSkills(&execCtx)
	registry := NewRegistry()
	defs := registry.VisibleDefinitions(execCtx)

	seen := make(map[string]bool, len(defs))
	for _, def := range defs {
		seen[def.Name] = true
	}

	required := []string{
		"automation_create_simple",
		"automation_query",
	}
	for _, name := range required {
		if !seen[name] {
			t.Fatalf("expected %q to be visible", name)
		}
	}

	removed := []string{
		"automation_create_detector",
		"automation_configure_incident_policy",
		"automation_configure_dispatch_policy",
		"automation_apply",
		"incident_ack",
		"incident_control",
		"incident_get",
		"incident_list",
		"automation_incident_query",
	}
	for _, name := range removed {
		if seen[name] {
			t.Fatalf("did not expect %q to be visible", name)
		}
	}
}

func TestAutomationMutationToolVisibilityUsesRoleContextAndSkills(t *testing.T) {
	_, execCtx := newSimpleBuilderTestHarness(t)
	registry := NewRegistry()

	mainParentWithoutSkills := visibleToolNames(registry.VisibleDefinitions(execCtx))
	if mainParentWithoutSkills["automation_create_simple"] {
		t.Fatal("automation_create_simple visible outside specialist role context")
	}
	if mainParentWithoutSkills["automation_control"] {
		t.Fatal("automation_control visible without automation-standard-behavior")
	}
	if !mainParentWithoutSkills["automation_query"] {
		t.Fatal("automation_query should remain visible for read-only inspection")
	}
	if !mainParentWithoutSkills["skill_use"] {
		t.Fatal("skill_use must remain visible so the model can activate automation skills")
	}

	execCtx.ActiveSkillNames = []string{"automation-standard-behavior"}
	mainParentWithStandardBehavior := visibleToolNames(registry.VisibleDefinitions(execCtx))
	if !mainParentWithStandardBehavior["automation_control"] {
		t.Fatal("automation_control should be visible after automation-standard-behavior is active")
	}
	if mainParentWithStandardBehavior["automation_create_simple"] {
		t.Fatal("automation_create_simple visible outside specialist role context with automation skills")
	}

	enableAutomationCreateSkills(&execCtx)
	mainParentWithCreateSkills := visibleToolNames(registry.VisibleDefinitions(execCtx))
	if !mainParentWithCreateSkills["automation_control"] {
		t.Fatal("automation_control should remain visible with full automation skills")
	}
	if mainParentWithCreateSkills["automation_create_simple"] {
		t.Fatal("automation_create_simple visible in main_parent context")
	}

	execCtx.ActiveSkillNames = nil
	execCtx.RoleSpec = &roles.Spec{RoleID: "doc_cleanup_operator"}
	specialistWithoutSkills := visibleToolNames(registry.VisibleDefinitions(execCtx))
	if !specialistWithoutSkills["automation_create_simple"] {
		t.Fatal("automation_create_simple should be visible in specialist role context")
	}
	if specialistWithoutSkills["automation_control"] {
		t.Fatal("automation_control visible in specialist role context without automation-standard-behavior")
	}

	execCtx.ActiveSkillNames = []string{"automation-standard-behavior"}
	specialistWithStandardBehavior := visibleToolNames(registry.VisibleDefinitions(execCtx))
	if !specialistWithStandardBehavior["automation_create_simple"] {
		t.Fatal("automation_create_simple should remain visible in specialist role context")
	}
	if !specialistWithStandardBehavior["automation_control"] {
		t.Fatal("automation_control should be visible in specialist role context after automation-standard-behavior is active")
	}
}

func visibleToolNames(defs []Definition) map[string]bool {
	seen := make(map[string]bool, len(defs))
	for _, def := range defs {
		seen[def.Name] = true
	}
	return seen
}
