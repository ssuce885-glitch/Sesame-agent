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
		{Skill: SkillSpec{Name: "alpha"}},
	}
	incoming := []ActivatedSkill{
		{Skill: SkillSpec{Name: "alpha"}},
		{Skill: SkillSpec{Name: "beta"}},
	}

	merged := MergeActive(existing, incoming...)
	if got, want := ActiveSkillNames(merged), []string{"alpha", "beta"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("ActiveSkillNames(first merge) = %v, want %v", got, want)
	}

	mergedAgain := MergeActive(merged, incoming...)
	if got, want := ActiveSkillNames(mergedAgain), []string{"alpha", "beta"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("ActiveSkillNames(second merge) = %v, want %v", got, want)
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
