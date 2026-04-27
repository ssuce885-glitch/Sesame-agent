package sqlite

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"go-agent/internal/types"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()

	store, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})
	return store
}

func TestSimpleAutomationRunRoundTrip(t *testing.T) {
	store := newTestStore(t)
	run := types.SimpleAutomationRun{
		AutomationID: "auto_1",
		Owner:        "role:log_repairer",
		DedupeKey:    "file:a.txt",
		TaskID:       "task_1",
		LastStatus:   "success",
		LastSummary:  "deleted a.txt",
	}
	if err := store.UpsertSimpleAutomationRun(context.Background(), run); err != nil {
		t.Fatal(err)
	}
	got, ok, err := store.GetSimpleAutomationRun(context.Background(), "auto_1", "file:a.txt")
	if err != nil || !ok {
		t.Fatalf("GetSimpleAutomationRun() ok=%v err=%v", ok, err)
	}
	if got.AutomationID != "auto_1" {
		t.Fatalf("AutomationID = %q", got.AutomationID)
	}
	if got.DedupeKey != "file:a.txt" {
		t.Fatalf("DedupeKey = %q", got.DedupeKey)
	}
	if got.Owner != "role:log_repairer" {
		t.Fatalf("Owner = %q", got.Owner)
	}
	if got.TaskID != "task_1" {
		t.Fatalf("TaskID = %q", got.TaskID)
	}
	if got.LastStatus != "success" {
		t.Fatalf("LastStatus = %q", got.LastStatus)
	}
	if got.LastSummary != "deleted a.txt" {
		t.Fatalf("LastSummary = %q", got.LastSummary)
	}
}

