package extensions

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDiscoverRequiresSkillJSONAndMarkdown(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "skills", "legacy-only", "SKILL.md"), "# Legacy")

	catalog, err := Discover(root, t.TempDir())
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	if len(catalog.Skills) != 0 {
		t.Fatalf("len(catalog.Skills) = %d, want 0", len(catalog.Skills))
	}
}

func TestDiscoverExcludesDisabledSkillFromCatalog(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "skills", "disabled", "SKILL.md"), "# Disabled")
	writeFile(t, filepath.Join(root, "skills", "disabled", "SKILL.json"), `{
		"name": "disabled",
		"description": "hidden",
		"enabled": false
	}`)

	catalog, err := Discover(root, t.TempDir())
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	if len(catalog.Skills) != 0 {
		t.Fatalf("len(catalog.Skills) = %d, want 0", len(catalog.Skills))
	}
}

func writeFile(t *testing.T, path string, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) error = %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", path, err)
	}
}
