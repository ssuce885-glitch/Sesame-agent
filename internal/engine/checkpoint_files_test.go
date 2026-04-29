package engine

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"go-agent/internal/store/sqlite"
)

func TestFileCheckpointServiceCheckpointDiffAndRollback(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}

	ctx := context.Background()
	workspace := t.TempDir()
	store, err := sqlite.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	writeTestFile(t, workspace, "a.txt", "one\n")
	service := NewFileCheckpointService(store, workspace)
	first, err := service.CheckpointBeforeTool(ctx, "sess_1", "turn_1", "call_1", "file_write", `{"path":"a.txt"}`)
	if err != nil {
		t.Fatalf("CheckpointBeforeTool(first) error = %v", err)
	}
	if first == nil || strings.TrimSpace(first.GitCommitHash) == "" {
		t.Fatalf("first checkpoint = %#v, want git commit hash", first)
	}
	if hasFileCheckpointPath(first.FilesChanged, ".sesame/git-checkpoints") {
		t.Fatalf("first checkpoint tracked shadow git repo: %#v", first.FilesChanged)
	}

	writeTestFile(t, workspace, "a.txt", "two\n")
	writeTestFile(t, workspace, "b.txt", "new\n")
	second, err := service.CheckpointBeforeTool(ctx, "sess_1", "turn_2", "call_2", "apply_patch", "patch")
	if err != nil {
		t.Fatalf("CheckpointBeforeTool(second) error = %v", err)
	}
	if second.ParentCheckpointID != first.ID {
		t.Fatalf("second parent = %q, want %q", second.ParentCheckpointID, first.ID)
	}
	diff, err := service.GetDiff(first.ID, second.ID)
	if err != nil {
		t.Fatalf("GetDiff() error = %v", err)
	}
	if !strings.Contains(diff, "a.txt") || !strings.Contains(diff, "b.txt") {
		t.Fatalf("diff = %q, want a.txt and b.txt", diff)
	}

	writeTestFile(t, workspace, "a.txt", "three\n")
	writeTestFile(t, workspace, "c.txt", "later\n")
	if err := service.RollbackTo(ctx, first.ID); err != nil {
		t.Fatalf("RollbackTo() error = %v", err)
	}
	if got := readTestFile(t, workspace, "a.txt"); got != "one\n" {
		t.Fatalf("a.txt after rollback = %q, want one", got)
	}
	if _, err := os.Stat(filepath.Join(workspace, "b.txt")); !os.IsNotExist(err) {
		t.Fatalf("b.txt exists after rollback, stat error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(workspace, "c.txt")); !os.IsNotExist(err) {
		t.Fatalf("c.txt exists after rollback, stat error = %v", err)
	}

	latest, ok, err := store.GetLatestFileCheckpoint(ctx, "sess_1")
	if err != nil {
		t.Fatalf("GetLatestFileCheckpoint() error = %v", err)
	}
	if !ok || latest.ToolName != "rollback" {
		t.Fatalf("latest checkpoint = %#v, %v; want rollback checkpoint", latest, ok)
	}
	if !strings.Contains(latest.DiffSummary, "a.txt") || !strings.Contains(latest.DiffSummary, "c.txt") {
		t.Fatalf("rollback diff summary = %q, want reverted files", latest.DiffSummary)
	}
}

func writeTestFile(t *testing.T, root, relPath, content string) {
	t.Helper()
	path := filepath.Join(root, relPath)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", relPath, err)
	}
}

func readTestFile(t *testing.T, root, relPath string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, relPath))
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", relPath, err)
	}
	return string(data)
}

func hasFileCheckpointPath(paths []string, prefix string) bool {
	for _, path := range paths {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}
