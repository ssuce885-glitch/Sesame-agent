package extensions

import (
	"errors"
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

func TestInspectSkillSourceSkipsDisabledRemoteCandidateFromRepoRoot(t *testing.T) {
	t.Cleanup(func() {
		githubRequestFunc = githubRequest
	})
	githubRequestFunc = func(rawURL, accept string) ([]byte, error) {
		switch rawURL {
		case "https://api.github.com/repos/example/skills/git/trees/main?recursive=1":
			if accept != "application/vnd.github+json" {
				t.Fatalf("githubRequestFunc() accept = %q, want application/vnd.github+json", accept)
			}
			return []byte(`{
				"tree": [
					{"path":"skills/disabled/SKILL.md","type":"blob"},
					{"path":"skills/disabled/SKILL.json","type":"blob"}
				],
				"truncated": false
			}`), nil
		case "https://api.github.com/repos/example/skills/contents/skills/disabled?ref=main":
			if accept != "application/vnd.github+json" {
				t.Fatalf("githubRequestFunc() accept = %q, want application/vnd.github+json", accept)
			}
			return []byte(`[
				{"name":"SKILL.md","type":"file"},
				{"name":"SKILL.json","type":"file"}
			]`), nil
		case "https://api.github.com/repos/example/skills/contents/skills/disabled/SKILL.json?ref=main":
			if accept != "application/vnd.github.raw" {
				t.Fatalf("githubRequestFunc() accept = %q, want application/vnd.github.raw", accept)
			}
			return []byte(`{
				"name": "disabled",
				"description": "hidden",
				"enabled": false
			}`), nil
		default:
			t.Fatalf("githubRequestFunc() unexpected URL %q", rawURL)
			return nil, nil
		}
	}

	plan, err := InspectSkillSource(t.TempDir(), t.TempDir(), InstallRequest{Source: "example/skills"})
	if err != nil {
		t.Fatalf("InspectSkillSource() error = %v", err)
	}
	if plan.AutoInstallable {
		t.Fatal("InspectSkillSource() AutoInstallable = true, want false for disabled remote candidate")
	}
	if len(plan.CandidatePaths) != 0 {
		t.Fatalf("InspectSkillSource() len(CandidatePaths) = %d, want 0", len(plan.CandidatePaths))
	}
	if plan.Path != "" {
		t.Fatalf("InspectSkillSource() Path = %q, want empty", plan.Path)
	}
}

func TestInspectSkillSourceErrorsWhenRemoteCandidateMetadataCannotBeValidated(t *testing.T) {
	t.Cleanup(func() {
		githubRequestFunc = githubRequest
	})
	githubRequestFunc = func(rawURL, accept string) ([]byte, error) {
		switch rawURL {
		case "https://api.github.com/repos/example/skills/git/trees/main?recursive=1":
			return []byte(`{
				"tree": [
					{"path":"skills/active/SKILL.md","type":"blob"},
					{"path":"skills/active/SKILL.json","type":"blob"},
					{"path":"skills/broken/SKILL.md","type":"blob"},
					{"path":"skills/broken/SKILL.json","type":"blob"}
				],
				"truncated": false
			}`), nil
		case "https://api.github.com/repos/example/skills/contents/skills/active?ref=main":
			return []byte(`[
				{"name":"SKILL.md","type":"file"},
				{"name":"SKILL.json","type":"file"}
			]`), nil
		case "https://api.github.com/repos/example/skills/contents/skills/active/SKILL.json?ref=main":
			return []byte(`{
				"name": "active"
			}`), nil
		case "https://api.github.com/repos/example/skills/contents/skills/broken?ref=main":
			return []byte(`[
				{"name":"SKILL.md","type":"file"},
				{"name":"SKILL.json","type":"file"}
			]`), nil
		case "https://api.github.com/repos/example/skills/contents/skills/broken/SKILL.json?ref=main":
			return nil, errors.New("temporary GitHub failure")
		default:
			t.Fatalf("githubRequestFunc() unexpected URL %q", rawURL)
			return nil, nil
		}
	}

	_, err := InspectSkillSource(t.TempDir(), t.TempDir(), InstallRequest{Source: "example/skills"})
	if err == nil {
		t.Fatal("InspectSkillSource() error = nil, want remote candidate validation failure")
	}
	if !strings.Contains(err.Error(), "skills/broken") {
		t.Fatalf("error = %v, want failing remote candidate path", err)
	}
}

func TestListRemoteSkillNamesFiltersLegacyAndDisabledChildren(t *testing.T) {
	t.Cleanup(func() {
		githubRequestFunc = githubRequest
	})
	githubRequestFunc = func(rawURL, accept string) ([]byte, error) {
		switch rawURL {
		case "https://api.github.com/repos/example/skills/contents/skills?ref=main":
			if accept != "application/vnd.github+json" {
				t.Fatalf("githubRequestFunc() accept = %q, want application/vnd.github+json", accept)
			}
			return []byte(`[
				{"name":"active","type":"dir"},
				{"name":"disabled","type":"dir"},
				{"name":"legacy","type":"dir"},
				{"name":"README.md","type":"file"}
			]`), nil
		case "https://api.github.com/repos/example/skills/contents/skills/active?ref=main":
			if accept != "application/vnd.github+json" {
				t.Fatalf("githubRequestFunc() accept = %q, want application/vnd.github+json", accept)
			}
			return []byte(`[
				{"name":"SKILL.md","type":"file"},
				{"name":"SKILL.json","type":"file"}
			]`), nil
		case "https://api.github.com/repos/example/skills/contents/skills/active/SKILL.json?ref=main":
			if accept != "application/vnd.github.raw" {
				t.Fatalf("githubRequestFunc() accept = %q, want application/vnd.github.raw", accept)
			}
			return []byte(`{
				"name": "active",
				"description": "shown"
			}`), nil
		case "https://api.github.com/repos/example/skills/contents/skills/disabled?ref=main":
			if accept != "application/vnd.github+json" {
				t.Fatalf("githubRequestFunc() accept = %q, want application/vnd.github+json", accept)
			}
			return []byte(`[
				{"name":"SKILL.md","type":"file"},
				{"name":"SKILL.json","type":"file"}
			]`), nil
		case "https://api.github.com/repos/example/skills/contents/skills/disabled/SKILL.json?ref=main":
			if accept != "application/vnd.github.raw" {
				t.Fatalf("githubRequestFunc() accept = %q, want application/vnd.github.raw", accept)
			}
			return []byte(`{
				"name": "disabled",
				"description": "hidden",
				"enabled": false
			}`), nil
		case "https://api.github.com/repos/example/skills/contents/skills/legacy?ref=main":
			if accept != "application/vnd.github+json" {
				t.Fatalf("githubRequestFunc() accept = %q, want application/vnd.github+json", accept)
			}
			return []byte(`[
				{"name":"SKILL.md","type":"file"}
			]`), nil
		default:
			t.Fatalf("githubRequestFunc() unexpected URL %q", rawURL)
			return nil, nil
		}
	}

	names, err := ListRemoteSkillNames("example/skills", "skills", "main")
	if err != nil {
		t.Fatalf("ListRemoteSkillNames() error = %v", err)
	}
	if len(names) != 1 || names[0] != "active" {
		t.Fatalf("ListRemoteSkillNames() = %v, want [active]", names)
	}
}

func TestListRemoteSkillNamesErrorsWhenChildMetadataCannotBeValidated(t *testing.T) {
	t.Cleanup(func() {
		githubRequestFunc = githubRequest
	})
	githubRequestFunc = func(rawURL, accept string) ([]byte, error) {
		switch rawURL {
		case "https://api.github.com/repos/example/skills/contents/skills?ref=main":
			return []byte(`[
				{"name":"active","type":"dir"},
				{"name":"broken","type":"dir"}
			]`), nil
		case "https://api.github.com/repos/example/skills/contents/skills/active?ref=main":
			return []byte(`[
				{"name":"SKILL.md","type":"file"},
				{"name":"SKILL.json","type":"file"}
			]`), nil
		case "https://api.github.com/repos/example/skills/contents/skills/active/SKILL.json?ref=main":
			return []byte(`{"name":"active"}`), nil
		case "https://api.github.com/repos/example/skills/contents/skills/broken?ref=main":
			return []byte(`[
				{"name":"SKILL.md","type":"file"},
				{"name":"SKILL.json","type":"file"}
			]`), nil
		case "https://api.github.com/repos/example/skills/contents/skills/broken/SKILL.json?ref=main":
			return []byte(`{`), nil
		default:
			t.Fatalf("githubRequestFunc() unexpected URL %q", rawURL)
			return nil, nil
		}
	}

	_, err := ListRemoteSkillNames("example/skills", "skills", "main")
	if err == nil {
		t.Fatal("ListRemoteSkillNames() error = nil, want child metadata validation failure")
	}
	if !strings.Contains(err.Error(), "skills/broken") {
		t.Fatalf("error = %v, want failing child path", err)
	}
}