func TestSimpleAutomationRunConflictUpdate(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	if err := store.UpsertSimpleAutomationRun(ctx, types.SimpleAutomationRun{
		AutomationID: "auto_1",
		DedupeKey:    "file:a.txt",
		Owner:        "role:log_repairer",
		TaskID:       "task_1",
		LastStatus:   "success",
		LastSummary:  "deleted a.txt",
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertSimpleAutomationRun(ctx, types.SimpleAutomationRun{
		AutomationID: "auto_1",
		DedupeKey:    "file:a.txt",
		Owner:        "role:supervisor",
		TaskID:       "task_2",
		LastStatus:   "failure",
		LastSummary:  "failed deleting a.txt",
	}); err != nil {
		t.Fatal(err)
	}

	got, ok, err := store.GetSimpleAutomationRun(ctx, "auto_1", "file:a.txt")
	if err != nil || !ok {
		t.Fatalf("GetSimpleAutomationRun() ok=%v err=%v", ok, err)
	}
	if got.Owner != "role:supervisor" {
		t.Fatalf("Owner = %q", got.Owner)
	}
	if got.TaskID != "task_2" {
		t.Fatalf("TaskID = %q", got.TaskID)
	}
	if got.LastStatus != "failure" {
		t.Fatalf("LastStatus = %q", got.LastStatus)
	}
	if got.LastSummary != "failed deleting a.txt" {
		t.Fatalf("LastSummary = %q", got.LastSummary)
	}
}

func TestSimpleAutomationRunClaimIsDedupeKeyAtomic(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	claimed, err := store.ClaimSimpleAutomationRun(ctx, types.SimpleAutomationRun{
		AutomationID: "auto_1",
		DedupeKey:    "file:a.txt",
		Owner:        "role:log_repairer",
		LastStatus:   "running",
		LastSummary:  "detected a.txt",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !claimed {
		t.Fatal("first claim returned false")
	}

	claimed, err = store.ClaimSimpleAutomationRun(ctx, types.SimpleAutomationRun{
		AutomationID: "auto_1",
		DedupeKey:    "file:a.txt",
		Owner:        "role:log_repairer",
		LastStatus:   "running",
		LastSummary:  "duplicate signal",
	})
	if err != nil {
		t.Fatal(err)
	}
	if claimed {
		t.Fatal("duplicate claim returned true")
	}

	got, ok, err := store.GetSimpleAutomationRun(ctx, "auto_1", "file:a.txt")
	if err != nil || !ok {
		t.Fatalf("GetSimpleAutomationRun() ok=%v err=%v", ok, err)
	}
	if got.LastSummary != "detected a.txt" {
		t.Fatalf("duplicate claim overwrote LastSummary = %q", got.LastSummary)
	}

	if err := store.UpsertSimpleAutomationRun(ctx, types.SimpleAutomationRun{
		AutomationID: "auto_1",
		DedupeKey:    "file:a.txt",
		Owner:        "role:log_repairer",
		TaskID:       "task_1",
		LastStatus:   "success",
		LastSummary:  "deleted a.txt",
	}); err != nil {
		t.Fatal(err)
	}
	claimed, err = store.ClaimSimpleAutomationRun(ctx, types.SimpleAutomationRun{
		AutomationID: "auto_1",
		DedupeKey:    "file:a.txt",
		Owner:        "role:log_repairer",
		LastStatus:   "running",
		LastSummary:  "detected a.txt again",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !claimed {
		t.Fatal("claim after terminal status returned false")
	}

	got, ok, err = store.GetSimpleAutomationRun(ctx, "auto_1", "file:a.txt")
	if err != nil || !ok {
		t.Fatalf("GetSimpleAutomationRun() ok=%v err=%v", ok, err)
	}
	if got.LastStatus != "running" {
		t.Fatalf("LastStatus after re-claim = %q", got.LastStatus)
	}
	if got.LastSummary != "detected a.txt again" {
		t.Fatalf("LastSummary after re-claim = %q", got.LastSummary)
	}
	if got.TaskID != "" {
		t.Fatalf("TaskID after re-claim = %q, want empty until task creation", got.TaskID)
	}
}

func TestSimpleAutomationRunSkipsInvalidKeys(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	if err := store.UpsertSimpleAutomationRun(ctx, types.SimpleAutomationRun{
		AutomationID: "   ",
		DedupeKey:    "file:a.txt",
		Owner:        "role:log_repairer",
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertSimpleAutomationRun(ctx, types.SimpleAutomationRun{
		AutomationID: "auto_1",
		DedupeKey:    "   ",
		Owner:        "role:log_repairer",
	}); err != nil {
		t.Fatal(err)
	}

	var rowCount int
	if err := store.DB().QueryRowContext(ctx, `select count(*) from automation_simple_runs`).Scan(&rowCount); err != nil {
		t.Fatal(err)
	}
	if rowCount != 0 {
		t.Fatalf("rowCount = %d", rowCount)
	}
}

func TestSimpleAutomationRunNormalizesTimestampOrdering(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	createdAt := time.Date(2026, time.April, 22, 12, 0, 0, 0, time.UTC)
	updatedAt := createdAt.Add(-5 * time.Minute)
	if err := store.UpsertSimpleAutomationRun(ctx, types.SimpleAutomationRun{
		AutomationID: "auto_1",
		DedupeKey:    "file:a.txt",
		Owner:        "role:log_repairer",
		CreatedAt:    createdAt,
		UpdatedAt:    updatedAt,
	}); err != nil {
		t.Fatal(err)
	}

	got, ok, err := store.GetSimpleAutomationRun(ctx, "auto_1", "file:a.txt")
	if err != nil || !ok {
		t.Fatalf("GetSimpleAutomationRun() ok=%v err=%v", ok, err)
	}
	if got.UpdatedAt.Before(got.CreatedAt) {
		t.Fatalf("UpdatedAt %s before CreatedAt %s", got.UpdatedAt.Format(time.RFC3339Nano), got.CreatedAt.Format(time.RFC3339Nano))
	}
}

func TestSimpleAutomationRunLookupByTaskID(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	if err := store.UpsertSimpleAutomationRun(ctx, types.SimpleAutomationRun{
		AutomationID: "auto_1",
		DedupeKey:    "file:a.txt",
		Owner:        "role:log_repairer",
		TaskID:       "task_1",
		LastStatus:   "running",
		LastSummary:  "detected file a.txt",
	}); err != nil {
		t.Fatal(err)
	}

	got, ok, err := store.GetSimpleAutomationRunByTaskID(ctx, "task_1")
	if err != nil || !ok {
		t.Fatalf("GetSimpleAutomationRunByTaskID() ok=%v err=%v", ok, err)
	}
	if got.AutomationID != "auto_1" {
		t.Fatalf("AutomationID = %q", got.AutomationID)
	}
	if got.DedupeKey != "file:a.txt" {
		t.Fatalf("DedupeKey = %q", got.DedupeKey)
	}

	if _, ok, err := store.GetSimpleAutomationRunByTaskID(ctx, "   "); err != nil || ok {
		t.Fatalf("GetSimpleAutomationRunByTaskID(blank) ok=%v err=%v", ok, err)
	}
}
