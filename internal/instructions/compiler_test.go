package instructions

import (
	"strings"
	"testing"

	"go-agent/internal/skills"
	"go-agent/internal/toolrouter"
)

func TestCompileOmitsAmbientLocalSkillSummaryByDefault(t *testing.T) {
	catalog := skills.Catalog{
		Skills: []skills.SkillSpec{
			{
				Name:        "browser-helper",
				Scope:       "workspace",
				Description: "Open pages and click buttons.",
				Body:        "which browser-cli\nwhich playwright",
				Policy: skills.SkillPolicy{
					CapabilityTags: []string{"browser_automation"},
				},
			},
		},
	}

	bundle := Compile(CompileInput{
		BaseText: "Base system text.",
		Catalog:  catalog,
		Message:  "帮我查今天热门新闻",
		Policy: toolrouter.PolicySummary{
			Profile: toolrouter.ProfileWebLookup,
		},
		VisibleTools: []string{"skill_use", "web_fetch"},
	})
	rendered := bundle.Render()

	for _, unwanted := range []string{
		"## Local skills",
		"## Implicit skill hints",
		"## Relevant skills",
		"browser-helper",
		"Open pages and click buttons.",
		"which browser-cli",
	} {
		if strings.Contains(rendered, unwanted) {
			t.Fatalf("Render() unexpectedly included %q:\n%s", unwanted, rendered)
		}
	}
}

func TestCompileRendersImplicitHintsActivatedSkillsAndToolPolicy(t *testing.T) {
	catalog := skills.Catalog{
		Skills: []skills.SkillSpec{
			{
				Name:        "web-news",
				Scope:       "workspace",
				Description: "Summarize hot news from fetched webpages.",
				Policy: skills.SkillPolicy{
					AllowImplicitActivation: true,
				},
			},
			{
				Name:        "send-email",
				Scope:       "global",
				Description: "Send emails via SMTP.",
				Body:        "Run the SMTP sender script.",
				Policy: skills.SkillPolicy{
					AllowFullInjection: true,
					PreferredTools:     []string{"shell_command"},
				},
			},
		},
	}
	activeSkills := []skills.ActivatedSkill{
		{
			Skill:  catalog.Skills[1],
			Reason: skills.ActivationReasonRetrieved,
		},
	}

	bundle := Compile(CompileInput{
		BaseText: "Base system text.",
		Catalog:  catalog,
		Message:  "帮我查今天热门新闻，然后发邮件给我摘要",
		Policy: toolrouter.PolicySummary{
			Profile:        toolrouter.ProfileWebLookup,
			Guidance:       []string{"Use direct web fetching for public webpages and answer directly from fetched pages."},
			PreferredTools: []string{"web_fetch", "shell_command"},
			MaxSteps:       4,
			MaxFetches:     3,
		},
		VisibleTools: []string{"skill_use", "shell_command", "web_fetch"},
		ActiveSkills: activeSkills,
	})
	rendered := bundle.Render()

	for _, want := range []string{
		"Base system text.",
		"## Implicit skill hints",
		"## Activated skills",
		"## Tool routing",
		"- web-news: Summarize hot news from fetched webpages.",
		"Run the SMTP sender script.",
		"Granted tools: shell_command",
		"Profile: web_lookup",
		"Preferred tools: web_fetch, shell_command",
		"Model-visible tools: skill_use, shell_command, web_fetch",
		"Soft limits: max_steps=4, max_fetches=3",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("Render() missing %q:\n%s", want, rendered)
		}
	}
	if len(bundle.Notices) != 1 || !strings.Contains(bundle.Notices[0], "send-email") {
		t.Fatalf("Notices = %v, want activation notice", bundle.Notices)
	}
}

