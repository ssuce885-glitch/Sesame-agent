package tools

import "testing"

func TestAutomationPublicSurfaceSimpleChainOnly(t *testing.T) {
	_, execCtx := newSimpleBuilderTestHarness(t)
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

func TestAutomationMutationToolsAreHiddenUntilRequiredSkillsAreActive(t *testing.T) {
	_, execCtx := newSimpleBuilderTestHarness(t)
	registry := NewRegistry()

	withoutSkills := visibleToolNames(registry.VisibleDefinitions(execCtx))
	if withoutSkills["automation_create_simple"] {
		t.Fatal("automation_create_simple visible without automation skills")
	}
	if withoutSkills["automation_control"] {
		t.Fatal("automation_control visible without automation-standard-behavior")
	}
	if !withoutSkills["automation_query"] {
		t.Fatal("automation_query should remain visible for read-only inspection")
	}
	if !withoutSkills["skill_use"] {
		t.Fatal("skill_use must remain visible so the model can activate automation skills")
	}

	execCtx.ActiveSkillNames = []string{"automation-standard-behavior"}
	withStandardBehavior := visibleToolNames(registry.VisibleDefinitions(execCtx))
	if !withStandardBehavior["automation_control"] {
		t.Fatal("automation_control should be visible after automation-standard-behavior is active")
	}
	if withStandardBehavior["automation_create_simple"] {
		t.Fatal("automation_create_simple visible before automation-normalizer is active")
	}

	enableAutomationCreateSkills(&execCtx)
	withCreateSkills := visibleToolNames(registry.VisibleDefinitions(execCtx))
	if !withCreateSkills["automation_control"] {
		t.Fatal("automation_control should remain visible with full automation skills")
	}
	if !withCreateSkills["automation_create_simple"] {
		t.Fatal("automation_create_simple should be visible after both automation skills are active")
	}
}

func visibleToolNames(defs []Definition) map[string]bool {
	seen := make(map[string]bool, len(defs))
	for _, def := range defs {
		seen[def.Name] = true
	}
	return seen
}
