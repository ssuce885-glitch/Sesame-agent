package skillcatalog

import (
	"archive/zip"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

func TestTestSkillFileValidatesExamplesAndTestsAssets(t *testing.T) {
	root := t.TempDir()
	skillPath := filepath.Join(root, "distribution-check", "SKILL.md")
	writeSkill(t, skillPath, `---
id: distribution-check
description: Validate packaged example and test assets.
requires_tools:
  - shell
risk_level: low
examples:
  - examples/sample.md
tests:
  - tests/case.md
---
Body.
`)
	writeSkill(t, filepath.Join(root, "distribution-check", "examples", "sample.md"), "example asset\n")

	findings, err := TestSkillFile(skillPath, []string{"shell"})
	if err != nil {
		t.Fatalf("TestSkillFile: %v", err)
	}
	if !containsString(findingMessages(findings), `tests: tests entry "tests/case.md" does not exist`) {
		t.Fatalf("expected missing tests asset finding, got %#v", findingMessages(findings))
	}
}

func TestTestSkillFileRejectsExamplesSymlink(t *testing.T) {
	root := t.TempDir()
	skillDir := filepath.Join(root, "examples-symlink")
	skillPath := filepath.Join(skillDir, "SKILL.md")
	writeSkill(t, skillPath, `---
id: examples-symlink
description: Example assets must not be symlinks.
requires_tools:
  - shell
risk_level: low
examples:
  - examples/sample.md
---
Body.
`)
	writeSkill(t, filepath.Join(skillDir, "examples", "sample-target.md"), "example asset\n")
	if err := os.Symlink("sample-target.md", filepath.Join(skillDir, "examples", "sample.md")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	findings, err := TestSkillFile(skillPath, []string{"shell"})
	if err != nil {
		t.Fatalf("TestSkillFile: %v", err)
	}
	if !containsString(findingMessages(findings), `examples: examples entry "examples/sample.md" must not be a symlink`) {
		t.Fatalf("expected example symlink finding, got %#v", findingMessages(findings))
	}
}

func TestTestSkillFileRejectsTestsEscape(t *testing.T) {
	root := t.TempDir()
	skillPath := filepath.Join(root, "tests-escape", "SKILL.md")
	writeSkill(t, skillPath, `---
id: tests-escape
description: Test assets must remain inside the skill directory.
requires_tools:
  - shell
risk_level: low
tests:
  - ../case.md
---
Body.
`)

	findings, err := TestSkillFile(skillPath, []string{"shell"})
	if err != nil {
		t.Fatalf("TestSkillFile: %v", err)
	}
	if !containsString(findingMessages(findings), `tests: tests entry "../case.md" escapes skill directory`) {
		t.Fatalf("expected tests escape finding, got %#v", findingMessages(findings))
	}
}

func TestTestSkillFileRejectsBackslashAssetPaths(t *testing.T) {
	root := t.TempDir()
	skillPath := filepath.Join(root, "backslash-assets", "SKILL.md")
	writeSkill(t, skillPath, `---
id: backslash-assets
description: Asset paths must use forward slashes.
requires_tools:
  - shell
risk_level: low
examples:
  - ..\example.md
tests:
  - tests\case.md
---
Body.
`)

	findings, err := TestSkillFile(skillPath, []string{"shell"})
	if err != nil {
		t.Fatalf("TestSkillFile: %v", err)
	}
	messages := findingMessages(findings)
	if !containsString(messages, `examples: examples entry "..\\example.md" must not contain backslashes`) {
		t.Fatalf("expected examples backslash finding, got %#v", messages)
	}
	if !containsString(messages, `tests: tests entry "tests\\case.md" must not contain backslashes`) {
		t.Fatalf("expected tests backslash finding, got %#v", messages)
	}
}

func TestPackSkillTemplateWritesStableArchive(t *testing.T) {
	sourceRoot := filepath.Join(t.TempDir(), "workflow-template-curator")
	writeSkill(t, filepath.Join(sourceRoot, "SKILL.md"), `---
id: workflow-template-curator
description: Curate workflow templates.
requires_tools:
  - file_read
risk_level: medium
prompt_file: prompt.md
examples:
  - examples/sample.md
tests:
  - tests/case.md
---
See prompt file.
`)
	writeSkill(t, filepath.Join(sourceRoot, "prompt.md"), "Prompt details.\n")
	writeSkill(t, filepath.Join(sourceRoot, "examples", "sample.md"), "Example details.\n")
	writeSkill(t, filepath.Join(sourceRoot, "tests", "case.md"), "Test details.\n")

	outPath := filepath.Join(t.TempDir(), "workflow-template-curator.zip")
	packedPath, err := PackSkillTemplate(sourceRoot, outPath)
	if err != nil {
		t.Fatalf("PackSkillTemplate: %v", err)
	}
	if packedPath != outPath {
		t.Fatalf("packedPath = %q, want %q", packedPath, outPath)
	}

	names, contents := readZipArchive(t, packedPath)
	wantNames := []string{
		"workflow-template-curator/SKILL.md",
		"workflow-template-curator/examples/sample.md",
		"workflow-template-curator/prompt.md",
		"workflow-template-curator/tests/case.md",
	}
	if !slices.Equal(names, wantNames) {
		t.Fatalf("archive entries = %#v, want %#v", names, wantNames)
	}
	if got := contents["workflow-template-curator/prompt.md"]; got != "Prompt details.\n" {
		t.Fatalf("unexpected prompt contents %q", got)
	}
}

func TestPackSkillTemplateRejectsSymlinkEntries(t *testing.T) {
	sourceRoot := filepath.Join(t.TempDir(), "workflow-template-curator")
	writeSkill(t, filepath.Join(sourceRoot, "SKILL.md"), `---
id: workflow-template-curator
description: Curate workflow templates.
requires_tools:
  - file_read
risk_level: medium
---
Body.
`)
	writeSkill(t, filepath.Join(sourceRoot, "notes-target.md"), "notes\n")
	if err := os.Symlink("notes-target.md", filepath.Join(sourceRoot, "notes.md")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	if _, err := PackSkillTemplate(sourceRoot, filepath.Join(t.TempDir(), "workflow-template-curator.zip")); err == nil || !strings.Contains(err.Error(), "symlink entries are not supported") {
		t.Fatalf("expected symlink entry rejection, got %v", err)
	}
}

func TestPackSkillTemplateRejectsBackslashEntryNames(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows cannot create a file name containing a backslash")
	}

	sourceRoot := filepath.Join(t.TempDir(), "workflow-template-curator")
	writeSkill(t, filepath.Join(sourceRoot, "SKILL.md"), `---
id: workflow-template-curator
description: Curate workflow templates.
requires_tools:
  - file_read
risk_level: medium
---
Body.
`)
	writeSkill(t, filepath.Join(sourceRoot, `..\evil.txt`), "blocked\n")

	outPath := filepath.Join(t.TempDir(), "workflow-template-curator.zip")
	if _, err := PackSkillTemplate(sourceRoot, outPath); err == nil || !strings.Contains(err.Error(), "must not contain backslashes") {
		t.Fatalf("expected backslash entry rejection, got %v", err)
	}
	if _, err := os.Stat(outPath); !os.IsNotExist(err) {
		t.Fatalf("expected no archive to be written, stat err=%v", err)
	}
}

func TestPackSkillTemplateRejectsSingleFileExternalAssets(t *testing.T) {
	sourcePath := filepath.Join(t.TempDir(), "notification-draft.md")
	writeSkill(t, sourcePath, `---
id: notification-draft
description: Draft notification messages only.
requires_tools:
  - file_write
risk_level: medium
prompt_file: prompt.md
examples:
  - examples/draft.md
tests:
  - tests/case.md
---
Draft body.
`)

	outPath := filepath.Join(t.TempDir(), "notification-draft.zip")
	if _, err := PackSkillTemplate(sourcePath, outPath); err == nil || !strings.Contains(err.Error(), "use a directory template instead") {
		t.Fatalf("expected single-file asset rejection, got %v", err)
	}
	if _, err := os.Stat(outPath); !os.IsNotExist(err) {
		t.Fatalf("expected no archive to be written, stat err=%v", err)
	}
}

func TestPackSkillTemplateRejectsOverwrite(t *testing.T) {
	sourceRoot := filepath.Join(t.TempDir(), "notification-draft")
	writeSkill(t, filepath.Join(sourceRoot, "SKILL.md"), `---
id: notification-draft
description: Draft notification messages only.
requires_tools:
  - file_write
risk_level: medium
---
Body.
`)

	outPath := filepath.Join(t.TempDir(), "notification-draft.zip")
	writeSkill(t, outPath, "existing zip placeholder\n")

	if _, err := PackSkillTemplate(sourceRoot, outPath); err == nil || !strings.Contains(err.Error(), "output already exists") {
		t.Fatalf("expected overwrite rejection, got %v", err)
	}
}

func readZipArchive(t *testing.T, archivePath string) ([]string, map[string]string) {
	t.Helper()

	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		t.Fatalf("zip.OpenReader: %v", err)
	}
	defer reader.Close()

	names := make([]string, 0, len(reader.File))
	contents := make(map[string]string, len(reader.File))
	for _, file := range reader.File {
		names = append(names, file.Name)
		rc, err := file.Open()
		if err != nil {
			t.Fatalf("file.Open(%s): %v", file.Name, err)
		}
		raw, err := io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			t.Fatalf("ReadAll(%s): %v", file.Name, err)
		}
		contents[file.Name] = string(raw)
	}
	return names, contents
}
