package sqlite

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"go-agent/internal/types"
)

func TestFileCheckpointRoundTripAndLatest(t *testing.T) {
	ctx := context.Background()
	store, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	createdAt := time.Date(2026, time.April, 28, 12, 0, 0, 0, time.UTC)
	first := types.FileCheckpoint{
		ID:            "filecp_1",
		SessionID:     "sess_1",
		TurnID:        "turn_1",
		ToolCallID:    "call_1",
		ToolName:      "file_write",
		Reason:        `{"path":"a.txt"}`,
		GitCommitHash: "abc123",
		FilesChanged:  []string{"a.txt"},
		DiffSummary:   "a.txt | 1 +",
		CreatedAt:     createdAt,
	}
	second := types.FileCheckpoint{
		ID:                 "filecp_2",
		SessionID:          "sess_1",
		TurnID:             "turn_2",
		ToolCallID:         "call_2",
		ToolName:           "apply_patch",
		Reason:             "patch",
		GitCommitHash:      "def456",
		FilesChanged:       []string{"a.txt", "b.txt"},
		DiffSummary:        "2 files changed",
		ParentCheckpointID: first.ID,
		CreatedAt:          createdAt.Add(time.Second),
	}
	if err := store.InsertFileCheckpoint(ctx, first); err != nil {
		t.Fatalf("InsertFileCheckpoint(first) error = %v", err)
	}
	if err := store.InsertFileCheckpoint(ctx, second); err != nil {
		t.Fatalf("InsertFileCheckpoint(second) error = %v", err)
	}

	got, ok, err := store.GetFileCheckpoint(ctx, second.ID)
	if err != nil {
		t.Fatalf("GetFileCheckpoint() error = %v", err)
	}
	if !ok {
		t.Fatal("GetFileCheckpoint() found no checkpoint")
	}
	if got.ID != second.ID || got.ParentCheckpointID != first.ID || got.GitCommitHash != second.GitCommitHash {
		t.Fatalf("checkpoint = %#v, want %#v", got, second)
	}
	if !reflect.DeepEqual(got.FilesChanged, second.FilesChanged) {
		t.Fatalf("FilesChanged = %#v, want %#v", got.FilesChanged, second.FilesChanged)
	}

	latest, ok, err := store.GetLatestFileCheckpoint(ctx, "sess_1")
	if err != nil {
		t.Fatalf("GetLatestFileCheckpoint() error = %v", err)
	}
	if !ok || latest.ID != second.ID {
		t.Fatalf("latest = %#v, %v; want %s", latest, ok, second.ID)
	}

	listed, err := store.ListFileCheckpointsBySession(ctx, "sess_1", 10)
	if err != nil {
		t.Fatalf("ListFileCheckpointsBySession() error = %v", err)
	}
	if len(listed) != 2 || listed[0].ID != second.ID || listed[1].ID != first.ID {
		t.Fatalf("listed = %#v, want newest first", listed)
	}
}
