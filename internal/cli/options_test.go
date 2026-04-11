package cli

import "testing"

func TestParseOptionsSeparatesPromptFromFlags(t *testing.T) {
	opts, err := ParseOptions([]string{"--status", "check daemon"})
	if err != nil {
		t.Fatalf("ParseOptions() error = %v", err)
	}

	if !opts.ShowStatus {
		t.Fatal("ShowStatus = false, want true")
	}
	if opts.InitialPrompt != "check daemon" {
		t.Fatalf("InitialPrompt = %q, want %q", opts.InitialPrompt, "check daemon")
	}
}

func TestParseOptionsCapturesStartupOverrides(t *testing.T) {
	opts, err := ParseOptions([]string{
		"--resume", "sess_123",
		"--daemon", "latest",
		"--list-daemons",
		"--print",
		"--data-dir", "E:/tmp/agentd",
		"--model", "gpt-5.4",
		"--permission-mode", "trusted_local",
		"hello there",
	})
	if err != nil {
		t.Fatalf("ParseOptions() error = %v", err)
	}

	if opts.ResumeID != "sess_123" {
		t.Fatalf("ResumeID = %q, want %q", opts.ResumeID, "sess_123")
	}
	if opts.DaemonRef != "latest" {
		t.Fatalf("DaemonRef = %q, want %q", opts.DaemonRef, "latest")
	}
	if !opts.ListDaemons {
		t.Fatal("ListDaemons = false, want true")
	}
	if !opts.PrintOnly {
		t.Fatal("PrintOnly = false, want true")
	}
	if opts.DataDir != "E:/tmp/agentd" {
		t.Fatalf("DataDir = %q, want %q", opts.DataDir, "E:/tmp/agentd")
	}
	if opts.Model != "gpt-5.4" {
		t.Fatalf("Model = %q, want %q", opts.Model, "gpt-5.4")
	}
	if opts.PermissionMode != "trusted_local" {
		t.Fatalf("PermissionMode = %q, want %q", opts.PermissionMode, "trusted_local")
	}
	if opts.InitialPrompt != "hello there" {
		t.Fatalf("InitialPrompt = %q, want %q", opts.InitialPrompt, "hello there")
	}
}

func TestParseOptionsSkillInstallCommand(t *testing.T) {
	opts, err := ParseOptions([]string{"skill", "install", "openai/skills", "--path", "skills/.curated/parallel", "--scope", "workspace", "--ref", "main"})
	if err != nil {
		t.Fatalf("ParseOptions() error = %v", err)
	}
	if opts.Skill == nil {
		t.Fatal("Skill = nil, want parsed skill command")
	}
	if opts.Skill.Action != "install" {
		t.Fatalf("Action = %q, want install", opts.Skill.Action)
	}
	if opts.Skill.Source != "openai/skills" {
		t.Fatalf("Source = %q, want openai/skills", opts.Skill.Source)
	}
	if opts.Skill.Path != "skills/.curated/parallel" {
		t.Fatalf("Path = %q, want skills/.curated/parallel", opts.Skill.Path)
	}
	if opts.Skill.Scope != "workspace" {
		t.Fatalf("Scope = %q, want workspace", opts.Skill.Scope)
	}
}

func TestParseOptionsSkillListRemoteCommand(t *testing.T) {
	opts, err := ParseOptions([]string{"skill", "list", "--repo", "openai/skills", "--path", "skills/.curated"})
	if err != nil {
		t.Fatalf("ParseOptions() error = %v", err)
	}
	if opts.Skill == nil {
		t.Fatal("Skill = nil, want parsed skill command")
	}
	if opts.Skill.Action != "list" {
		t.Fatalf("Action = %q, want list", opts.Skill.Action)
	}
	if opts.Skill.Repo != "openai/skills" {
		t.Fatalf("Repo = %q, want openai/skills", opts.Skill.Repo)
	}
	if opts.Skill.Path != "skills/.curated" {
		t.Fatalf("Path = %q, want skills/.curated", opts.Skill.Path)
	}
}

func TestParseOptionsSkillListRemoteCommandRequiresRepoAndPathTogether(t *testing.T) {
	if _, err := ParseOptions([]string{"skill", "list", "--repo", "openai/skills"}); err == nil {
		t.Fatal("ParseOptions() error = nil, want usage error when --path is missing")
	}
	if _, err := ParseOptions([]string{"skill", "list", "--path", "skills/.curated"}); err == nil {
		t.Fatal("ParseOptions() error = nil, want usage error when --repo is missing")
	}
}

func TestParseOptionsSkillInspectCommand(t *testing.T) {
	opts, err := ParseOptions([]string{"skill", "inspect", "https://github.com/openai/skills", "--scope", "workspace"})
	if err != nil {
		t.Fatalf("ParseOptions() error = %v", err)
	}
	if opts.Skill == nil {
		t.Fatal("Skill = nil, want parsed skill command")
	}
	if opts.Skill.Action != "inspect" {
		t.Fatalf("Action = %q, want inspect", opts.Skill.Action)
	}
	if opts.Skill.Source != "https://github.com/openai/skills" {
		t.Fatalf("Source = %q, want repo URL", opts.Skill.Source)
	}
	if opts.Skill.Scope != "workspace" {
		t.Fatalf("Scope = %q, want workspace", opts.Skill.Scope)
	}
}

func TestParseOptionsSkillInstallRepoRootWithoutPath(t *testing.T) {
	opts, err := ParseOptions([]string{"skill", "install", "https://github.com/openai/skills"})
	if err != nil {
		t.Fatalf("ParseOptions() error = %v", err)
	}
	if opts.Skill == nil {
		t.Fatal("Skill = nil, want parsed skill command")
	}
	if opts.Skill.Action != "install" {
		t.Fatalf("Action = %q, want install", opts.Skill.Action)
	}
	if opts.Skill.Source != "https://github.com/openai/skills" {
		t.Fatalf("Source = %q, want repo URL", opts.Skill.Source)
	}
	if opts.Skill.Path != "" {
		t.Fatalf("Path = %q, want empty", opts.Skill.Path)
	}
}
