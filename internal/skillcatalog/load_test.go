package skillcatalog

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestLoadCatalogLoadsSkillDirsAndFrontMatter(t *testing.T) {
	globalRoot := t.TempDir()
	workspaceRoot := t.TempDir()
	systemDir := filepath.Join(globalRoot, "system", "skills", "system-skill")
	globalDir := filepath.Join(globalRoot, "skills")
	workspaceDir := filepath.Join(workspaceRoot, "skills", "workspace-skill")

	writeSkill(t, filepath.Join(systemDir, "SKILL.md"), `---
id: system-skill
name: system-skill
version: 1.2.3
description: System skill.
scope: system
requires_tools:
  - shell
  - file_read
allowed-tools: shell, file_read
risk_level: medium
approval_required: true
prompt_file: prompt.md
examples:
  - examples/runbook.md
tests:
  - tests/dry-run.md
permissions:
  network: false
  delivery: draft_only
when-to-use:
  - system trigger
policy:
  allow_implicit_activation: true
agent:
  type: explorer
  tools:
    - rg
---
System body.
`)
	writeSkill(t, filepath.Join(systemDir, "prompt.md"), "Prompt file body.\n")
	writeSkill(t, filepath.Join(globalDir, "global.md"), `---
description: Global skill.
allowed_tools:
  - file_write
triggers: global trigger
---
Global body.
`)
	writeSkill(t, filepath.Join(workspaceDir, "SKILL.md"), "\ufeff---\nname: workspace-skill\nwhen-to-use: workspace trigger\n---\nWorkspace body.\n")

	catalog, err := LoadCatalog(globalRoot, workspaceRoot)
	if err != nil {
		t.Fatalf("LoadCatalog: %v", err)
	}

	if got, want := catalog.SkillNames(), []string{"global", "system-skill", "workspace-skill"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("SkillNames = %#v, want %#v", got, want)
	}
	system := catalog.Skills[1]
	if system.Scope != "system" || system.Description != "System skill." || system.Body != "System body." {
		t.Fatalf("unexpected system skill: %+v", system)
	}
	if system.ID != "system-skill" || system.Version != "1.2.3" || system.ManifestScope != "system" {
		t.Fatalf("manifest fields not loaded: %+v", system)
	}
	if got, want := system.AllowedTools, []string{"shell", "file_read"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("AllowedTools = %#v, want %#v", got, want)
	}
	if got, want := system.RequiresTools, []string{"shell", "file_read"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("RequiresTools = %#v, want %#v", got, want)
	}
	if !system.ApprovalRequired || system.RiskLevel != "medium" || system.PromptFile != "prompt.md" {
		t.Fatalf("risk/approval/prompt not loaded: %+v", system)
	}
	if got, want := system.Examples, []string{"examples/runbook.md"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Examples = %#v, want %#v", got, want)
	}
	if got, want := system.Tests, []string{"tests/dry-run.md"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Tests = %#v, want %#v", got, want)
	}
	if got, ok := system.Permissions["network"].(bool); !ok || got {
		t.Fatalf("expected boolean permission, got %+v", system.Permissions)
	}
	if got, ok := system.Permissions["delivery"].(string); !ok || got != "draft_only" {
		t.Fatalf("expected string permission, got %+v", system.Permissions)
	}
	if !system.Policy.AllowImplicitActivation || system.Agent.Type != "explorer" {
		t.Fatalf("front matter policy/agent not loaded: %+v", system)
	}

	workspace := catalog.Skills[2]
	if workspace.Body != "Workspace body." || !reflect.DeepEqual(workspace.Triggers, []string{"workspace trigger"}) {
		t.Fatalf("workspace skill not loaded from BOM/front matter: %+v", workspace)
	}
}

func TestLoadCatalogIgnoresMissingDirs(t *testing.T) {
	catalog, err := LoadCatalog(filepath.Join(t.TempDir(), "missing-global"), filepath.Join(t.TempDir(), "missing-workspace"))
	if err != nil {
		t.Fatalf("LoadCatalog missing dirs: %v", err)
	}
	if len(catalog.Skills) != 0 {
		t.Fatalf("skills = %+v, want empty", catalog.Skills)
	}
}

func writeSkill(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}
