package toolrouter

import (
	"testing"

	"go-agent/internal/skills"
	"go-agent/internal/tools"
)

func TestDecideHotNewsUsesWebLookupProfile(t *testing.T) {
	decision := Decide("帮我查今天热门新闻", nil)
	if decision.Summary.Profile != ProfileWebLookup {
		t.Fatalf("Profile = %q, want %q", decision.Summary.Profile, ProfileWebLookup)
	}

	filtered := decision.FilterDefinitions([]tools.Definition{
		{Name: "web_fetch"},
		{Name: "skill_use"},
		{Name: "shell_command"},
		{Name: "task_create"},
		{Name: "file_read"},
	})
	assertFilteredToolNames(t, filtered, []string{"web_fetch", "skill_use", "file_read"})
}

func TestDecideBrowserInteractionUsesBrowserAutomationProfile(t *testing.T) {
	decision := Decide("打开 https://example.com 并点击按钮", nil)
	if decision.Summary.Profile != ProfileBrowserAutomation {
		t.Fatalf("Profile = %q, want %q", decision.Summary.Profile, ProfileBrowserAutomation)
	}
	if len(decision.Summary.SkillTags) != 1 || decision.Summary.SkillTags[0] != "browser_automation" {
		t.Fatalf("SkillTags = %v, want [browser_automation]", decision.Summary.SkillTags)
	}
}

func TestDecideScheduledWeatherReportUsesScheduledReportProfile(t *testing.T) {
	decision := Decide("两分钟后告诉我天气", nil)
	if decision.Summary.Profile != ProfileScheduledReport {
		t.Fatalf("Profile = %q, want %q", decision.Summary.Profile, ProfileScheduledReport)
	}

	filtered := decision.FilterDefinitions([]tools.Definition{
		{Name: "schedule_report"},
		{Name: "skill_use"},
		{Name: "task_create"},
		{Name: "shell_command"},
		{Name: "file_read"},
		{Name: "web_fetch"},
	})
	assertFilteredToolNames(t, filtered, []string{"schedule_report"})
}

func TestDecideIncludesPreferredToolsFromActivatedSkills(t *testing.T) {
	decision := Decide("which go", []skills.ActivatedSkill{
		{
			Skill: skills.SkillSpec{
				Name: "system-info",
				Policy: skills.SkillPolicy{
					PreferredTools: []string{"shell_command"},
				},
			},
		},
	})
	if len(decision.Summary.PreferredTools) == 0 || decision.Summary.PreferredTools[0] != "shell_command" {
		t.Fatalf("PreferredTools = %v, want shell_command first", decision.Summary.PreferredTools)
	}
}

func TestDecideAllowsSkillGrantedToolUnderWebLookup(t *testing.T) {
	decision := Decide("帮我查天气然后发邮件", []skills.ActivatedSkill{
		{
			Skill: skills.SkillSpec{
				Name:         "send-email",
				AllowedTools: []string{"shell_command"},
			},
			Reason: skills.ActivationReasonToolUse,
		},
	})
	filtered := decision.FilterDefinitions([]tools.Definition{
		{Name: "web_fetch"},
		{Name: "skill_use"},
		{Name: "shell_command"},
	})
	assertFilteredToolNames(t, filtered, []string{"web_fetch", "skill_use", "shell_command"})
}

func TestDecideAddsLegacyShellGrantForExplicitSkillUse(t *testing.T) {
	decision := Decide("帮我发邮件", []skills.ActivatedSkill{
		{
			Skill: skills.SkillSpec{
				Name: "send-email",
			},
			Reason: skills.ActivationReasonToolUse,
		},
	})
	filtered := decision.FilterDefinitions([]tools.Definition{
		{Name: "web_fetch"},
		{Name: "skill_use"},
		{Name: "shell_command"},
	})
	assertFilteredToolNames(t, filtered, []string{"web_fetch", "skill_use", "shell_command"})
}

func assertFilteredToolNames(t *testing.T, defs []tools.Definition, want []string) {
	t.Helper()
	if len(defs) != len(want) {
		t.Fatalf("len(filtered) = %d, want %d (%v)", len(defs), len(want), want)
	}
	for index, def := range defs {
		if def.Name != want[index] {
			t.Fatalf("filtered[%d] = %q, want %q", index, def.Name, want[index])
		}
	}
}
