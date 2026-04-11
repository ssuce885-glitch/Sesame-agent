package extensions

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestInspectSkillSourceRejectsLegacySingleFileSkill(t *testing.T) {
	source := t.TempDir()
	writeFile(t, filepath.Join(source, "SKILL.md"), "# Legacy")

	_, err := InspectSkillSource(t.TempDir(), t.TempDir(), InstallRequest{Source: source})
	if err == nil {
		t.Fatal("InspectSkillSource() error = nil, want dual-file validation error")
	}
	if !strings.Contains(err.Error(), "SKILL.json") {
		t.Fatalf("error = %v, want missing SKILL.json message", err)
	}
}

func TestInspectSkillSourceRejectsRemoteExplicitPathWithoutSkillJSON(t *testing.T) {
	t.Cleanup(func() {
		githubRequestFunc = githubRequest
	})
	githubRequestFunc = func(rawURL, accept string) ([]byte, error) {
		if rawURL != "https://api.github.com/repos/example/skills/contents/skills/legacy-only?ref=main" {
			t.Fatalf("githubRequestFunc() url = %q, want remote explicit path contents URL", rawURL)
		}
		if accept != "application/vnd.github+json" {
			t.Fatalf("githubRequestFunc() accept = %q, want application/vnd.github+json", accept)
		}
		return []byte(`[
			{"name":"SKILL.md","type":"file"}
		]`), nil
	}

	_, err := InspectSkillSource(t.TempDir(), t.TempDir(), InstallRequest{
		Source: "example/skills",
		Path:   "skills/legacy-only",
	})
	if err == nil {
		t.Fatal("InspectSkillSource() error = nil, want dual-file validation error")
	}
	if !strings.Contains(err.Error(), "SKILL.json") {
		t.Fatalf("error = %v, want missing SKILL.json message", err)
	}
}
