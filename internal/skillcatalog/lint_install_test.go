package skillcatalog

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLintSkillFileReportsManifestErrors(t *testing.T) {
	root := t.TempDir()
	skillPath := filepath.Join(root, "broken", "SKILL.md")
	writeSkill(t, skillPath, `---
description:
requires_tools:
  - shell
  - missing_tool
risk_level:
prompt_file: prompt.md
token: placeholder
---
`)
	writeSkill(t, filepath.Join(root, "broken", "prompt.md"), "\n")

	findings, err := LintSkillFile(skillPath, []string{"shell", "file_read"})
	if err != nil {
		t.Fatalf("LintSkillFile: %v", err)
	}

	got := findingMessages(findings)
	wantSubstrings := []string{
		"id: front matter must include non-empty id or name",
		"description: description is required",
		"front_matter: front matter contains an obvious secret-like key or value",
		"prompt_file: prompt_file \"prompt.md\" is empty",
		"requires_tools: unknown required tool \"missing_tool\"",
		"risk_level: risk_level is required",
	}
	for _, want := range wantSubstrings {
		if !containsString(got, want) {
			t.Fatalf("expected finding %q in %#v", want, got)
		}
	}
}

func TestLintSkillFileRequiresBodyOrPrompt(t *testing.T) {
	skillPath := filepath.Join(t.TempDir(), "empty.md")
	writeSkill(t, skillPath, `---
id: empty-skill
description: Empty skill.
requires_tools: shell
risk_level: low
---
`)

	findings, err := LintSkillFile(skillPath, []string{"shell"})
	if err != nil {
		t.Fatalf("LintSkillFile: %v", err)
	}
	if !containsString(findingMessages(findings), "body: body or prompt_file is required") {
		t.Fatalf("expected missing body finding, got %#v", findingMessages(findings))
	}
}

func TestLintSkillFileDoesNotTreatTaskIDAsSecret(t *testing.T) {
	skillPath := filepath.Join(t.TempDir(), "clean.md")
	writeSkill(t, skillPath, `---
id: task-triage
description: Triage queued tasks.
requires_tools:
  - shell
risk_level: low
---
Body.
`)

	findings, err := LintSkillFile(skillPath, []string{"shell"})
	if err != nil {
		t.Fatalf("LintSkillFile: %v", err)
	}
	if containsString(findingMessages(findings), "front_matter: front matter contains an obvious secret-like key or value") {
		t.Fatalf("unexpected secret finding for task id: %#v", findingMessages(findings))
	}
}

func TestLintSkillFileFlagsAPIKeyPattern(t *testing.T) {
	skillPath := filepath.Join(t.TempDir(), "secret.md")
	writeSkill(t, skillPath, `---
id: draft-helper
description: Draft helper.
requires_tools:
  - shell
risk_level: low
api_key: sk-proj-1234567890abcdef
---
Body.
`)

	findings, err := LintSkillFile(skillPath, []string{"shell"})
	if err != nil {
		t.Fatalf("LintSkillFile: %v", err)
	}
	if !containsString(findingMessages(findings), "front_matter: front matter contains an obvious secret-like key or value") {
		t.Fatalf("expected secret finding, got %#v", findingMessages(findings))
	}
}

func TestLintSkillFileDoesNotFlagOpenAIKeyUnderGenericKeyName(t *testing.T) {
	skillPath := filepath.Join(t.TempDir(), "generic-key.md")
	writeSkill(t, skillPath, `---
id: generic-key
description: Generic key should not trip secret lint.
requires_tools:
  - shell
risk_level: low
key: sk-example-identifier-12345
---
Body.
`)

	findings, err := LintSkillFile(skillPath, []string{"shell"})
	if err != nil {
		t.Fatalf("LintSkillFile: %v", err)
	}
	if containsString(findingMessages(findings), "front_matter: front matter contains an obvious secret-like key or value") {
		t.Fatalf("unexpected secret finding for generic key: %#v", findingMessages(findings))
	}
}

func TestLintSkillFileIgnoresOpenAIKeyPatternOutsideFrontMatter(t *testing.T) {
	skillPath := filepath.Join(t.TempDir(), "body-only.md")
	writeSkill(t, skillPath, `---
id: body-only
description: Body examples should not trip front matter lint.
requires_tools:
  - shell
risk_level: low
---
Inline example sk-proj-1234567890abcdef.

`+"```yaml\napi_key: sk-proj-1234567890abcdef\n```\n")

	findings, err := LintSkillFile(skillPath, []string{"shell"})
	if err != nil {
		t.Fatalf("LintSkillFile: %v", err)
	}
	if containsString(findingMessages(findings), "front_matter: front matter contains an obvious secret-like key or value") {
		t.Fatalf("unexpected secret finding for markdown body: %#v", findingMessages(findings))
	}
}

