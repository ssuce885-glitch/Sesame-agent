package daemon

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go-agent/internal/store/sqlite"
	"go-agent/internal/stream"
	"go-agent/internal/task"
	"go-agent/internal/types"
)

type fakeSimpleAutomationWatcherInstaller struct {
	reinstalled []types.AutomationSpec
}

func (w *fakeSimpleAutomationWatcherInstaller) Reinstall(_ context.Context, spec types.AutomationSpec) (types.AutomationWatcherRuntime, error) {
	w.reinstalled = append(w.reinstalled, spec)
	return types.AutomationWatcherRuntime{AutomationID: spec.ID, State: types.AutomationWatcherStateRunning}, nil
}

func newDaemonTestStore(t *testing.T) *sqlite.Store {
	t.Helper()

	store, err := sqlite.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})
	return store
}

func writeRoleForNotifierTest(t *testing.T, workspaceRoot, roleID, prompt string) {
	t.Helper()

	roleDir := filepath.Join(workspaceRoot, "roles", roleID)
	if err := os.MkdirAll(roleDir, 0o755); err != nil {
		t.Fatalf("MkdirAll role dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(roleDir, "role.yaml"), []byte("display_name: "+roleID+"\n"), 0o644); err != nil {
		t.Fatalf("write role.yaml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(roleDir, "prompt.md"), []byte(prompt+"\n"), 0o644); err != nil {
		t.Fatalf("write prompt.md: %v", err)
	}
}

func simpleAutomationSpecForTest(workspaceRoot, owner string) types.AutomationSpec {
	return types.AutomationSpec{
		ID:               "auto_1",
		Title:            "Simple Repair",
		WorkspaceRoot:    workspaceRoot,
		Goal:             "repair detected issues",
		State:            types.AutomationStateActive,
		Mode:             types.AutomationModeSimple,
		Owner:            owner,
		WatcherLifecycle: json.RawMessage(`{}`),
		RetriggerPolicy:  json.RawMessage(`{}`),
	}
}

func completedSimpleAutomationTask(taskID, workspaceRoot string, status task.TaskStatus, outcome types.ChildAgentOutcome, summary, resultText string, observedAt time.Time) task.Task {
	observedAt = observedAt.UTC()
	return task.Task{
		ID:                 taskID,
		Type:               task.TaskTypeAgent,
		Status:             status,
		Kind:               "automation_simple",
		Command:            "simple automation prompt",
		Description:        "simple automation match: auto_1",
		WorkspaceRoot:      workspaceRoot,
		Outcome:            outcome,
		OutcomeSummary:     summary,
		FinalResultKind:    task.FinalResultKindAssistantText,
		FinalResultText:    resultText,
		FinalResultReadyAt: &observedAt,
	}
}

func TestSimpleAutomationTaskTerminalDefaultsReportDeliveryToMainAgentAndUpdatesRun(t *testing.T) {
	ctx := context.Background()
	store := newDaemonTestStore(t)
	now := time.Date(2026, time.April, 22, 20, 0, 0, 0, time.UTC)
	notifier := taskTerminalNotifier{
		store: store,
		now:   func() time.Time { return now },
	}

	spec := simpleAutomationSpecForTest(filepath.Join(t.TempDir(), "workspace"), "role:log_repairer")
	if err := store.UpsertAutomation(ctx, spec); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertSimpleAutomationRun(ctx, types.SimpleAutomationRun{
		AutomationID: spec.ID,
		DedupeKey:    "file:a.txt",
		Owner:        spec.Owner,
		TaskID:       "task_1",
		LastStatus:   "running",
		LastSummary:  "new file detected",
		CreatedAt:    now.Add(-time.Minute),
		UpdatedAt:    now.Add(-time.Minute),
	}); err != nil {
		t.Fatal(err)
	}

	completed := completedSimpleAutomationTask(
		"task_1",
		spec.WorkspaceRoot,
		task.TaskStatusCompleted,
		types.ChildAgentOutcomeSuccess,
		"deleted file:a.txt",
		"deleted file:a.txt",
		now,
	)
	if err := notifier.NotifyTaskTerminal(ctx, completed); err != nil {
		t.Fatal(err)
	}

	run, ok, err := store.GetSimpleAutomationRun(ctx, spec.ID, "file:a.txt")
	if err != nil || !ok {
		t.Fatalf("GetSimpleAutomationRun() ok=%v err=%v", ok, err)
	}
	if run.TaskID != "task_1" {
		t.Fatalf("TaskID = %q", run.TaskID)
	}
	if run.LastStatus != "success" {
		t.Fatalf("LastStatus = %q", run.LastStatus)
	}
	if run.LastSummary != "deleted file:a.txt" {
		t.Fatalf("LastSummary = %q", run.LastSummary)
	}
	if run.UpdatedAt.Before(run.CreatedAt) {
		t.Fatalf("UpdatedAt %s before CreatedAt %s", run.UpdatedAt.Format(time.RFC3339Nano), run.CreatedAt.Format(time.RFC3339Nano))
	}

	mainSessionID, ok, err := store.GetRoleSessionID(ctx, spec.WorkspaceRoot, types.SessionRoleMainParent)
	if err != nil || !ok {
		t.Fatalf("GetRoleSessionID(main_parent) ok=%v err=%v", ok, err)
	}
	reports, err := store.ListReportDeliveryItems(ctx, mainSessionID)
	if err != nil {
		t.Fatal(err)
	}
	if len(reports) != 1 {
		t.Fatalf("main queued reports = %d", len(reports))
	}
	if reports[0].Envelope.Status != "completed" {
		t.Fatalf("Status = %q", reports[0].Envelope.Status)
	}
	if reports[0].SourceID != "task_1" {
		t.Fatalf("TaskID = %q", reports[0].SourceID)
	}
	if _, ok, err := store.GetSpecialistSessionID(ctx, spec.WorkspaceRoot, "log_repairer"); err != nil {
		t.Fatal(err)
	} else if ok {
		t.Fatal("unexpected specialist owner report session for default automation delivery")
	}
}

func TestScheduledReportTaskTerminalQueuesMainAgentReport(t *testing.T) {
	ctx := context.Background()
	store := newDaemonTestStore(t)
	now := time.Date(2026, time.April, 24, 16, 0, 0, 0, time.UTC)
	workspaceRoot := "/tmp/workspace"
	mainSession, _, _, err := store.EnsureRoleSession(ctx, workspaceRoot, types.SessionRoleMainParent)
	if err != nil {
		t.Fatal(err)
	}
	specialistSession, _, _, err := store.EnsureSpecialistSession(ctx, workspaceRoot, "reddit_monitor", "watch reddit", nil)
	if err != nil {
		t.Fatal(err)
	}

	notifier := taskTerminalNotifier{
		store: store,
		bus:   stream.NewBus(),
		now:   func() time.Time { return now },
	}
	completed := task.Task{
		ID:                 "task_report_1",
		Type:               task.TaskTypeAgent,
		Status:             task.TaskStatusCompleted,
		Kind:               "scheduled_report",
		Command:            "check reddit hot topics",
		Description:        "Reddit hot topics",
		ScheduledJobID:     "cron_reddit",
		ParentSessionID:    specialistSession.ID,
		WorkspaceRoot:      workspaceRoot,
		FinalResultKind:    task.FinalResultKindAssistantText,
		FinalResultText:    "topic A\n topic B",
		FinalResultReadyAt: &now,
	}
	if err := notifier.NotifyTaskTerminal(ctx, completed); err != nil {
		t.Fatal(err)
	}

	mainReports, err := store.ListReportDeliveryItems(ctx, mainSession.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(mainReports) != 1 {
		t.Fatalf("main queued reports = %d", len(mainReports))
	}
	if mainReports[0].SourceKind != types.ReportSourceTaskResult {
		t.Fatalf("Source = %q", mainReports[0].SourceKind)
	}
	if mainReports[0].SourceID != "task_report_1" {
		t.Fatalf("TaskID = %q", mainReports[0].SourceID)
	}
	if len(mainReports[0].Envelope.Sections) == 0 || mainReports[0].Envelope.Sections[0].Text != "topic A\n topic B" {
		t.Fatalf("ResultText = %#v", mainReports[0].Envelope.Sections)
	}

	specialistReports, err := store.ListReportDeliveryItems(ctx, specialistSession.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(specialistReports) != 0 {
		t.Fatalf("specialist queued reports = %d", len(specialistReports))
	}

	reportItems, err := store.ListWorkspaceReportDeliveryItems(ctx, workspaceRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(reportItems) != 1 {
		t.Fatalf("report items = %d", len(reportItems))
	}
}

func TestSimpleAutomationTaskTerminalResumesWatcherAfterSuccessContinue(t *testing.T) {
	ctx := context.Background()
	store := newDaemonTestStore(t)
	now := time.Date(2026, time.April, 22, 20, 0, 30, 0, time.UTC)
	watcher := &fakeSimpleAutomationWatcherInstaller{}
	notifier := taskTerminalNotifier{
		store:   store,
		watcher: watcher,
		now:     func() time.Time { return now },
	}

	spec := simpleAutomationSpecForTest(filepath.Join(t.TempDir(), "workspace"), "role:log_repairer")
	spec.SimplePolicy = types.SimpleAutomationPolicy{OnSuccess: "continue", OnFailure: "pause", OnBlocked: "escalate"}
	writeRoleForNotifierTest(t, spec.WorkspaceRoot, "log_repairer", "Repair logs")
	if err := store.UpsertAutomation(ctx, spec); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertSimpleAutomationRun(ctx, types.SimpleAutomationRun{
		AutomationID: spec.ID,
		DedupeKey:    "file:a.txt",
		Owner:        spec.Owner,
		TaskID:       "task_1",
		LastStatus:   "running",
		LastSummary:  "new file detected",
		CreatedAt:    now.Add(-time.Minute),
		UpdatedAt:    now.Add(-time.Minute),
	}); err != nil {
		t.Fatal(err)
	}

	completed := completedSimpleAutomationTask(
		"task_1",
		spec.WorkspaceRoot,
		task.TaskStatusCompleted,
		types.ChildAgentOutcomeSuccess,
		"deleted file:a.txt",
		"deleted file:a.txt",
		now,
	)
	if err := notifier.NotifyTaskTerminal(ctx, completed); err != nil {
		t.Fatal(err)
	}

	if len(watcher.reinstalled) != 1 {
		t.Fatalf("watcher reinstalls = %d", len(watcher.reinstalled))
	}
	if watcher.reinstalled[0].ID != spec.ID {
		t.Fatalf("reinstalled automation = %q", watcher.reinstalled[0].ID)
	}
}

func TestSimpleAutomationTaskTerminalKeepsWatcherPausedAfterBlockedEscalate(t *testing.T) {
	ctx := context.Background()
	store := newDaemonTestStore(t)
	now := time.Date(2026, time.April, 22, 20, 0, 45, 0, time.UTC)
	watcher := &fakeSimpleAutomationWatcherInstaller{}
	notifier := taskTerminalNotifier{
		store:   store,
		watcher: watcher,
		now:     func() time.Time { return now },
	}

	spec := simpleAutomationSpecForTest(filepath.Join(t.TempDir(), "workspace"), "role:log_repairer")
	spec.SimplePolicy = types.SimpleAutomationPolicy{OnSuccess: "continue", OnFailure: "pause", OnBlocked: "escalate"}
	writeRoleForNotifierTest(t, spec.WorkspaceRoot, "log_repairer", "Repair logs")
	if err := store.UpsertAutomation(ctx, spec); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertSimpleAutomationRun(ctx, types.SimpleAutomationRun{
		AutomationID: spec.ID,
		DedupeKey:    "file:a.txt",
		Owner:        spec.Owner,
		TaskID:       "task_1",
		LastStatus:   "running",
		LastSummary:  "new file detected",
		CreatedAt:    now.Add(-time.Minute),
		UpdatedAt:    now.Add(-time.Minute),
	}); err != nil {
		t.Fatal(err)
	}

	completed := completedSimpleAutomationTask(
		"task_1",
		spec.WorkspaceRoot,
		task.TaskStatusFailed,
		types.ChildAgentOutcomeBlocked,
		"blocked waiting for input",
		"blocked waiting for input",
		now,
	)
	if err := notifier.NotifyTaskTerminal(ctx, completed); err != nil {
		t.Fatal(err)
	}

	if len(watcher.reinstalled) != 0 {
		t.Fatalf("watcher reinstalls = %d", len(watcher.reinstalled))
	}
}

func TestSimpleAutomationTaskTerminalDoesNotResumeWatcherWhenAutomationPaused(t *testing.T) {
	ctx := context.Background()
	store := newDaemonTestStore(t)
	now := time.Date(2026, time.April, 22, 20, 0, 50, 0, time.UTC)
	watcher := &fakeSimpleAutomationWatcherInstaller{}
	notifier := taskTerminalNotifier{
		store:   store,
		watcher: watcher,
		now:     func() time.Time { return now },
	}

	spec := simpleAutomationSpecForTest(filepath.Join(t.TempDir(), "workspace"), "role:log_repairer")
	spec.State = types.AutomationStatePaused
	spec.SimplePolicy = types.SimpleAutomationPolicy{OnSuccess: "continue", OnFailure: "pause", OnBlocked: "escalate"}
	writeRoleForNotifierTest(t, spec.WorkspaceRoot, "log_repairer", "Repair logs")
	if err := store.UpsertAutomation(ctx, spec); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertSimpleAutomationRun(ctx, types.SimpleAutomationRun{
		AutomationID: spec.ID,
		DedupeKey:    "file:a.txt",
		Owner:        spec.Owner,
		TaskID:       "task_1",
		LastStatus:   "running",
		LastSummary:  "new file detected",
		CreatedAt:    now.Add(-time.Minute),
		UpdatedAt:    now.Add(-time.Minute),
	}); err != nil {
		t.Fatal(err)
	}

	completed := completedSimpleAutomationTask(
		"task_1",
		spec.WorkspaceRoot,
		task.TaskStatusCompleted,
		types.ChildAgentOutcomeSuccess,
		"deleted file:a.txt",
		"deleted file:a.txt",
		now,
	)
	if err := notifier.NotifyTaskTerminal(ctx, completed); err != nil {
		t.Fatal(err)
	}

	if len(watcher.reinstalled) != 0 {
		t.Fatalf("watcher reinstalls = %d", len(watcher.reinstalled))
	}
}

func TestSimpleAutomationTaskTerminalReportTargetMainAgent(t *testing.T) {
	ctx := context.Background()
	store := newDaemonTestStore(t)
	now := time.Date(2026, time.April, 22, 20, 1, 0, 0, time.UTC)
	notifier := taskTerminalNotifier{
		store: store,
		now:   func() time.Time { return now },
	}

	spec := simpleAutomationSpecForTest(filepath.Join(t.TempDir(), "workspace"), "role:log_repairer")
	spec.ReportTarget = "main_agent"
	if err := store.UpsertAutomation(ctx, spec); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertSimpleAutomationRun(ctx, types.SimpleAutomationRun{
		AutomationID: spec.ID,
		DedupeKey:    "file:b.txt",
		Owner:        spec.Owner,
		TaskID:       "task_2",
		LastStatus:   "running",
		LastSummary:  "new file detected",
		CreatedAt:    now.Add(-time.Minute),
		UpdatedAt:    now.Add(-time.Minute),
	}); err != nil {
		t.Fatal(err)
	}

	completed := completedSimpleAutomationTask(
		"task_2",
		spec.WorkspaceRoot,
		task.TaskStatusCompleted,
		types.ChildAgentOutcomeSuccess,
		"deleted file:b.txt",
		"deleted file:b.txt",
		now,
	)
	if err := notifier.NotifyTaskTerminal(ctx, completed); err != nil {
		t.Fatal(err)
	}

	mainSessionID, ok, err := store.GetRoleSessionID(ctx, spec.WorkspaceRoot, types.SessionRoleMainParent)
	if err != nil || !ok {
		t.Fatalf("GetRoleSessionID() ok=%v err=%v", ok, err)
	}
	reports, err := store.ListReportDeliveryItems(ctx, mainSessionID)
	if err != nil {
		t.Fatal(err)
	}
	if len(reports) != 1 {
		t.Fatalf("main queued reports = %d", len(reports))
	}

	if _, ok, err := store.GetSpecialistSessionID(ctx, spec.WorkspaceRoot, "log_repairer"); err != nil {
		t.Fatal(err)
	} else if ok {
		t.Fatal("unexpected specialist owner session for report_target=main_agent")
	}
}

func TestSimpleAutomationTaskTerminalEscalationPolicies(t *testing.T) {
	tests := []struct {
		name              string
		runDedupeKey      string
		policy            types.SimpleAutomationPolicy
		outcome           types.ChildAgentOutcome
		expectedRunStatus string
	}{
		{
			name:              "failure escalate still reports to main agent",
			runDedupeKey:      "file:c.txt",
			policy:            types.SimpleAutomationPolicy{OnFailure: "escalate"},
			outcome:           types.ChildAgentOutcomeFailure,
			expectedRunStatus: "failure",
		},
		{
			name:              "blocked escalate reports once to main agent",
			runDedupeKey:      "file:d.txt",
			policy:            types.SimpleAutomationPolicy{OnBlocked: "escalate"},
			outcome:           types.ChildAgentOutcomeBlocked,
			expectedRunStatus: "blocked",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			store := newDaemonTestStore(t)
			now := time.Date(2026, time.April, 22, 20, 2, 0, 0, time.UTC)
			notifier := taskTerminalNotifier{
				store: store,
				now:   func() time.Time { return now },
			}

			spec := simpleAutomationSpecForTest(filepath.Join(t.TempDir(), "workspace"), "role:worker")
			spec.SimplePolicy = tc.policy
			if err := store.UpsertAutomation(ctx, spec); err != nil {
				t.Fatal(err)
			}
			if err := store.UpsertSimpleAutomationRun(ctx, types.SimpleAutomationRun{
				AutomationID: spec.ID,
				DedupeKey:    tc.runDedupeKey,
				Owner:        spec.Owner,
				TaskID:       "task_3",
				LastStatus:   "running",
				LastSummary:  "new file detected",
				CreatedAt:    now.Add(-time.Minute),
				UpdatedAt:    now.Add(-time.Minute),
			}); err != nil {
				t.Fatal(err)
			}

			completed := completedSimpleAutomationTask(
				"task_3",
				spec.WorkspaceRoot,
				task.TaskStatusFailed,
				tc.outcome,
				"automation task ended",
				"automation task ended",
				now,
			)
			if err := notifier.NotifyTaskTerminal(ctx, completed); err != nil {
				t.Fatal(err)
			}

			run, ok, err := store.GetSimpleAutomationRun(ctx, spec.ID, tc.runDedupeKey)
			if err != nil || !ok {
				t.Fatalf("GetSimpleAutomationRun() ok=%v err=%v", ok, err)
			}
			if run.LastStatus != tc.expectedRunStatus {
				t.Fatalf("LastStatus = %q", run.LastStatus)
			}

			mainSessionID, ok, err := store.GetRoleSessionID(ctx, spec.WorkspaceRoot, types.SessionRoleMainParent)
			if err != nil || !ok {
				t.Fatalf("GetRoleSessionID(main_parent) ok=%v err=%v", ok, err)
			}
			mainReports, err := store.ListReportDeliveryItems(ctx, mainSessionID)
			if err != nil {
				t.Fatal(err)
			}
			if len(mainReports) != 1 {
				t.Fatalf("main queued reports = %d", len(mainReports))
			}
			if _, ok, err := store.GetSpecialistSessionID(ctx, spec.WorkspaceRoot, "worker"); err != nil {
				t.Fatal(err)
			} else if ok {
				t.Fatal("unexpected specialist owner report session for automation escalation")
			}
		})
	}
}

func TestSimpleAutomationTaskTerminalIgnoresNonSimpleTaskKind(t *testing.T) {
	ctx := context.Background()
	store := newDaemonTestStore(t)
	now := time.Date(2026, time.April, 22, 20, 3, 0, 0, time.UTC)
	notifier := taskTerminalNotifier{
		store: store,
		now:   func() time.Time { return now },
	}

	spec := simpleAutomationSpecForTest(filepath.Join(t.TempDir(), "workspace"), "role:log_repairer")
	if err := store.UpsertAutomation(ctx, spec); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertSimpleAutomationRun(ctx, types.SimpleAutomationRun{
		AutomationID: spec.ID,
		DedupeKey:    "file:e.txt",
		Owner:        spec.Owner,
		TaskID:       "task_4",
		LastStatus:   "running",
		LastSummary:  "new file detected",
		CreatedAt:    now.Add(-time.Minute),
		UpdatedAt:    now.Add(-time.Minute),
	}); err != nil {
		t.Fatal(err)
	}

	completed := completedSimpleAutomationTask(
		"task_4",
		spec.WorkspaceRoot,
		task.TaskStatusCompleted,
		types.ChildAgentOutcomeSuccess,
		"deleted file:e.txt",
		"deleted file:e.txt",
		now,
	)
	completed.Kind = "automation_dispatch"
	if err := notifier.NotifyTaskTerminal(ctx, completed); err != nil {
		t.Fatal(err)
	}

	run, ok, err := store.GetSimpleAutomationRun(ctx, spec.ID, "file:e.txt")
	if err != nil || !ok {
		t.Fatalf("GetSimpleAutomationRun() ok=%v err=%v", ok, err)
	}
	if run.LastStatus != "running" {
		t.Fatalf("LastStatus = %q", run.LastStatus)
	}
	if run.LastSummary != "new file detected" {
		t.Fatalf("LastSummary = %q", run.LastSummary)
	}
}

func TestSimpleAutomationTaskTerminalSourceRoleRequiresInstalledRole(t *testing.T) {
	ctx := context.Background()
	store := newDaemonTestStore(t)
	now := time.Date(2026, time.April, 22, 20, 4, 0, 0, time.UTC)
	notifier := taskTerminalNotifier{
		store: store,
		now:   func() time.Time { return now },
	}

	spec := simpleAutomationSpecForTest(filepath.Join(t.TempDir(), "workspace"), "role:log_repairer")
	if err := store.UpsertAutomation(ctx, spec); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertSimpleAutomationRun(ctx, types.SimpleAutomationRun{
		AutomationID: spec.ID,
		DedupeKey:    "file:f.txt",
		Owner:        spec.Owner,
		TaskID:       "task_missing_role",
		LastStatus:   "running",
		LastSummary:  "new file detected",
		CreatedAt:    now.Add(-time.Minute),
		UpdatedAt:    now.Add(-time.Minute),
	}); err != nil {
		t.Fatal(err)
	}

	completed := completedSimpleAutomationTask(
		"task_missing_role",
		spec.WorkspaceRoot,
		task.TaskStatusCompleted,
		types.ChildAgentOutcomeSuccess,
		"deleted file:f.txt",
		"deleted file:f.txt",
		now,
	)
	completed.TargetRole = "log_repairer"
	err := notifier.NotifyTaskTerminal(ctx, completed)
	if err == nil {
		t.Fatal("expected missing specialist role error")
	}
	if !strings.Contains(err.Error(), "specialist role is not installed: log_repairer") {
		t.Fatalf("error = %v, want missing specialist role error", err)
	}
}
