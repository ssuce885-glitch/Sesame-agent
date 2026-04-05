package engine

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadWorkspacePromptBundleOrdersPromptAndRules(t *testing.T) {
	root := t.TempDir()
	writeWorkspaceFile(t, root, ".agentd/prompt.md", "workspace prompt")
	writeWorkspaceFile(t, root, ".agentd/rules/10-go.md", "go rule")
	writeWorkspaceFile(t, root, ".agentd/rules/00-core.md", "core rule")
	writeWorkspaceFile(t, root, ".agentd/rules/20-tests.md", "tests rule")

	got, notices, err := loadWorkspacePromptBundle(root, 32768)
	if err != nil {
		t.Fatalf("loadWorkspacePromptBundle() error = %v", err)
	}
	if len(notices) != 0 {
		t.Fatalf("Notices = %v, want empty", notices)
	}

	want := strings.Join([]string{
		"workspace prompt",
		"core rule",
		"go rule",
		"tests rule",
	}, "\n\n")
	if got != want {
		t.Fatalf("loadWorkspacePromptBundle() = %q, want %q", got, want)
	}
}

func TestLoadWorkspacePromptBundleReturnsNoticeOnBudgetOverflow(t *testing.T) {
	root := t.TempDir()
	writeWorkspaceFile(t, root, ".agentd/prompt.md", "12345")
	writeWorkspaceFile(t, root, ".agentd/rules/00-core.md", "rule")

	got, notices, err := loadWorkspacePromptBundle(root, 5)
	if err != nil {
		t.Fatalf("loadWorkspacePromptBundle() error = %v", err)
	}
	if got != "12345" {
		t.Fatalf("Text = %q, want %q", got, "12345")
	}
	if len(notices) != 1 {
		t.Fatalf("Notices = %v, want single truncation notice", notices)
	}
}

func TestLoadWorkspacePromptBundleSkipsPathsFrontmatter(t *testing.T) {
	root := t.TempDir()
	writeWorkspaceFile(t, root, ".agentd/rules/00-core.md", "---\npaths:\n  - internal/**\n---\ncore rule")

	got, notices, err := loadWorkspacePromptBundle(root, 32768)
	if err != nil {
		t.Fatalf("loadWorkspacePromptBundle() error = %v", err)
	}
	if got != "" {
		t.Fatalf("Text = %q, want empty", got)
	}
	if len(notices) != 0 {
		t.Fatalf("Notices = %v, want empty", notices)
	}
}

func TestLoadWorkspacePromptBundleReturnsErrorWhenPromptPathIsUnreadable(t *testing.T) {
	root := t.TempDir()
	promptPath := filepath.Join(root, ".agentd", "prompt.md")
	if err := os.MkdirAll(promptPath, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	_, _, err := loadWorkspacePromptBundle(root, 32768)
	if err == nil {
		t.Fatal("loadWorkspacePromptBundle() error = nil, want error")
	}
}

func TestLoadWorkspacePromptBundleSkipsUnreadableRuleFile(t *testing.T) {
	root := t.TempDir()
	writeWorkspaceFile(t, root, ".agentd/rules/00-good.md", "good rule")
	writeWorkspaceFile(t, root, ".agentd/rules/10-bad.md", "bad rule")

	originalReadFile := readFile
	readFile = func(path string) ([]byte, error) {
		if filepath.Base(path) == "10-bad.md" {
			return nil, errors.New("boom")
		}
		return originalReadFile(path)
	}
	t.Cleanup(func() {
		readFile = originalReadFile
	})

	got, notices, err := loadWorkspacePromptBundle(root, 32768)
	if err != nil {
		t.Fatalf("loadWorkspacePromptBundle() error = %v", err)
	}
	if len(notices) != 0 {
		t.Fatalf("Notices = %v, want empty", notices)
	}
	if got != "good rule" {
		t.Fatalf("Text = %q, want %q", got, "good rule")
	}
}

func writeWorkspaceFile(t *testing.T, root, rel, text string) {
	t.Helper()

	path := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
}
