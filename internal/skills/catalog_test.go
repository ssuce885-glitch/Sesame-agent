package skills

import "testing"

func TestActivatePrefersExplicitReferenceOverTrigger(t *testing.T) {
	catalog := Catalog{
		Skills: []SkillSpec{
			{
				Name:         "system-info",
				Scope:        "workspace",
				Triggers:     []string{"查询系统配置"},
				AllowedTools: []string{"shell_command"},
				Policy: SkillPolicy{
					AllowImplicitActivation: true,
				},
				Agent: AgentSpec{
					Tools: []string{"shell_command"},
				},
			},
		},
	}

	activated := Activate(catalog, "请用 $system-info 查询系统配置")
	if len(activated) != 1 {
		t.Fatalf("len(activated) = %d, want 1", len(activated))
	}
	if activated[0].Reason != ActivationReasonExplicit {
		t.Fatalf("Reason = %q, want %q", activated[0].Reason, ActivationReasonExplicit)
	}

	preferred := PreferredTools(activated)
	if len(preferred) != 1 || preferred[0] != "shell_command" {
		t.Fatalf("PreferredTools() = %v, want [shell_command]", preferred)
	}
}

func TestActivateByTrigger(t *testing.T) {
	catalog := Catalog{
		Skills: []SkillSpec{
			{
				Name:     "system-info",
				Scope:    "workspace",
				Triggers: []string{"查询系统配置"},
				Policy: SkillPolicy{
					AllowImplicitActivation: true,
				},
			},
		},
	}

	activated := Activate(catalog, "帮我查询系统配置")
	if len(activated) != 1 {
		t.Fatalf("len(activated) = %d, want 1", len(activated))
	}
	if activated[0].Reason != ActivationReasonTrigger {
		t.Fatalf("Reason = %q, want %q", activated[0].Reason, ActivationReasonTrigger)
	}
}

func TestActivateSkipsTriggerWhenImplicitActivationIsDisabled(t *testing.T) {
	catalog := Catalog{
		Skills: []SkillSpec{
			{
				Name:     "browser-helper",
				Scope:    "workspace",
				Triggers: []string{"打开网页"},
			},
		},
	}

	activated := Activate(catalog, "帮我打开网页")
	if len(activated) != 0 {
		t.Fatalf("len(activated) = %d, want 0", len(activated))
	}
}

func TestSelectByCapabilityTags(t *testing.T) {
	catalog := Catalog{
		Skills: []SkillSpec{
			{
				Name: "browser-helper",
				Policy: SkillPolicy{
					CapabilityTags:     []string{"browser_automation"},
					AllowFullInjection: true,
				},
			},
		},
	}

	selected := SelectByCapabilityTags(catalog, []string{"browser_automation"})
	if len(selected) != 1 {
		t.Fatalf("len(selected) = %d, want 1", len(selected))
	}
	if selected[0].Reason != ActivationReasonProfile {
		t.Fatalf("Reason = %q, want %q", selected[0].Reason, ActivationReasonProfile)
	}
}
