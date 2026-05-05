package main

import (
	"archive/zip"
	"bytes"
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestConnectAddrNormalizesWildcardHosts(t *testing.T) {
	tests := map[string]string{
		"0.0.0.0:8421": "127.0.0.1:8421",
		":8421":        "127.0.0.1:8421",
		"[::]:8421":    "127.0.0.1:8421",
		"127.0.0.1:9":  "127.0.0.1:9",
		"bad-address":  "bad-address",
	}
	for input, want := range tests {
		if got := connectAddr(input); got != want {
			t.Fatalf("connectAddr(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestRunSkillLintCommand(t *testing.T) {
	root := t.TempDir()
	skillPath := filepath.Join(root, "examples", "skills", "lintable", "SKILL.md")
	writeSkillFile(t, skillPath, `---
id: lintable
description: Lintable skill.
requires_tools:
  - shell
risk_level: low
---
Body.
`)

	var stdout, stderr bytes.Buffer
	code := run(context.Background(), []string{"skill", "lint", skillPath}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run returned %d, stderr=%q", code, stderr.String())
	}
	if got := stdout.String(); !strings.Contains(got, "OK ") {
		t.Fatalf("expected OK output, got %q", got)
	}
}

func TestRunSkillLintCommandSupportsMultiplePaths(t *testing.T) {
	root := t.TempDir()
	firstSkillPath := filepath.Join(root, "examples", "skills", "first", "SKILL.md")
	secondSkillPath := filepath.Join(root, "examples", "skills", "second", "SKILL.md")
	writeSkillFile(t, firstSkillPath, `---
id: first
description: First lintable skill.
requires_tools:
  - shell
risk_level: low
---
Body.
`)
	writeSkillFile(t, secondSkillPath, `---
id: second
description: Second lintable skill.
requires_tools:
  - shell
risk_level: low
---
Body.
`)

	var stdout, stderr bytes.Buffer
	code := run(context.Background(), []string{"skill", "lint", firstSkillPath, secondSkillPath}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run returned %d, stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	for _, path := range []string{firstSkillPath, secondSkillPath} {
		if !strings.Contains(stdout.String(), "OK "+path) {
			t.Fatalf("expected OK output for %s, got %q", path, stdout.String())
		}
	}
}

func TestRunSkillLintCommandAcceptsGatedToolDependencies(t *testing.T) {
	root := t.TempDir()
	skillPath := filepath.Join(root, "examples", "skills", "automation-builder", "SKILL.md")
	writeSkillFile(t, skillPath, `---
id: automation-builder
description: Build automations.
requires_tools:
  - automation_create_simple
risk_level: high
---
Body.
`)

	var stdout, stderr bytes.Buffer
	code := run(context.Background(), []string{"skill", "lint", skillPath}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run returned %d, stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "OK "+skillPath) {
		t.Fatalf("expected OK output, got %q", stdout.String())
	}
}

func TestRunSkillLintCommandFailsWhenAnyPathHasLintErrors(t *testing.T) {
	root := t.TempDir()
	validSkillPath := filepath.Join(root, "examples", "skills", "valid", "SKILL.md")
	invalidSkillPath := filepath.Join(root, "examples", "skills", "invalid", "SKILL.md")
	writeSkillFile(t, validSkillPath, `---
id: valid
description: Valid skill.
requires_tools:
  - shell
risk_level: low
---
Body.
`)
	writeSkillFile(t, invalidSkillPath, `---
id: invalid
requires_tools:
  - shell
risk_level: low
---
Body.
`)

	var stdout, stderr bytes.Buffer
	code := run(context.Background(), []string{"skill", "lint", validSkillPath, invalidSkillPath}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("run returned %d, stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "OK "+validSkillPath) {
		t.Fatalf("expected OK output for valid path, got %q", stdout.String())
	}
	if !strings.Contains(stdout.String(), "ERROR "+invalidSkillPath+" description: description is required") {
		t.Fatalf("expected path-specific lint error, got %q", stdout.String())
	}
}

func TestRunSkillTestCommandUsage(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run(context.Background(), []string{"skill", "test"}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("run returned %d, stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "usage: sesame skill test <path...> [--workspace <root>]") {
		t.Fatalf("expected usage output, got %q", stderr.String())
	}
}

func TestRunSkillTestCommandSuccess(t *testing.T) {
	root := t.TempDir()
	skillPath := filepath.Join(root, "examples", "skills", "distribution-check", "SKILL.md")
	writeSkillFile(t, skillPath, `---
id: distribution-check
description: Distribution check skill.
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
	writeSkillFile(t, filepath.Join(root, "examples", "skills", "distribution-check", "examples", "sample.md"), "example asset\n")
	writeSkillFile(t, filepath.Join(root, "examples", "skills", "distribution-check", "tests", "case.md"), "test asset\n")

	var stdout, stderr bytes.Buffer
	code := run(context.Background(), []string{"skill", "test", skillPath}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run returned %d, stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "OK "+skillPath) {
		t.Fatalf("expected OK output, got %q", stdout.String())
	}
}

func TestRunSkillTestCommandReportsAssetFailure(t *testing.T) {
	root := t.TempDir()
	skillPath := filepath.Join(root, "examples", "skills", "distribution-check", "SKILL.md")
	writeSkillFile(t, skillPath, `---
id: distribution-check
description: Distribution check skill.
requires_tools:
  - shell
risk_level: low
tests:
  - tests/case.md
---
Body.
`)

	var stdout, stderr bytes.Buffer
	code := run(context.Background(), []string{"skill", "test", skillPath}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("run returned %d, stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), `ERROR `+skillPath+` tests: tests entry "tests/case.md" does not exist`) {
		t.Fatalf("expected tests asset error, got %q", stdout.String())
	}
}

func TestRunSkillInstallByTemplateName(t *testing.T) {
	repoRoot := t.TempDir()
	templateDir := filepath.Join(repoRoot, "examples", "skills", "notification-draft")
	if err := os.MkdirAll(templateDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(templateDir, "SKILL.md"), []byte(`---
id: notification-draft
description: Draft notification messages only.
requires_tools:
  - file_write
risk_level: medium
---
Draft body.
`), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	if err := os.Chdir(repoRoot); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(cwd)
	})

	workspace := filepath.Join(repoRoot, "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatalf("MkdirAll workspace: %v", err)
	}

	var stdout, stderr bytes.Buffer
	code := run(context.Background(), []string{"skill", "install", "notification-draft", "--workspace", workspace}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run returned %d, stderr=%q", code, stderr.String())
	}
	installedPath := filepath.Join(workspace, "skills", "notification-draft", "SKILL.md")
	if _, err := os.Stat(installedPath); err != nil {
		t.Fatalf("expected installed skill: %v", err)
	}
	if got := stdout.String(); !strings.Contains(got, installedPath) {
		t.Fatalf("expected installed path in output, got %q", got)
	}
}

func TestRunSkillInstallByTemplateNamePrefersExamplesOverWorkspaceRelativePath(t *testing.T) {
	repoRoot := t.TempDir()
	templateDir := filepath.Join(repoRoot, "examples", "skills", "notification-draft")
	writeSkillFile(t, filepath.Join(templateDir, "SKILL.md"), `---
id: notification-draft
description: Example template.
requires_tools:
  - file_write
risk_level: medium
---
Example body.
`)
	writeSkillFile(t, filepath.Join(templateDir, "prompt.md"), "example prompt\n")

	workspace := filepath.Join(repoRoot, "workspace")
	writeSkillFile(t, filepath.Join(workspace, "notification-draft", "SKILL.md"), `---
id: local-shadow
description: Local shadow path.
requires_tools:
  - file_write
risk_level: low
---
Shadow body.
`)

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	if err := os.Chdir(repoRoot); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(cwd)
	})

	var stdout, stderr bytes.Buffer
	code := run(context.Background(), []string{"skill", "install", "notification-draft", "--workspace", workspace}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run returned %d, stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}

	installedPath := filepath.Join(workspace, "skills", "notification-draft", "SKILL.md")
	raw, err := os.ReadFile(installedPath)
	if err != nil {
		t.Fatalf("ReadFile installed skill: %v", err)
	}
	if !strings.Contains(string(raw), "Example body.") {
		t.Fatalf("expected example template contents, got %q", string(raw))
	}
}

func TestRunSkillPackCommandUsage(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run(context.Background(), []string{"skill", "pack", "notification-draft"}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("run returned %d, stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "--out is required") {
		t.Fatalf("expected --out usage error, got %q", stderr.String())
	}
}

func TestRunSkillInstallCommandRejectsOutFlag(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run(context.Background(), []string{"skill", "install", "notification-draft", "--workspace", t.TempDir(), "--out", "ignored.zip"}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("run returned %d, stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), `unknown flag "--out"`) {
		t.Fatalf("expected unknown --out flag error, got %q", stderr.String())
	}
}

func TestRunSkillValidationCommandsRejectOutFlag(t *testing.T) {
	for _, subcommand := range []string{"lint", "test"} {
		t.Run(subcommand, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := run(context.Background(), []string{"skill", subcommand, "SKILL.md", "--out", "ignored.zip"}, &stdout, &stderr)
			if code != 2 {
				t.Fatalf("run returned %d, stdout=%q stderr=%q", code, stdout.String(), stderr.String())
			}
			if !strings.Contains(stderr.String(), `unknown flag "--out"`) {
				t.Fatalf("expected unknown --out flag error, got %q", stderr.String())
			}
		})
	}
}

func TestRunSkillPackCommandByTemplateName(t *testing.T) {
	repoRoot := t.TempDir()
	templateDir := filepath.Join(repoRoot, "examples", "skills", "notification-draft")
	writeSkillFile(t, filepath.Join(templateDir, "SKILL.md"), `---
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
	writeSkillFile(t, filepath.Join(templateDir, "prompt.md"), "prompt details\n")
	writeSkillFile(t, filepath.Join(templateDir, "examples", "draft.md"), "example details\n")
	writeSkillFile(t, filepath.Join(templateDir, "tests", "case.md"), "test details\n")

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	if err := os.Chdir(repoRoot); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(cwd)
	})

	outPath := filepath.Join(t.TempDir(), "notification-draft.zip")
	var stdout, stderr bytes.Buffer
	code := run(context.Background(), []string{"skill", "pack", "notification-draft", "--out", outPath}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run returned %d, stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "packed "+outPath) {
		t.Fatalf("expected packed output, got %q", stdout.String())
	}

	wantEntries := []string{
		"notification-draft/SKILL.md",
		"notification-draft/examples/draft.md",
		"notification-draft/prompt.md",
		"notification-draft/tests/case.md",
	}
	if got := zipEntryNames(t, outPath); !slices.Equal(got, wantEntries) {
		t.Fatalf("archive entries = %#v, want %#v", got, wantEntries)
	}
}

func TestRunSkillPackCommandFailsValidationWithoutWritingArchive(t *testing.T) {
	repoRoot := t.TempDir()
	templateDir := filepath.Join(repoRoot, "examples", "skills", "notification-draft")
	writeSkillFile(t, filepath.Join(templateDir, "SKILL.md"), `---
id: notification-draft
description: Draft notification messages only.
requires_tools:
  - file_write
risk_level: medium
tests:
  - tests/case.md
---
Draft body.
`)

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	if err := os.Chdir(repoRoot); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(cwd)
	})

	outPath := filepath.Join(t.TempDir(), "notification-draft.zip")
	var stdout, stderr bytes.Buffer
	code := run(context.Background(), []string{"skill", "pack", "notification-draft", "--out", outPath}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("run returned %d, stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), `tests: tests entry "tests/case.md" does not exist`) {
		t.Fatalf("expected validation output, got %q", stdout.String())
	}
	if _, err := os.Stat(outPath); !os.IsNotExist(err) {
		t.Fatalf("expected no archive to be written, stat err=%v", err)
	}
}

func TestResolveSkillTemplateSourceTreatsBackslashAsPathSeparator(t *testing.T) {
	workspace := t.TempDir()
	templateDir := filepath.Join(workspace, "examples", "skills", "notification-draft")
	writeSkillFile(t, filepath.Join(templateDir, "SKILL.md"), `---
id: notification-draft
description: Draft notification messages only.
requires_tools:
  - file_write
risk_level: medium
---
Draft body.
`)

	input := strings.Join([]string{"examples", "skills", "notification-draft"}, `\`)
	got, err := resolveSkillTemplateSource(input, workspace)
	if err != nil {
		t.Fatalf("resolveSkillTemplateSource: %v", err)
	}
	if want := templateDir; got != want {
		t.Fatalf("resolveSkillTemplateSource(%q) = %q, want %q", input, got, want)
	}
}

func writeSkillFile(t *testing.T, path string, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

func zipEntryNames(t *testing.T, archivePath string) []string {
	t.Helper()

	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		t.Fatalf("zip.OpenReader: %v", err)
	}
	defer reader.Close()

	names := make([]string, 0, len(reader.File))
	for _, file := range reader.File {
		names = append(names, file.Name)
	}
	return names
}
