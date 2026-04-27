package runtime

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWithinWorkspaceRejectsSymlinkEscape(t *testing.T) {
	workspaceRoot := t.TempDir()
	outsideRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(outsideRoot, "secret.txt"), []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outsideRoot, filepath.Join(workspaceRoot, "outside")); err != nil {
		t.Fatal(err)
	}

	err := WithinWorkspace(workspaceRoot, filepath.Join(workspaceRoot, "outside", "secret.txt"))
	if err == nil {
		t.Fatal("WithinWorkspace allowed a symlink escape")
	}
}

func TestWithinWorkspaceRejectsMissingPathUnderSymlinkEscape(t *testing.T) {
	workspaceRoot := t.TempDir()
	outsideRoot := t.TempDir()
	if err := os.Symlink(outsideRoot, filepath.Join(workspaceRoot, "outside")); err != nil {
		t.Fatal(err)
	}

	err := WithinWorkspace(workspaceRoot, filepath.Join(workspaceRoot, "outside", "new.txt"))
	if err == nil {
		t.Fatal("WithinWorkspace allowed a missing path under a symlink escape")
	}
}

func TestWithinWorkspaceAllowsMissingPathInsideWorkspace(t *testing.T) {
	workspaceRoot := t.TempDir()

	if err := WithinWorkspace(workspaceRoot, filepath.Join(workspaceRoot, "new.txt")); err != nil {
		t.Fatalf("WithinWorkspace() error = %v", err)
	}
}
