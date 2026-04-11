package skills

import "testing"

func TestRetrieveSelectsSendEmailForCompoundTask(t *testing.T) {
	catalog := Catalog{
		Skills: []SkillSpec{
			{
				Name:        "send-email",
				Description: "Send emails via SMTP.",
				AllowedTools: []string{
					"shell_command",
				},
			},
			{
				Name:        "agent-browser",
				Description: "Browser automation helper for websites and clicking buttons.",
				Policy: SkillPolicy{
					AllowImplicitActivation: true,
				},
			},
		},
	}

	result := Retrieve(catalog, "帮我查今天合肥天气，然后发邮件给我", nil)
	if len(result.Selected) != 1 {
		t.Fatalf("len(result.Selected) = %d, want 1", len(result.Selected))
	}
	if result.Selected[0].Skill.Name != "send-email" {
		t.Fatalf("selected skill = %q, want send-email", result.Selected[0].Skill.Name)
	}
	if result.Selected[0].Reason != ActivationReasonRetrieved {
		t.Fatalf("reason = %q, want %q", result.Selected[0].Reason, ActivationReasonRetrieved)
	}
}

func TestRetrieveSelectsBrowserSkillForExplicitBrowserTask(t *testing.T) {
	catalog := Catalog{
		Skills: []SkillSpec{
			{
				Name:        "agent-browser",
				Description: "Browser automation helper for websites, clicks, forms, login, and screenshots.",
				Policy: SkillPolicy{
					AllowImplicitActivation: true,
					AllowFullInjection:      false,
				},
				AllowedTools: []string{"shell_command"},
			},
		},
	}

	result := Retrieve(catalog, "打开 https://example.com 并点击登录按钮", nil)
	if len(result.Selected) != 1 {
		t.Fatalf("len(result.Selected) = %d, want 1", len(result.Selected))
	}
	if result.Selected[0].Skill.Name != "agent-browser" {
		t.Fatalf("selected skill = %q, want agent-browser", result.Selected[0].Skill.Name)
	}
}

func TestRetrieveSkipsBrowserSkillForGenericNewsTask(t *testing.T) {
	catalog := Catalog{
		Skills: []SkillSpec{
			{
				Name:        "agent-browser",
				Description: "Browser automation helper for websites, clicks, forms, login, and screenshots.",
				Policy: SkillPolicy{
					AllowImplicitActivation: true,
				},
			},
		},
	}

	result := Retrieve(catalog, "帮我查今天热门新闻", nil)
	if len(result.Selected) != 0 {
		t.Fatalf("len(result.Selected) = %d, want 0", len(result.Selected))
	}
	if len(result.Suggested) != 0 {
		t.Fatalf("len(result.Suggested) = %d, want 0", len(result.Suggested))
	}
}

func TestRetrieveSkipsAutoActivationForScheduledTask(t *testing.T) {
	catalog := Catalog{
		Skills: []SkillSpec{
			{
				Name:        "send-email",
				Description: "Send emails via SMTP.",
				AllowedTools: []string{
					"shell_command",
				},
			},
		},
	}

	result := Retrieve(catalog, "两分钟后给我发一封邮件，内容是今天合肥天气", nil)
	if len(result.Selected) != 0 {
		t.Fatalf("len(result.Selected) = %d, want 0", len(result.Selected))
	}
	if !result.Task.WantsScheduling {
		t.Fatal("task should be classified as scheduled")
	}
}

func TestRetrieveForExecutionSelectsSendEmailForDetailedWeatherPrompt(t *testing.T) {
	catalog := Catalog{
		Skills: []SkillSpec{
			{
				Name:        "send-email",
				Description: "Send emails via SMTP. Configure in ~/.sesame/config.json under skills.entries.send-email.env.",
				Body:        "SMTP reference: requires authorization code (not login password).",
			},
			{
				Name:        "agent-browser",
				Description: "Browser automation helper for websites and clicking buttons.",
				Policy: SkillPolicy{
					AllowImplicitActivation: true,
				},
			},
		},
	}

	result := RetrieveForExecution(
		catalog,
		"查询合肥市的当前天气信息，包括温度、天气状况、湿度、风向风速等详细数据，然后将这些信息发送到邮箱 ssuce885@gmail.com",
		nil,
	)
	if len(result.Selected) != 1 {
		t.Fatalf("len(result.Selected) = %d, want 1 (selected=%v suggested=%v)", len(result.Selected), result.Selected, result.Suggested)
	}
	if result.Selected[0].Skill.Name != "send-email" {
		t.Fatalf("selected skill = %q, want send-email", result.Selected[0].Skill.Name)
	}
}
