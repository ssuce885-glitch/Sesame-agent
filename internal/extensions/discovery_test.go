package extensions

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadCatalogSeedsBundledSystemSkills(t *testing.T) {
	home := t.TempDir()
	workspace := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	globalRoot := filepath.Join(home, ".sesame")
	catalog, err := LoadCatalog(globalRoot, workspace)
	if err != nil {
		t.Fatalf("LoadCatalog() error = %v", err)
	}

	for _, name := range []string{"skill-installer", "skill-normalizer"} {
		skillPath := filepath.Join(globalRoot, "skills", ".system", name, "SKILL.md")
		if _, err := os.Stat(skillPath); err != nil {
			t.Fatalf("system skill %q was not seeded: %v", name, err)
		}
	}

	normalizer, ok := findSkillByName(catalog.Skills, "skill-normalizer")
	if !ok {
		t.Fatalf("catalog missing bundled system skill %q", "skill-normalizer")
	}
	if normalizer.Scope != ScopeSystem {
		t.Fatalf("normalizer scope = %q, want %q", normalizer.Scope, ScopeSystem)
	}
	if normalizer.Policy.AllowImplicitActivation {
		t.Fatal("normalizer allow_implicit_activation = true, want false")
	}
	if !normalizer.Policy.AllowFullInjection {
		t.Fatal("normalizer allow_full_injection = false, want true")
	}
	if got := normalizer.Policy.PreferredTools; len(got) == 0 {
		t.Fatal("normalizer preferred_tools empty, want seeded values")
	}
	if !strings.Contains(normalizer.Body, "Do not enable `allow_implicit_activation` by default") {
		t.Fatalf("normalizer body missing normalization rule, got %q", normalizer.Body)
	}
}

func TestDiscoverPrefersWorkspaceSkillsAndFindsTools(t *testing.T) {
	home := t.TempDir()
	workspace := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	writeFile(t, filepath.Join(home, ".sesame", "skills", "demo", "SKILL.md"), "---\nname: demo\ndescription: global skill\n---\nGlobal body")
	writeFile(t, filepath.Join(workspace, ".sesame", "skills", "demo", "SKILL.md"), "---\nname: demo\ndescription: workspace skill\n---\nWorkspace body")
	writeFile(t, filepath.Join(workspace, ".sesame", "tools", "build", "tool.json"), `{
  "name": "build_tool",
  "description": "workspace build tool",
  "command": "./run.sh",
  "input_schema": {
    "type": "object",
    "properties": {},
    "additionalProperties": false
  }
}`)

	catalog, err := Discover(filepath.Join(home, ".sesame"), workspace)
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	if len(catalog.Skills) != 1 {
		t.Fatalf("len(Skills) = %d, want 1", len(catalog.Skills))
	}
	if catalog.Skills[0].Scope != "workspace" {
		t.Fatalf("skill scope = %q, want workspace", catalog.Skills[0].Scope)
	}
	if catalog.Skills[0].Description != "workspace skill" {
		t.Fatalf("skill description = %q, want workspace override", catalog.Skills[0].Description)
	}
	if len(catalog.Tools) != 1 {
		t.Fatalf("len(Tools) = %d, want 1", len(catalog.Tools))
	}
	if catalog.Tools[0].Name != "build_tool" {
		t.Fatalf("tool name = %q, want build_tool", catalog.Tools[0].Name)
	}
	if catalog.Tools[0].Description != "workspace build tool" {
		t.Fatalf("tool description = %q, want workspace build tool", catalog.Tools[0].Description)
	}
}

func TestInvalidateCatalogCacheReloadsWorkspaceSkills(t *testing.T) {
	home := t.TempDir()
	workspace := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	globalRoot := filepath.Join(home, ".sesame")
	catalog, err := LoadCatalog(globalRoot, workspace)
	if err != nil {
		t.Fatalf("LoadCatalog() error = %v", err)
	}
	if _, ok := findSkillByName(catalog.Skills, "dynamic-skill"); ok {
		t.Fatal("unexpected dynamic-skill before install")
	}

	writeFile(t, filepath.Join(workspace, ".sesame", "skills", "dynamic-skill", "SKILL.md"), "---\nname: dynamic-skill\ndescription: hot loaded\n---\nDynamic body")

	InvalidateCatalogCache(globalRoot, workspace)

	reloaded, err := LoadCatalog(globalRoot, workspace)
	if err != nil {
		t.Fatalf("LoadCatalog() after invalidation error = %v", err)
	}
	skill, ok := findSkillByName(reloaded.Skills, "dynamic-skill")
	if !ok {
		t.Fatal("reloaded catalog missing dynamic-skill")
	}
	if skill.Scope != ScopeWorkspace {
		t.Fatalf("skill scope = %q, want %q", skill.Scope, ScopeWorkspace)
	}
}

