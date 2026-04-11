package instructions

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go-agent/internal/skills"
)

func TestCompileRendersCatalogOnlyUntilSkillIsActivated(t *testing.T) {
	globalRoot := t.TempDir()
	workspaceRoot := t.TempDir()
	skillDir := filepath.Join(globalRoot, "skills", "compiler-test")

	writeSkillFile(t, filepath.Join(skillDir, "SKILL.json"), `{
		"name": "compiler-test",
		"description": "compiler behavior test",
		"tool_dependencies": ["functions.shell_command"]
	}`)
	writeSkillFile(t, filepath.Join(skillDir, "SKILL.md"), "compiler body")

	catalog, err := skills.LoadCatalog(globalRoot, workspaceRoot)
	if err != nil {
		t.Fatalf("skills.LoadCatalog() error = %v", err)
	}

	startBundle, err := Compile(CompileInput{Catalog: catalog})
	if err != nil {
		t.Fatalf("Compile(start) error = %v", err)
	}
	startPrompt := startBundle.Render()
	if !strings.Contains(startPrompt, "Installed local skills:") {
		t.Fatalf("start prompt missing catalog section: %q", startPrompt)
	}
	if !strings.Contains(startPrompt, "skill_use") {
		t.Fatalf("start prompt missing skill_use hint: %q", startPrompt)
	}
	if strings.Contains(startPrompt, "compiler body") {
		t.Fatalf("start prompt unexpectedly contains active skill body: %q", startPrompt)
	}

	spec, ok := catalog.FindByName("compiler-test")
	if !ok {
		t.Fatalf("catalog.FindByName(%q) ok = false", "compiler-test")
	}
	active := []skills.ActivatedSkill{{Skill: spec}}

	activeBundle, err := Compile(CompileInput{
		Catalog:         catalog,
		Active:          active,
		NewlyActivated:  active,
		PreviouslyTools: nil,
	})
	if err != nil {
		t.Fatalf("Compile(active) error = %v", err)
	}
	activePrompt := activeBundle.Render()
	if strings.Contains(activePrompt, "Installed local skills:") {
		t.Fatalf("active prompt unexpectedly contains catalog section: %q", activePrompt)
	}
	if !strings.Contains(activePrompt, "compiler body") {
		t.Fatalf("active prompt missing skill body: %q", activePrompt)
	}
	if !strings.Contains(activePrompt, "functions.shell_command") {
		t.Fatalf("active prompt missing newly enabled tool: %q", activePrompt)
	}
}

func writeSkillFile(t *testing.T, path string, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", path, err)
	}
}
