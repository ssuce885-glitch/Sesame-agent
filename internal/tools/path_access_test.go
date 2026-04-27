package tools

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEnsureAllowedReadPathRejectsSymlinkEscape(t *testing.T) {
	workspaceRoot := t.TempDir()
	outsideRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(outsideRoot, "secret.txt"), []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outsideRoot, filepath.Join(workspaceRoot, "outside")); err != nil {
		t.Fatal(err)
	}

	err := ensureAllowedReadPath(ExecContext{WorkspaceRoot: workspaceRoot}, filepath.Join(workspaceRoot, "outside", "secret.txt"))
	if err == nil {
		t.Fatal("ensureAllowedReadPath allowed a symlink escape")
	}
}

func TestPathWithinRootAllowsMissingPathInsideRoot(t *testing.T) {
	workspaceRoot := t.TempDir()

	ok, err := pathWithinRoot(workspaceRoot, filepath.Join(workspaceRoot, "new.txt"))
	if err != nil {
		t.Fatalf("pathWithinRoot() error = %v", err)
	}
	if !ok {
		t.Fatal("pathWithinRoot rejected a missing path inside root")
	}
}