func TestLoadCatalogDetectsWorkspaceSkillAddedBetweenTurns(t *testing.T) {
	home := t.TempDir()
	workspace := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	globalRoot := filepath.Join(home, ".sesame")
	initial, err := LoadCatalog(globalRoot, workspace)
	if err != nil {
		t.Fatalf("LoadCatalog() error = %v", err)
	}
	if _, ok := findSkillByName(initial.Skills, "between-turns"); ok {
		t.Fatal("unexpected between-turns skill before file drop")
	}

	writeFile(t, filepath.Join(workspace, ".sesame", "skills", "between-turns", "SKILL.md"), "---\nname: between-turns\ndescription: loaded on next turn\n---\nBetween turn body")

	reloaded, err := LoadCatalog(globalRoot, workspace)
	if err != nil {
		t.Fatalf("LoadCatalog() after file drop error = %v", err)
	}
	skill, ok := findSkillByName(reloaded.Skills, "between-turns")
	if !ok {
		t.Fatal("reloaded catalog missing between-turns skill")
	}
	if skill.Description != "loaded on next turn" {
		t.Fatalf("skill description = %q, want %q", skill.Description, "loaded on next turn")
	}
}

func TestParseSkillDocumentSupportsStructuredYAMLFormat(t *testing.T) {
	parsed := parseSkillDocument("system-info", `name: system-info
description: 查询系统配置工具
triggers:
  - "查询系统配置"
  - "查看系统信息"
policy:
  allow_implicit_activation: true
  capability_tags:
    - system_inspect
  preferred_tools:
    - shell_command
agent:
  type: child
  description: 系统信息子代理
  instructions: |
    使用 shell_command 收集系统信息。
  tools:
    - shell_command
`)

	if parsed.Name != "system-info" {
		t.Fatalf("Name = %q, want system-info", parsed.Name)
	}
	if parsed.Description != "查询系统配置工具" {
		t.Fatalf("Description = %q, want structured description", parsed.Description)
	}
	if len(parsed.Triggers) != 2 {
		t.Fatalf("len(Triggers) = %d, want 2", len(parsed.Triggers))
	}
	if len(parsed.AllowedTools) != 0 {
		t.Fatalf("len(AllowedTools) = %d, want 0", len(parsed.AllowedTools))
	}
	if !parsed.Policy.AllowImplicitActivation {
		t.Fatal("Policy.AllowImplicitActivation = false, want true")
	}
	if len(parsed.Policy.CapabilityTags) != 1 || parsed.Policy.CapabilityTags[0] != "system_inspect" {
		t.Fatalf("Policy.CapabilityTags = %v, want [system_inspect]", parsed.Policy.CapabilityTags)
	}
	if len(parsed.Policy.PreferredTools) != 1 || parsed.Policy.PreferredTools[0] != "shell_command" {
		t.Fatalf("Policy.PreferredTools = %v, want [shell_command]", parsed.Policy.PreferredTools)
	}
	if parsed.Agent.Type != "child" {
		t.Fatalf("Agent.Type = %q, want child", parsed.Agent.Type)
	}
	if len(parsed.Agent.Tools) != 1 || parsed.Agent.Tools[0] != "shell_command" {
		t.Fatalf("Agent.Tools = %v, want [shell_command]", parsed.Agent.Tools)
	}
	if !strings.Contains(parsed.Body, "Preferred tools: shell_command") {
		t.Fatalf("Body = %q, want preferred tools summary", parsed.Body)
	}
	if !strings.Contains(parsed.Body, "使用 shell_command 收集系统信息。") {
		t.Fatalf("Body = %q, want embedded instructions", parsed.Body)
	}
}

func writeFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
}

func findSkillByName(skills []Skill, name string) (Skill, bool) {
	for _, skill := range skills {
		if strings.EqualFold(skill.Name, name) {
			return skill, true
		}
	}
	return Skill{}, false
}
