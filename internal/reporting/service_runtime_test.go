package reporting

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"go-agent/internal/store/sqlite"
)

func TestTickRunsCleanupHourly(t *testing.T) {
	ctx := context.Background()
	store, err := sqlite.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})

	now := time.Date(2026, 4, 28, 12, 0, 0, 0, time.UTC)
	cleanup := &recordingCleanupStore{}
	service := NewService(store)
	service.SetWorkspaceRoot("/workspace")
	service.SetCleanupStore(cleanup)
	service.SetClock(func() time.Time { return now })

	if err := service.Tick(ctx); err != nil {
		t.Fatalf("Tick() error = %v", err)
	}
	cleanup.assertCalls(t, 1)
	cleanup.assertWorkspaceRoot(t, "/workspace")
	cleanup.assertCutoff(t, "deprecated_memories", now.Add(-90*24*time.Hour))
	cleanup.assertCutoff(t, "reports", now.Add(-30*24*time.Hour))
	cleanup.assertCutoff(t, "digest_records", now.Add(-30*24*time.Hour))
	cleanup.assertCutoff(t, "report_deliveries", now.Add(-14*24*time.Hour))
	cleanup.assertCutoff(t, "child_agent_results", now.Add(-60*24*time.Hour))
	if cleanup.keepCount != 10 {
		t.Fatalf("conversation compaction keepCount = %d, want 10", cleanup.keepCount)
	}

	now = now.Add(30 * time.Minute)
	if err := service.Tick(ctx); err != nil {
		t.Fatalf("second Tick() error = %v", err)
	}
	cleanup.assertCalls(t, 1)

	now = now.Add(31 * time.Minute)
	if err := service.Tick(ctx); err != nil {
		t.Fatalf("third Tick() error = %v", err)
	}
	cleanup.assertCalls(t, 2)
}

type recordingCleanupStore struct {
	calls         map[string]int
	cutoffs       map[string]time.Time
	workspaceRoot string
	keepCount     int
}

func (s *recordingCleanupStore) CleanupDeprecatedMemories(_ context.Context, olderThan time.Time) (int64, error) {
	s.record("deprecated_memories", "", olderThan)
	return 0, nil
}

func (s *recordingCleanupStore) CleanupOldReports(_ context.Context, workspaceRoot string, olderThan time.Time) (int64, error) {
	s.record("reports", workspaceRoot, olderThan)
	return 0, nil
}

func (s *recordingCleanupStore) CleanupOldDigestRecords(_ context.Context, workspaceRoot string, olderThan time.Time) (int64, error) {
	s.record("digest_records", workspaceRoot, olderThan)
	return 0, nil
}

func (s *recordingCleanupStore) CleanupOldReportDeliveries(_ context.Context, workspaceRoot string, olderThan time.Time) (int64, error) {
	s.record("report_deliveries", workspaceRoot, olderThan)
	return 0, nil
}

func (s *recordingCleanupStore) CleanupOldChildAgentResults(_ context.Context, olderThan time.Time) (int64, error) {
	s.record("child_agent_results", "", olderThan)
	return 0, nil
}

func (s *recordingCleanupStore) CleanupOldConversationCompactions(_ context.Context, keepCount int) (int64, error) {
	if s.calls == nil {
		s.calls = make(map[string]int)
	}
	s.calls["conversation_compactions"]++
	s.keepCount = keepCount
	return 0, nil
}

func (s *recordingCleanupStore) record(name, workspaceRoot string, olderThan time.Time) {
	if s.calls == nil {
		s.calls = make(map[string]int)
	}
	if s.cutoffs == nil {
		s.cutoffs = make(map[string]time.Time)
	}
	s.calls[name]++
	s.cutoffs[name] = olderThan
	if workspaceRoot != "" {
		s.workspaceRoot = workspaceRoot
	}
}

func (s *recordingCleanupStore) assertCalls(t *testing.T, want int) {
	t.Helper()
	names := []string{
		"deprecated_memories",
		"reports",
		"digest_records",
		"report_deliveries",
		"child_agent_results",
		"conversation_compactions",
	}
	for _, name := range names {
		if got := s.calls[name]; got != want {
			t.Fatalf("%s calls = %d, want %d", name, got, want)
		}
	}
}

func (s *recordingCleanupStore) assertWorkspaceRoot(t *testing.T, want string) {
	t.Helper()
	if s.workspaceRoot != want {
		t.Fatalf("workspaceRoot = %q, want %q", s.workspaceRoot, want)
	}
}

func (s *recordingCleanupStore) assertCutoff(t *testing.T, name string, want time.Time) {
	t.Helper()
	if got := s.cutoffs[name]; !got.Equal(want) {
		t.Fatalf("%s cutoff = %s, want %s", name, got.Format(time.RFC3339Nano), want.Format(time.RFC3339Nano))
	}
}
