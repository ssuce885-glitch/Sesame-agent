package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"go-agent/internal/engine"
	"go-agent/internal/store/sqlite"
	"go-agent/internal/types"
)

func TestFileCheckpointRoutesListDiffAndRollback(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}

	ctx := context.Background()
	deps := NewTestDependencies(t)
	store, ok := deps.Store.(*sqlite.Store)
	if !ok {
		t.Fatal("test dependencies store is not sqlite.Store")
	}
	workspace := t.TempDir()
	deps.WorkspaceRoot = workspace
	service := engine.NewFileCheckpointService(store, workspace)
	deps.FileCheckpoints = service

	session, _, _, err := store.EnsureRoleSession(ctx, workspace, types.SessionRoleMainParent)
	if err != nil {
		t.Fatalf("EnsureRoleSession() error = %v", err)
	}
	writeHTTPCheckpointFile(t, workspace, "a.txt", "one\n")
	first, err := service.CheckpointBeforeTool(ctx, session.ID, "turn_1", "call_1", "file_write", "first")
	if err != nil {
		t.Fatalf("CheckpointBeforeTool(first) error = %v", err)
	}
	writeHTTPCheckpointFile(t, workspace, "a.txt", "two\n")
	second, err := service.CheckpointBeforeTool(ctx, session.ID, "turn_2", "call_2", "apply_patch", "second")
	if err != nil {
		t.Fatalf("CheckpointBeforeTool(second) error = %v", err)
	}

	router := NewRouter(deps)
	listResp := httptest.NewRecorder()
	router.ServeHTTP(listResp, httptest.NewRequest(http.MethodGet, "/v1/session/checkpoints", nil))
	if listResp.Code != http.StatusOK {
		t.Fatalf("GET checkpoints status = %d, body = %s", listResp.Code, listResp.Body.String())
	}
	var listed fileCheckpointListResponse
	if err := json.Unmarshal(listResp.Body.Bytes(), &listed); err != nil {
		t.Fatalf("decode list response error = %v", err)
	}
	if len(listed.Checkpoints) != 2 || listed.Checkpoints[0].ID != second.ID {
		t.Fatalf("listed checkpoints = %#v, want newest first", listed.Checkpoints)
	}

	diffResp := httptest.NewRecorder()
	router.ServeHTTP(diffResp, httptest.NewRequest(http.MethodGet, "/v1/session/checkpoints/"+second.ID+"/diff", nil))
	if diffResp.Code != http.StatusOK {
		t.Fatalf("GET checkpoint diff status = %d, body = %s", diffResp.Code, diffResp.Body.String())
	}
	var diff fileCheckpointDiffResponse
	if err := json.Unmarshal(diffResp.Body.Bytes(), &diff); err != nil {
		t.Fatalf("decode diff response error = %v", err)
	}
	if diff.Checkpoint.ID != second.ID || !strings.Contains(diff.Diff, "a.txt") {
		t.Fatalf("diff response = %#v, want second checkpoint diff for a.txt", diff)
	}

	writeHTTPCheckpointFile(t, workspace, "a.txt", "three\n")
	rollbackResp := httptest.NewRecorder()
	router.ServeHTTP(rollbackResp, httptest.NewRequest(http.MethodPost, "/v1/session/checkpoints/"+first.ID+"/rollback", nil))
	if rollbackResp.Code != http.StatusOK {
		t.Fatalf("POST checkpoint rollback status = %d, body = %s", rollbackResp.Code, rollbackResp.Body.String())
	}
	if got := readHTTPCheckpointFile(t, workspace, "a.txt"); got != "one\n" {
		t.Fatalf("a.txt after rollback = %q, want one", got)
	}
}

func writeHTTPCheckpointFile(t *testing.T, root, relPath, content string) {
	t.Helper()
	path := filepath.Join(root, relPath)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", relPath, err)
	}
}

func readHTTPCheckpointFile(t *testing.T, root, relPath string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, relPath))
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", relPath, err)
	}
	return string(data)
}