func TestCompileRendersCatalogSnapshotForSkillQuery(t *testing.T) {
	catalog := skills.Catalog{
		Skills: []skills.SkillSpec{
			{
				Name:        "agent-browser",
				Scope:       "global",
				Path:        "/home/demo/.sesame/skills/agent-browser",
				Description: "Browser automation helper.",
				Body:        "which playwright\nwhich browser-cli",
			},
			{
				Name:        "skill-normalizer",
				Scope:       "system",
				Path:        "/home/demo/.sesame/skills/.system/skill-normalizer",
				Description: "Normalize third-party skills.",
			},
		},
		SkillDirs: skills.SkillDirectories{
			System:    "/home/demo/.sesame/skills/.system",
			Global:    "/home/demo/.sesame/skills",
			Workspace: "/workspace/.sesame/skills",
		},
	}

	bundle := Compile(CompileInput{
		BaseText: "Base system text.",
		Catalog:  catalog,
		Message:  "你当前skills文件夹里有哪些skills？",
		Policy: toolrouter.PolicySummary{
			Profile: toolrouter.ProfileCodebaseEdit,
		},
		VisibleTools: []string{"file_read"},
	})
	rendered := bundle.Render()

	for _, want := range []string{
		"## Catalog snapshot",
		"Installed catalog is separate from turn-visible tools for this request.",
		"system: /home/demo/.sesame/skills/.system",
		"global: /home/demo/.sesame/skills",
		"workspace: /workspace/.sesame/skills",
		"agent-browser [global]: Browser automation helper. (/home/demo/.sesame/skills/agent-browser)",
		"skill-normalizer [system]: Normalize third-party skills. (/home/demo/.sesame/skills/.system/skill-normalizer)",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("Render() missing %q:\n%s", want, rendered)
		}
	}
	for _, unwanted := range []string{
		"which playwright",
		"which browser-cli",
	} {
		if strings.Contains(rendered, unwanted) {
			t.Fatalf("Render() unexpectedly included %q:\n%s", unwanted, rendered)
		}
	}
}

func TestCompileRendersCompactActivatedSkillWhenFullInjectionDisabled(t *testing.T) {
	catalog := skills.Catalog{
		Skills: []skills.SkillSpec{
			{
				Name:        "agent-browser",
				Scope:       "global",
				Description: "Browser automation helper for clicking and screenshots.",
				Body:        "npm i -g agent-browser\nagent-browser open https://example.com",
				Policy: skills.SkillPolicy{
					AllowFullInjection: false,
				},
			},
		},
	}
	activeSkills := []skills.ActivatedSkill{
		{
			Skill:  catalog.Skills[0],
			Reason: skills.ActivationReasonRetrieved,
		},
	}

	bundle := Compile(CompileInput{
		BaseText: "Base system text.",
		Catalog:  catalog,
		Message:  "打开 https://example.com 并点击登录按钮",
		Policy: toolrouter.PolicySummary{
			Profile: toolrouter.ProfileBrowserAutomation,
		},
		VisibleTools: []string{"shell_command", "skill_use", "web_fetch"},
		ActiveSkills: activeSkills,
	})
	rendered := bundle.Render()

	for _, want := range []string{
		"## Activated skills",
		"agent-browser",
		"Summary: Browser automation helper for clicking and screenshots.",
		"Use `skill_use` with its name if you need the full local instructions.",
		"Granted tools: shell_command",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("Render() missing %q:\n%s", want, rendered)
		}
	}
	for _, unwanted := range []string{
		"npm i -g agent-browser",
		"agent-browser open https://example.com",
	} {
		if strings.Contains(rendered, unwanted) {
			t.Fatalf("Render() unexpectedly included %q:\n%s", unwanted, rendered)
		}
	}
}

func TestCompileSkipsIrrelevantSkillsForGenericNewsTask(t *testing.T) {
	catalog := skills.Catalog{
		Skills: []skills.SkillSpec{
			{
				Name:        "agent-browser",
				Scope:       "global",
				Description: "Browser automation helper for clicking and screenshots.",
				Policy: skills.SkillPolicy{
					AllowImplicitActivation: true,
				},
			},
			{
				Name:        "send-email",
				Scope:       "global",
				Description: "Send emails via SMTP.",
			},
		},
	}

	bundle := Compile(CompileInput{
		BaseText: "Base system text.",
		Catalog:  catalog,
		Message:  "帮我查今天热门新闻",
		Policy: toolrouter.PolicySummary{
			Profile: toolrouter.ProfileWebLookup,
		},
		VisibleTools: []string{"skill_use", "web_fetch"},
	})
	rendered := bundle.Render()

	for _, unwanted := range []string{
		"agent-browser",
		"send-email",
		"## Implicit skill hints",
		"## Relevant skills",
	} {
		if strings.Contains(rendered, unwanted) {
			t.Fatalf("Render() unexpectedly included %q:\n%s", unwanted, rendered)
		}
	}
}