func TestLintSkillFileRejectsPromptFileSymlink(t *testing.T) {
	root := t.TempDir()
	skillPath := filepath.Join(root, "symlinked-prompt", "SKILL.md")
	writeSkill(t, skillPath, `---
id: symlinked-prompt
description: Prompt file should be a regular file.
requires_tools:
  - shell
risk_level: low
prompt_file: prompt.md
---
Use the prompt file.
`)
	writeSkill(t, filepath.Join(root, "symlinked-prompt", "prompt-target.md"), "Prompt details.\n")
	if err := os.Symlink("prompt-target.md", filepath.Join(root, "symlinked-prompt", "prompt.md")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	findings, err := LintSkillFile(skillPath, []string{"shell"})
	if err != nil {
		t.Fatalf("LintSkillFile: %v", err)
	}
	if !containsString(findingMessages(findings), "prompt_file: prompt_file \"prompt.md\" must not be a symlink") {
		t.Fatalf("expected prompt_file symlink finding, got %#v", findingMessages(findings))
	}
}

func TestLintSkillFileRejectsPromptFileSymlinkedIntermediateDir(t *testing.T) {
	root := t.TempDir()
	skillDir := filepath.Join(root, "symlinked-prompt-dir")
	skillPath := filepath.Join(skillDir, "SKILL.md")
	writeSkill(t, skillPath, `---
id: symlinked-prompt-dir
description: Prompt file should not traverse symlinked directories.
requires_tools:
  - shell
risk_level: low
prompt_file: prompts/prompt.md
---
Use the prompt file.
`)
	writeSkill(t, filepath.Join(skillDir, "prompt-assets", "prompt.md"), "Prompt details.\n")
	if err := os.Symlink("prompt-assets", filepath.Join(skillDir, "prompts")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	findings, err := LintSkillFile(skillPath, []string{"shell"})
	if err != nil {
		t.Fatalf("LintSkillFile: %v", err)
	}
	if !containsString(findingMessages(findings), "prompt_file: prompt_file \"prompts/prompt.md\" must not traverse symlink component \"prompts\"") {
		t.Fatalf("expected prompt_file intermediate symlink finding, got %#v", findingMessages(findings))
	}
}

func TestLintSkillFileRejectsPromptFileEscape(t *testing.T) {
	root := t.TempDir()
	skillDir := filepath.Join(root, "prompt-escape")
	skillPath := filepath.Join(skillDir, "SKILL.md")
	writeSkill(t, skillPath, `---
id: prompt-escape
description: Prompt file must stay inside the skill directory.
requires_tools:
  - shell
risk_level: low
prompt_file: ../prompt.md
---
Use the prompt file.
`)

	findings, err := LintSkillFile(skillPath, []string{"shell"})
	if err != nil {
		t.Fatalf("LintSkillFile: %v", err)
	}
	if !containsString(findingMessages(findings), "prompt_file: prompt_file \"../prompt.md\" escapes skill directory") {
		t.Fatalf("expected prompt_file escape finding, got %#v", findingMessages(findings))
	}
}

func TestLintSkillFileRejectsPromptFileBackslashes(t *testing.T) {
	root := t.TempDir()
	skillPath := filepath.Join(root, "prompt-backslash", "SKILL.md")
	writeSkill(t, skillPath, `---
id: prompt-backslash
description: Prompt file paths must use forward slashes.
requires_tools:
  - shell
risk_level: low
prompt_file: ..\prompt.md
---
Use the prompt file.
`)

	findings, err := LintSkillFile(skillPath, []string{"shell"})
	if err != nil {
		t.Fatalf("LintSkillFile: %v", err)
	}
	if !containsString(findingMessages(findings), `prompt_file: prompt_file "..\\prompt.md" must not contain backslashes`) {
		t.Fatalf("expected prompt_file backslash finding, got %#v", findingMessages(findings))
	}
}

func TestEnsureNoSymlinkComponentsStabilizesUnexpectedPromptValidationErrors(t *testing.T) {
	root := t.TempDir()
	skillDir := filepath.Join(root, "prompt-validation-error")
	baseDir := skillDir
	target := filepath.Join(skillDir, "locked", "subdir", "prompt.md")
	rel := "locked/subdir/prompt.md"
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	lockedDir := filepath.Join(skillDir, "locked")
	expectedErr := errors.New("injected lstat failure")

	err := ensureNoSymlinkComponentsWithLstat(baseDir, target, rel, func(path string) (os.FileInfo, error) {
		if path == lockedDir {
			return nil, expectedErr
		}
		return os.Lstat(path)
	})
	if err == nil {
		t.Fatal("expected validation error")
	}
	if got, want := err.Error(), `prompt_file "locked/subdir/prompt.md" could not be validated`; got != want {
		t.Fatalf("error = %q, want %q", got, want)
	}
}

func TestInstallSkillTemplateCopiesDirectoryAndRejectsOverwrite(t *testing.T) {
	sourceRoot := filepath.Join(t.TempDir(), "workflow-template-curator")
	writeSkill(t, filepath.Join(sourceRoot, "SKILL.md"), `---
id: workflow-template-curator
description: Curate workflow templates.
requires_tools:
  - file_read
  - file_write
risk_level: medium
prompt_file: prompt.md
---
See prompt file.
`)
	writeSkill(t, filepath.Join(sourceRoot, "prompt.md"), "Prompt details.\n")
	writeSkill(t, filepath.Join(sourceRoot, "examples", "sample.md"), "Sample.\n")

	workspace := t.TempDir()
	installedPath, err := InstallSkillTemplate(sourceRoot, workspace)
	if err != nil {
		t.Fatalf("InstallSkillTemplate: %v", err)
	}
	if want := filepath.Join(workspace, "skills", "workflow-template-curator", "SKILL.md"); installedPath != want {
		t.Fatalf("installedPath = %q, want %q", installedPath, want)
	}
	for _, path := range []string{
		filepath.Join(workspace, "skills", "workflow-template-curator", "SKILL.md"),
		filepath.Join(workspace, "skills", "workflow-template-curator", "prompt.md"),
		filepath.Join(workspace, "skills", "workflow-template-curator", "examples", "sample.md"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected installed file %s: %v", path, err)
		}
	}

	if _, err := InstallSkillTemplate(sourceRoot, workspace); err == nil || !strings.Contains(err.Error(), "destination already exists") {
		t.Fatalf("expected overwrite error, got %v", err)
	}
}

func TestInstallSkillTemplateRejectsExternalSkillsSymlink(t *testing.T) {
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

	workspace := t.TempDir()
	externalRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(externalRoot, "skills"), 0o755); err != nil {
		t.Fatalf("MkdirAll external skills: %v", err)
	}
	if err := os.Symlink(filepath.Join(externalRoot, "skills"), filepath.Join(workspace, "skills")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	if _, err := InstallSkillTemplate(sourceRoot, workspace); err == nil || !strings.Contains(err.Error(), "outside workspace root") {
		t.Fatalf("expected external skills symlink rejection, got %v", err)
	}
}

func TestInstallSkillNameRejectsInvalidCharacters(t *testing.T) {
	sourcePath := filepath.Join(t.TempDir(), "SKILL.md")
	cases := []struct {
		name string
		spec SkillSpec
	}{
		{
			name: "id with slash",
			spec: SkillSpec{ID: "bad/name"},
		},
		{
			name: "id with backslash",
			spec: SkillSpec{ID: `bad\name`},
		},
		{
			name: "name with slash",
			spec: SkillSpec{Name: "bad/name"},
		},
		{
			name: "name with backslash",
			spec: SkillSpec{Name: `bad\name`},
		},
		{
			name: "dot",
			spec: SkillSpec{ID: "."},
		},
		{
			name: "dotdot",
			spec: SkillSpec{ID: ".."},
		},
		{
			name: "less than",
			spec: SkillSpec{ID: "bad<name"},
		},
		{
			name: "greater than",
			spec: SkillSpec{ID: "bad>name"},
		},
		{
			name: "colon",
			spec: SkillSpec{ID: "bad:name"},
		},
		{
			name: "quote",
			spec: SkillSpec{ID: `bad"name`},
		},
		{
			name: "pipe",
			spec: SkillSpec{ID: "bad|name"},
		},
		{
			name: "question mark",
			spec: SkillSpec{ID: "bad?name"},
		},
		{
			name: "asterisk",
			spec: SkillSpec{ID: "bad*name"},
		},
		{
			name: "ascii control",
			spec: SkillSpec{ID: "bad\x1fname"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := installSkillName(tc.spec, sourcePath); err == nil || !strings.Contains(err.Error(), "invalid skill template name") {
				t.Fatalf("expected invalid skill template name error, got %v", err)
			}
		})
	}
}

func findingMessages(findings []LintFinding) []string {
	out := make([]string, 0, len(findings))
	for _, finding := range findings {
		out = append(out, finding.Field+": "+finding.Message)
	}
	return out
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
