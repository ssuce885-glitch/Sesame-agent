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
