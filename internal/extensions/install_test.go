package extensions

import (
	"archive/zip"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadCatalogInstallsSystemSkills(t *testing.T) {
	home := t.TempDir()
	workspace := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	catalog, err := LoadCatalog(filepath.Join(home, ".sesame"), workspace)
	if err != nil {
		t.Fatalf("LoadCatalog() error = %v", err)
	}
	if len(catalog.Skills) == 0 {
		t.Fatal("len(Skills) = 0, want bundled system skills")
	}
	var found bool
	for _, skill := range catalog.Skills {
		if skill.Name == "skill-installer" {
			found = true
			if skill.Scope != ScopeSystem {
				t.Fatalf("skill-installer scope = %q, want %q", skill.Scope, ScopeSystem)
			}
			if !strings.Contains(skill.Path, filepath.Join(".sesame", "skills", ".system")) {
				t.Fatalf("skill-installer path = %q, want system path", skill.Path)
			}
		}
	}
	if !found {
		t.Fatalf("catalog.Skills = %#v, want bundled skill-installer", catalog.Skills)
	}
}

func TestInstallSkillCopiesLocalDirectoryIntoWorkspaceScope(t *testing.T) {
	home := t.TempDir()
	workspace := t.TempDir()
	source := filepath.Join(t.TempDir(), "demo-skill")
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	writeFile(t, filepath.Join(source, "SKILL.md"), "---\nname: demo-skill\ndescription: workspace helper\n---\nUse the workspace helper.")
	writeFile(t, filepath.Join(source, "scripts", "run.sh"), "#!/usr/bin/env bash\necho ok\n")

	result, err := InstallSkill(filepath.Join(home, ".sesame"), workspace, InstallRequest{
		Scope:  ScopeWorkspace,
		Source: source,
	})
	if err != nil {
		t.Fatalf("InstallSkill() error = %v", err)
	}
	if result.Scope != ScopeWorkspace {
		t.Fatalf("result.Scope = %q, want %q", result.Scope, ScopeWorkspace)
	}
	if _, err := os.Stat(filepath.Join(result.Destination, "SKILL.md")); err != nil {
		t.Fatalf("installed SKILL.md missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(result.Destination, "scripts", "run.sh")); err != nil {
		t.Fatalf("installed script missing: %v", err)
	}

	skills, err := ListSkills(filepath.Join(home, ".sesame"), workspace, ScopeWorkspace)
	if err != nil {
		t.Fatalf("ListSkills() error = %v", err)
	}
	if len(skills) != 1 {
		t.Fatalf("len(skills) = %d, want 1", len(skills))
	}
	if skills[0].Name != "demo-skill" {
		t.Fatalf("skills[0].Name = %q, want demo-skill", skills[0].Name)
	}
}

func TestRemoveSkillMatchesDisplayName(t *testing.T) {
	home := t.TempDir()
	workspace := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	skillDir := filepath.Join(home, ".sesame", "skills", "demo-dir")
	writeFile(t, filepath.Join(skillDir, "SKILL.md"), "---\nname: demo-skill\n---\nRemove me")

	result, err := RemoveSkill(filepath.Join(home, ".sesame"), workspace, ScopeGlobal, "demo-skill")
	if err != nil {
		t.Fatalf("RemoveSkill() error = %v", err)
	}
	if result.Name != "demo-skill" {
		t.Fatalf("result.Name = %q, want demo-skill", result.Name)
	}
	if _, err := os.Stat(skillDir); !os.IsNotExist(err) {
		t.Fatalf("skillDir still exists, err = %v", err)
	}
}

func TestInstallSkillRejectsUnsupportedScope(t *testing.T) {
	home := t.TempDir()
	workspace := t.TempDir()
	source := filepath.Join(t.TempDir(), "demo-skill")
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	writeFile(t, filepath.Join(source, "SKILL.md"), "---\nname: demo\n---\nBody")

	_, err := InstallSkill(filepath.Join(home, ".sesame"), workspace, InstallRequest{
		Scope:  ScopeSystem,
		Source: source,
	})
	if err == nil {
		t.Fatal("InstallSkill() error = nil, want unsupported scope error")
	}
	if !strings.Contains(err.Error(), "global or workspace") {
		t.Fatalf("error = %v, want global/workspace restriction", err)
	}
}

func TestParseGitHubSkillURL(t *testing.T) {
	parsed, ok, err := parseGitHubSkillURL("https://github.com/openai/skills/tree/main/skills/.curated/parallel", "", "")
	if err != nil {
		t.Fatalf("parseGitHubSkillURL() error = %v", err)
	}
	if !ok {
		t.Fatal("ok = false, want true")
	}
	if parsed.owner != "openai" || parsed.repo != "skills" {
		t.Fatalf("parsed repo = %s/%s, want openai/skills", parsed.owner, parsed.repo)
	}
	if parsed.ref != "main" {
		t.Fatalf("parsed.ref = %q, want main", parsed.ref)
	}
	if parsed.repoPath != "skills/.curated/parallel" {
		t.Fatalf("parsed.repoPath = %q, want skills/.curated/parallel", parsed.repoPath)
	}
}

func TestInspectSkillSourceReadsDocumentationForRepoRoot(t *testing.T) {
	home := t.TempDir()
	workspace := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	withGitHubRequestStub(t, func(rawURL, accept string) ([]byte, error) {
		switch {
		case strings.Contains(rawURL, "/git/trees/main"):
			return []byte(`{"tree":[{"path":"README.md","type":"blob"},{"path":"skills/foo/SKILL.md","type":"blob"},{"path":"skills/bar/SKILL.md","type":"blob"}]}`), nil
		case strings.Contains(rawURL, "/contents/README.md"):
			return []byte("Install by copying one of the skills directories into .sesame/skills after choosing the right one."), nil
		default:
			return nil, fmt.Errorf("unexpected URL: %s", rawURL)
		}
	})

	plan, err := InspectSkillSource(filepath.Join(home, ".sesame"), workspace, InstallRequest{
		Source: "https://github.com/acme/skills",
	})
	if err != nil {
		t.Fatalf("InspectSkillSource() error = %v", err)
	}
	if plan.Track != InstallTrackDocumentation {
		t.Fatalf("plan.Track = %q, want %q", plan.Track, InstallTrackDocumentation)
	}
	if plan.AutoInstallable {
		t.Fatal("plan.AutoInstallable = true, want false for ambiguous repo root")
	}
	if !plan.ReadmeFound || plan.ReadmePath != "README.md" {
		t.Fatalf("README = %+v, want README.md", plan)
	}
	if len(plan.CandidatePaths) != 2 {
		t.Fatalf("len(plan.CandidatePaths) = %d, want 2", len(plan.CandidatePaths))
	}
	if plan.ManualReason == "" {
		t.Fatal("plan.ManualReason = empty, want explanation for multiple candidates")
	}
}

func TestInstallSkillFromRepoRootAutoResolvesSingleCandidate(t *testing.T) {
	home := t.TempDir()
	workspace := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	archive := makeGitHubZipArchive(t, map[string]string{
		"repo-main/README.md":            "Copy this skill into your sesame skills folder.",
		"repo-main/skills/demo/SKILL.md": "---\nname: demo\n---\nDemo body",
		"repo-main/skills/demo/info.txt": "ok",
	})
	withGitHubRequestStub(t, func(rawURL, accept string) ([]byte, error) {
		switch {
		case strings.Contains(rawURL, "/git/trees/main"):
			return []byte(`{"tree":[{"path":"README.md","type":"blob"},{"path":"skills/demo/SKILL.md","type":"blob"}]}`), nil
		case strings.Contains(rawURL, "/contents/README.md"):
			return []byte("Copy this skill into .sesame/skills or install it with the Sesame CLI."), nil
		case strings.Contains(rawURL, "/zipball/main"):
			return archive, nil
		default:
			return nil, fmt.Errorf("unexpected URL: %s", rawURL)
		}
	})

	result, err := InstallSkill(filepath.Join(home, ".sesame"), workspace, InstallRequest{
		Scope:  ScopeWorkspace,
		Source: "https://github.com/acme/demo-repo",
	})
	if err != nil {
		t.Fatalf("InstallSkill() error = %v", err)
	}
	if result.DirectoryName != "demo" {
		t.Fatalf("result.DirectoryName = %q, want demo", result.DirectoryName)
	}
	if _, err := os.Stat(filepath.Join(workspace, ".sesame", "skills", "demo", "SKILL.md")); err != nil {
		t.Fatalf("installed SKILL.md missing: %v", err)
	}
}

func TestInstallSkillFromAgentsSourceStillUsesSesameInstallRoot(t *testing.T) {
	home := t.TempDir()
	workspace := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	archive := makeGitHubZipArchive(t, map[string]string{
		"repo-main/README.md":                           "Copy this skill into Sesame's skill directory.",
		"repo-main/.agents/skills/source-demo/SKILL.md": "---\nname: source-demo\n---\nDemo body",
	})
	withGitHubRequestStub(t, func(rawURL, accept string) ([]byte, error) {
		switch {
		case strings.Contains(rawURL, "/git/trees/main"):
			return []byte(`{"tree":[{"path":"README.md","type":"blob"},{"path":".agents/skills/source-demo/SKILL.md","type":"blob"}]}`), nil
		case strings.Contains(rawURL, "/contents/README.md"):
			return []byte("Copy this skill into .sesame/skills or install it with the Sesame CLI."), nil
		case strings.Contains(rawURL, "/zipball/main"):
			return archive, nil
		default:
			return nil, fmt.Errorf("unexpected URL: %s", rawURL)
		}
	})

	result, err := InstallSkill(filepath.Join(home, ".sesame"), workspace, InstallRequest{
		Scope:  ScopeGlobal,
		Source: "https://github.com/acme/demo-repo",
	})
	if err != nil {
		t.Fatalf("InstallSkill() error = %v", err)
	}

	wantDestination := filepath.Join(home, ".sesame", "skills", "source-demo")
	if result.Destination != wantDestination {
		t.Fatalf("result.Destination = %q, want %q", result.Destination, wantDestination)
	}
	if strings.Contains(result.Destination, ".agents") {
		t.Fatalf("result.Destination = %q, want Sesame install root only", result.Destination)
	}
	if _, err := os.Stat(filepath.Join(wantDestination, "SKILL.md")); err != nil {
		t.Fatalf("installed SKILL.md missing: %v", err)
	}
}

func withGitHubRequestStub(t *testing.T, stub func(rawURL, accept string) ([]byte, error)) {
	t.Helper()
	previous := githubRequestFunc
	githubRequestFunc = stub
	t.Cleanup(func() {
		githubRequestFunc = previous
	})
}

func makeGitHubZipArchive(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	writer := zip.NewWriter(&buf)
	for name, content := range files {
		fileWriter, err := writer.Create(name)
		if err != nil {
			t.Fatalf("Create(%q) error = %v", name, err)
		}
		if _, err := fileWriter.Write([]byte(content)); err != nil {
			t.Fatalf("Write(%q) error = %v", name, err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	return buf.Bytes()
}

func TestInspectSkillSourceFiltersForeignAndTemplatePaths(t *testing.T) {
	home := t.TempDir()
	workspace := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	withGitHubRequestStub(t, func(rawURL, accept string) ([]byte, error) {
		switch {
		case strings.Contains(rawURL, "/git/trees/main"):
			return []byte(`{"tree":[{"path":"README.md","type":"blob"},{"path":".agents/skills/start/SKILL.md","type":"blob"},{"path":".claude/skills/trellis-meta/SKILL.md","type":"blob"},{"path":"packages/cli/src/templates/codex/skills/start/SKILL.md","type":"blob"}]}`), nil
		case strings.Contains(rawURL, "/contents/README.md"):
			return []byte("Copy the shared agent skills into your own platform skill directory."), nil
		default:
			return nil, fmt.Errorf("unexpected URL: %s", rawURL)
		}
	})

	plan, err := InspectSkillSource(filepath.Join(home, ".sesame"), workspace, InstallRequest{Source: "https://github.com/acme/skills"})
	if err != nil {
		t.Fatalf("InspectSkillSource() error = %v", err)
	}
	if len(plan.CandidatePaths) != 1 || plan.CandidatePaths[0] != ".agents/skills/start" {
		t.Fatalf("CandidatePaths = %#v, want only .agents/skills/start", plan.CandidatePaths)
	}
	if len(plan.IgnoredCandidatePaths) != 2 {
		t.Fatalf("IgnoredCandidatePaths = %#v, want 2 ignored paths", plan.IgnoredCandidatePaths)
	}
	joinedNotes := strings.Join(plan.Notes, "\n")
	if !strings.Contains(joinedNotes, ".claude/skills/trellis-meta") {
		t.Fatalf("Notes = %q, want foreign candidate mention", joinedNotes)
	}
}

func TestInspectSkillSourceRejectsForeignExplicitPath(t *testing.T) {
	home := t.TempDir()
	workspace := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	_, err := InspectSkillSource(filepath.Join(home, ".sesame"), workspace, InstallRequest{
		Source: "https://github.com/acme/skills/tree/main/.claude/skills/trellis-meta",
	})
	if err == nil {
		t.Fatal("InspectSkillSource() error = nil, want rejection for foreign platform path")
	}
	if !strings.Contains(err.Error(), "platform-specific") {
		t.Fatalf("error = %v, want platform-specific rejection", err)
	}
}
