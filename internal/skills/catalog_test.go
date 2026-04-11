package skills

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestLoadCatalogReadsSkillBodyOnDemand(t *testing.T) {
	globalRoot := t.TempDir()
	workspaceRoot := t.TempDir()
	skillDir := filepath.Join(globalRoot, "skills", "runtime-test")

	writeSkillFile(t, filepath.Join(skillDir, "SKILL.json"), `{
		"name": "runtime-test",
		"description": "runtime test skill"
	}`)
	writeSkillFile(t, filepath.Join(skillDir, "SKILL.md"), "initial body")

	catalog, err := LoadCatalog(globalRoot, workspaceRoot)
	if err != nil {
		t.Fatalf("LoadCatalog() error = %v", err)
	}

	skill, ok := catalog.FindByName("runtime-test")
	if !ok {
		t.Fatalf("catalog.FindByName(%q) ok = false", "runtime-test")
	}

	writeSkillFile(t, filepath.Join(skillDir, "SKILL.md"), "updated body")

	body, err := catalog.ReadBody(skill)
	if err != nil {
		t.Fatalf("catalog.ReadBody() error = %v", err)
	}
	if body != "updated body" {
		t.Fatalf("catalog.ReadBody() = %q, want %q", body, "updated body")
	}
}

func TestMergeActiveIsIdempotentBySkillName(t *testing.T) {
	existing := []ActivatedSkill{
		{Skill: SkillSpec{Name: "alpha"}, Body: "alpha body"},
	}
	incoming := []ActivatedSkill{
		{Skill: SkillSpec{Name: "alpha"}, Body: "new alpha body"},
		{Skill: SkillSpec{Name: "beta"}, Body: "beta body"},
	}

	merged := MergeActive(existing, incoming...)
	if got, want := ActiveSkillNames(merged), []string{"alpha", "beta"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("ActiveSkillNames(first merge) = %v, want %v", got, want)
	}

	mergedAgain := MergeActive(merged, incoming...)
	if got, want := ActiveSkillNames(mergedAgain), []string{"alpha", "beta"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("ActiveSkillNames(second merge) = %v, want %v", got, want)
	}
	if mergedAgain[0].Body != "alpha body" {
		t.Fatalf("mergedAgain[0].Body = %q, want %q", mergedAgain[0].Body, "alpha body")
	}
}

func TestActivateByNamesLoadsBodyAndIsIdempotent(t *testing.T) {
	globalRoot := t.TempDir()
	workspaceRoot := t.TempDir()
	skillDir := filepath.Join(globalRoot, "skills", "activate-test")
	writeSkillFile(t, filepath.Join(skillDir, "SKILL.json"), `{
		"name": "activate-test",
		"description": "activate test skill"
	}`)
	writeSkillFile(t, filepath.Join(skillDir, "SKILL.md"), "activate body")

	catalog, err := LoadCatalog(globalRoot, workspaceRoot)
	if err != nil {
		t.Fatalf("LoadCatalog() error = %v", err)
	}

	active, err := ActivateByNames(catalog, []string{"activate-test", "activate-test"})
	if err != nil {
		t.Fatalf("ActivateByNames() error = %v", err)
	}
	if got, want := ActiveSkillNames(active), []string{"activate-test"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("ActiveSkillNames(active) = %v, want %v", got, want)
	}
	if active[0].Body != "activate body" {
		t.Fatalf("active[0].Body = %q, want %q", active[0].Body, "activate body")
	}
}

func TestActivateByNamesFailsWhenBodyCannotBeRead(t *testing.T) {
	globalRoot := t.TempDir()
	workspaceRoot := t.TempDir()
	skillDir := filepath.Join(globalRoot, "skills", "broken-skill")
	writeSkillFile(t, filepath.Join(skillDir, "SKILL.json"), `{
		"name": "broken-skill",
		"description": "broken skill"
	}`)
	writeSkillFile(t, filepath.Join(skillDir, "SKILL.md"), "body before deletion")

	catalog, err := LoadCatalog(globalRoot, workspaceRoot)
	if err != nil {
		t.Fatalf("LoadCatalog() error = %v", err)
	}

	if err := os.Remove(filepath.Join(skillDir, "SKILL.md")); err != nil {
		t.Fatalf("Remove(SKILL.md) error = %v", err)
	}

	if _, err := ActivateByNames(catalog, []string{"broken-skill"}); err == nil {
		t.Fatalf("ActivateByNames() error = nil, want non-nil")
	}
}

func TestLoadCatalogPreservesExactDependencySpellings(t *testing.T) {
	globalRoot := t.TempDir()
	workspaceRoot := t.TempDir()
	skillDir := filepath.Join(globalRoot, "skills", "exact-deps")

	writeSkillFile(t, filepath.Join(skillDir, "SKILL.json"), `{
		"name": "exact-deps",
		"description": "preserve exact dependency spellings",
		"tool_dependencies": ["shell_command", "Shell_Command"],
		"preferred_tools": ["apply_patch", "Apply_Patch"],
		"env_dependencies": ["OPENAI_API_KEY", "OpenAI_Api_Key"]
	}`)
	writeSkillFile(t, filepath.Join(skillDir, "SKILL.md"), "exact deps body")

	catalog, err := LoadCatalog(globalRoot, workspaceRoot)
	if err != nil {
		t.Fatalf("LoadCatalog() error = %v", err)
	}

	skill, ok := catalog.FindByName("exact-deps")
	if !ok {
		t.Fatalf("catalog.FindByName(%q) ok = false", "exact-deps")
	}

	if got, want := skill.ToolDependencies, []string{"shell_command", "Shell_Command"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("skill.ToolDependencies = %v, want %v", got, want)
	}
	if got, want := skill.PreferredTools, []string{"apply_patch", "Apply_Patch"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("skill.PreferredTools = %v, want %v", got, want)
	}
	if got, want := skill.EnvDependencies, []string{"OPENAI_API_KEY", "OpenAI_Api_Key"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("skill.EnvDependencies = %v, want %v", got, want)
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
