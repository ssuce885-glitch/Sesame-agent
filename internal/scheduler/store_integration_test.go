package scheduler

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"go-agent/internal/store/sqlite"
	"go-agent/internal/task"
	"go-agent/internal/types"
)

func setupSchedulerStoreTest(t *testing.T) (*Service, *sqlite.Store, *task.Manager, func() time.Time) {
	t.Helper()

	store, err := sqlite.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("sqlite.Open: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	wsStore := newMockWorkspaceStore()
	agentExec := &mockAgentExecutor{}
	taskMgr := task.NewManager(task.Config{
		WorkspaceStore:   wsStore,
		TerminalNotifier: nil,
	}, nil, agentExec)

	now := time.Date(2026, 4, 29, 12, 0, 0, 0, time.UTC)
	svc := NewService(store, taskMgr)
	svc.SetClock(func() time.Time { return now })
	return svc, store, taskMgr, func() time.Time { return now }
}

type sqliteTxAdapter struct {
	*sqlite.Store
	withTxCalls int
}

func (a *sqliteTxAdapter) WithTx(ctx context.Context, fn func(RuntimeTx) error) error {
	a.withTxCalls++
	return a.Store.WithTx(ctx, func(tx sqlite.RuntimeTx) error {
		return fn(tx)
	})
}

func mustGetScheduledJob(t *testing.T, ctx context.Context, store *sqlite.Store, jobID string) types.ScheduledJob {
	t.Helper()

	job, ok, err := store.GetScheduledJob(ctx, jobID)
	if err != nil {
		t.Fatalf("GetScheduledJob(%q): %v", jobID, err)
	}
	if !ok {
		t.Fatalf("GetScheduledJob(%q): not found", jobID)
	}
	return job
}

func mustGetChildAgentSpec(t *testing.T, ctx context.Context, store *sqlite.Store, agentID string) types.ChildAgentSpec {
	t.Helper()

	spec, ok, err := store.GetChildAgentSpec(ctx, agentID)
	if err != nil {
		t.Fatalf("GetChildAgentSpec(%q): %v", agentID, err)
	}
	if !ok {
		t.Fatalf("GetChildAgentSpec(%q): not found", agentID)
	}
	return spec
}

func mustGetReportGroup(t *testing.T, ctx context.Context, store *sqlite.Store, groupID string) types.ReportGroup {
	t.Helper()

	group, ok, err := store.GetReportGroup(ctx, groupID)
	if err != nil {
		t.Fatalf("GetReportGroup(%q): %v", groupID, err)
	}
	if !ok {
		t.Fatalf("GetReportGroup(%q): not found", groupID)
	}
	return group
}

func TestIntegrationCreateDispatchRecordAndQuery(t *testing.T) {
	ctx := context.Background()
	svc, store, taskMgr, _ := setupSchedulerStoreTest(t)

	job, err := svc.CreateJob(ctx, CreateJobInput{
		Name:           "test-job",
		WorkspaceRoot:  filepath.ToSlash(t.TempDir()),
		OwnerSessionID: "session-1",
		Prompt:         "run report",
		EveryMinutes:   15,
	})
	if err != nil {
		t.Fatalf("CreateJob: %v", err)
	}

	stored := mustGetScheduledJob(t, ctx, store, job.ID)
	if stored.Kind != types.ScheduleKindEvery {
		t.Fatalf("Kind = %q, want %q", stored.Kind, types.ScheduleKindEvery)
	}
	if stored.EveryMinutes != 15 {
		t.Fatalf("EveryMinutes = %d, want 15", stored.EveryMinutes)
	}

	spec := mustGetChildAgentSpec(t, ctx, store, job.ID)
	if spec.AgentID != job.ID {
		t.Fatalf("AgentID = %q, want %q", spec.AgentID, job.ID)
	}

	laterNow := job.NextRunAt.Add(time.Minute)
	svc.SetClock(func() time.Time { return laterNow })
	err = svc.handleDueJob(ctx, job, laterNow)
	if err != nil {
		t.Fatalf("handleDueJob: %v", err)
	}

	runningJob := mustGetScheduledJob(t, ctx, store, job.ID)
	if runningJob.LastStatus != types.ScheduledJobStatusRunning {
		t.Fatalf("LastStatus = %q, want %q", runningJob.LastStatus, types.ScheduledJobStatusRunning)
	}
	if runningJob.TotalRuns != 1 {
		t.Fatalf("TotalRuns = %d, want 1", runningJob.TotalRuns)
	}
	if runningJob.LastTaskID == "" {
		t.Fatal("LastTaskID = empty, want task ID")
	}

	taskID := runningJob.LastTaskID
	taskRow, ok, err := taskMgr.Get(taskID, job.WorkspaceRoot)
	if err != nil {
		t.Fatalf("Get(task): %v", err)
	}
	if !ok {
		t.Fatal("task not found")
	}
	if taskRow.Command != job.Prompt {
		t.Fatalf("task.Command = %q, want %q", taskRow.Command, job.Prompt)
	}
	if taskRow.WorkspaceRoot != job.WorkspaceRoot {
		t.Fatalf("task.WorkspaceRoot = %q, want %q", taskRow.WorkspaceRoot, job.WorkspaceRoot)
	}

	endTime := laterNow.Add(30 * time.Second)
	err = svc.RecordTaskTerminal(ctx, task.Task{
		ID:             taskID,
		ScheduledJobID: job.ID,
		Status:         task.TaskStatusCompleted,
		WorkspaceRoot:  job.WorkspaceRoot,
		EndTime:        &endTime,
	})
	if err != nil {
		t.Fatalf("RecordTaskTerminal: %v", err)
	}

	completedJob := mustGetScheduledJob(t, ctx, store, job.ID)
	if completedJob.LastStatus != types.ScheduledJobStatusSucceeded {
		t.Fatalf("LastStatus = %q, want %q", completedJob.LastStatus, types.ScheduledJobStatusSucceeded)
	}
	if completedJob.SuccessCount != 1 {
		t.Fatalf("SuccessCount = %d, want 1", completedJob.SuccessCount)
	}
	if completedJob.LastTaskID != taskID {
		t.Fatalf("LastTaskID = %q, want %q", completedJob.LastTaskID, taskID)
	}
	if !completedJob.LastRunAt.Equal(endTime) {
		t.Fatalf("LastRunAt = %v, want %v", completedJob.LastRunAt, endTime)
	}
}

func TestIntegrationCreateJobNonTransactionalPath(t *testing.T) {
	ctx := context.Background()
	svc, store, _, _ := setupSchedulerStoreTest(t)

	if _, ok := any(store).(transactionalStore); ok {
		t.Fatal("*sqlite.Store unexpectedly implements transactionalStore")
	}

	job, err := svc.CreateJob(ctx, CreateJobInput{
		Name:           "non-tx-job",
		WorkspaceRoot:  filepath.ToSlash(t.TempDir()),
		OwnerSessionID: "session-1",
		Prompt:         "run report",
		EveryMinutes:   15,
	})
	if err != nil {
		t.Fatalf("CreateJob: %v", err)
	}

	stored := mustGetScheduledJob(t, ctx, store, job.ID)
	if stored.ID != job.ID {
		t.Fatalf("stored ID = %q, want %q", stored.ID, job.ID)
	}

	spec := mustGetChildAgentSpec(t, ctx, store, job.ID)
	if spec.AgentID != job.ID {
		t.Fatalf("AgentID = %q, want %q", spec.AgentID, job.ID)
	}
}

func TestIntegrationCreateJobTransactionalPath(t *testing.T) {
	ctx := context.Background()
	_, store, taskMgr, now := setupSchedulerStoreTest(t)

	adapter := &sqliteTxAdapter{Store: store}
	if _, ok := any(adapter).(transactionalStore); !ok {
		t.Fatal("sqliteTxAdapter does not implement transactionalStore")
	}

	svc := NewService(adapter, taskMgr)
	svc.SetClock(now)

	job, err := svc.CreateJob(ctx, CreateJobInput{
		Name:                    "tx-job",
		WorkspaceRoot:           filepath.ToSlash(t.TempDir()),
		OwnerSessionID:          "session-1",
		Prompt:                  "run report",
		EveryMinutes:            15,
		ReportGroupID:           "group-1",
		ReportGroupTitle:        "Reports",
		ReportGroupEveryMinutes: 60,
	})
	if err != nil {
		t.Fatalf("CreateJob: %v", err)
	}

	if adapter.withTxCalls != 1 {
		t.Fatalf("withTxCalls = %d, want 1", adapter.withTxCalls)
	}

	stored := mustGetScheduledJob(t, ctx, store, job.ID)
	if stored.ID != job.ID {
		t.Fatalf("stored ID = %q, want %q", stored.ID, job.ID)
	}

	spec := mustGetChildAgentSpec(t, ctx, store, job.ID)
	if spec.AgentID != job.ID {
		t.Fatalf("AgentID = %q, want %q", spec.AgentID, job.ID)
	}

	group := mustGetReportGroup(t, ctx, store, "group-1")
	if group.SessionID != "session-1" {
		t.Fatalf("SessionID = %q, want %q", group.SessionID, "session-1")
	}
	if len(group.Sources) != 1 || group.Sources[0] != job.ID {
		t.Fatalf("Sources = %v, want [%s]", group.Sources, job.ID)
	}
	if group.Schedule.Kind != types.ScheduleKindEvery {
		t.Fatalf("Schedule.Kind = %q, want %q", group.Schedule.Kind, types.ScheduleKindEvery)
	}
	if group.Schedule.EveryMinutes != 60 {
		t.Fatalf("Schedule.EveryMinutes = %d, want 60", group.Schedule.EveryMinutes)
	}
}

func TestIntegrationHandleDueJobSkipIfRunningTrue(t *testing.T) {
	ctx := context.Background()
	svc, store, taskMgr, _ := setupSchedulerStoreTest(t)

	skipIfRunning := true
	job, err := svc.CreateJob(ctx, CreateJobInput{
		Name:           "skip-job",
		WorkspaceRoot:  filepath.ToSlash(t.TempDir()),
		OwnerSessionID: "session-1",
		Prompt:         "run report",
		EveryMinutes:   15,
		SkipIfRunning:  &skipIfRunning,
	})
	if err != nil {
		t.Fatalf("CreateJob: %v", err)
	}

	activeTask, err := taskMgr.Create(ctx, task.CreateTaskInput{
		Type:          task.TaskTypeAgent,
		Command:       "run report",
		WorkspaceRoot: job.WorkspaceRoot,
	})
	if err != nil {
		t.Fatalf("Create task: %v", err)
	}

	job.LastTaskID = activeTask.ID
	if err := store.UpsertScheduledJob(ctx, job); err != nil {
		t.Fatalf("UpsertScheduledJob: %v", err)
	}

	laterNow := job.NextRunAt.Add(time.Minute)
	svc.SetClock(func() time.Time { return laterNow })
	err = svc.handleDueJob(ctx, job, laterNow)
	if err != nil {
		t.Fatalf("handleDueJob: %v", err)
	}

	updated := mustGetScheduledJob(t, ctx, store, job.ID)
	if updated.LastStatus != types.ScheduledJobStatusSkipped {
		t.Fatalf("LastStatus = %q, want %q", updated.LastStatus, types.ScheduledJobStatusSkipped)
	}
	if updated.SkipCount != 1 {
		t.Fatalf("SkipCount = %d, want 1", updated.SkipCount)
	}
	if updated.LastError != "previous run still active" {
		t.Fatalf("LastError = %q, want %q", updated.LastError, "previous run still active")
	}
}

func TestIntegrationHandleDueJobSkipIfRunningFalse(t *testing.T) {
	ctx := context.Background()
	svc, store, taskMgr, _ := setupSchedulerStoreTest(t)

	skipIfRunning := false
	job, err := svc.CreateJob(ctx, CreateJobInput{
		Name:           "no-skip-job",
		WorkspaceRoot:  filepath.ToSlash(t.TempDir()),
		OwnerSessionID: "session-1",
		Prompt:         "run report",
		EveryMinutes:   15,
		SkipIfRunning:  &skipIfRunning,
	})
	if err != nil {
		t.Fatalf("CreateJob: %v", err)
	}

	activeTask, err := taskMgr.Create(ctx, task.CreateTaskInput{
		Type:          task.TaskTypeAgent,
		Command:       "run report",
		WorkspaceRoot: job.WorkspaceRoot,
	})
	if err != nil {
		t.Fatalf("Create task: %v", err)
	}

	job.LastTaskID = activeTask.ID
	if err := store.UpsertScheduledJob(ctx, job); err != nil {
		t.Fatalf("UpsertScheduledJob: %v", err)
	}

	laterNow := job.NextRunAt.Add(time.Minute)
	svc.SetClock(func() time.Time { return laterNow })
	err = svc.handleDueJob(ctx, job, laterNow)
	if err != nil {
		t.Fatalf("handleDueJob: %v", err)
	}

	updated := mustGetScheduledJob(t, ctx, store, job.ID)
	if updated.LastStatus != types.ScheduledJobStatusPending {
		t.Fatalf("LastStatus = %q, want %q", updated.LastStatus, types.ScheduledJobStatusPending)
	}
	if updated.TotalRuns != 0 {
		t.Fatalf("TotalRuns = %d, want 0", updated.TotalRuns)
	}
	if updated.LastTaskID != activeTask.ID {
		t.Fatalf("LastTaskID = %q, want %q", updated.LastTaskID, activeTask.ID)
	}
}

func TestIntegrationDeleteJobCascade(t *testing.T) {
	ctx := context.Background()
	svc, store, _, _ := setupSchedulerStoreTest(t)

	job, err := svc.CreateJob(ctx, CreateJobInput{
		Name:                    "delete-job",
		WorkspaceRoot:           filepath.ToSlash(t.TempDir()),
		OwnerSessionID:          "session-1",
		Prompt:                  "run report",
		EveryMinutes:            15,
		ReportGroupID:           "group-1",
		ReportGroupTitle:        "Reports",
		ReportGroupEveryMinutes: 60,
	})
	if err != nil {
		t.Fatalf("CreateJob: %v", err)
	}

	group := mustGetReportGroup(t, ctx, store, "group-1")
	if len(group.Sources) != 1 || group.Sources[0] != job.ID {
		t.Fatalf("Sources = %v, want [%s]", group.Sources, job.ID)
	}

	deleted, err := svc.DeleteJob(ctx, job.ID)
	if err != nil {
		t.Fatalf("DeleteJob: %v", err)
	}
	if !deleted {
		t.Fatal("deleted = false, want true")
	}

	if _, ok, err := store.GetScheduledJob(ctx, job.ID); err != nil {
		t.Fatalf("GetScheduledJob after delete: %v", err)
	} else if ok {
		t.Fatalf("GetScheduledJob(%q): still present", job.ID)
	}

	if _, ok, err := store.GetChildAgentSpec(ctx, job.ID); err != nil {
		t.Fatalf("GetChildAgentSpec after delete: %v", err)
	} else if ok {
		t.Fatalf("GetChildAgentSpec(%q): still present", job.ID)
	}

	group = mustGetReportGroup(t, ctx, store, "group-1")
	if len(group.Sources) != 0 {
		t.Fatalf("Sources = %v, want empty", group.Sources)
	}
}

func TestIntegrationSetJobEnabledCycle(t *testing.T) {
	ctx := context.Background()
	svc, store, _, now := setupSchedulerStoreTest(t)

	job, err := svc.CreateJob(ctx, CreateJobInput{
		Name:           "toggle-job",
		WorkspaceRoot:  filepath.ToSlash(t.TempDir()),
		OwnerSessionID: "session-1",
		Prompt:         "run report",
		EveryMinutes:   15,
	})
	if err != nil {
		t.Fatalf("CreateJob: %v", err)
	}

	disabled, ok, err := svc.SetJobEnabled(ctx, job.ID, false)
	if err != nil {
		t.Fatalf("SetJobEnabled(false): %v", err)
	}
	if !ok {
		t.Fatal("SetJobEnabled(false): ok = false, want true")
	}
	if disabled.Enabled {
		t.Fatal("Enabled = true, want false")
	}

	storedDisabled := mustGetScheduledJob(t, ctx, store, job.ID)
	if storedDisabled.Enabled {
		t.Fatal("stored Enabled = true, want false")
	}

	enabled, ok, err := svc.SetJobEnabled(ctx, job.ID, true)
	if err != nil {
		t.Fatalf("SetJobEnabled(true): %v", err)
	}
	if !ok {
		t.Fatal("SetJobEnabled(true): ok = false, want true")
	}
	if !enabled.Enabled {
		t.Fatal("Enabled = false, want true")
	}
	wantNextRun := now().Add(15 * time.Minute)
	if !enabled.NextRunAt.Equal(wantNextRun) {
		t.Fatalf("NextRunAt = %v, want %v", enabled.NextRunAt, wantNextRun)
	}

	storedEnabled := mustGetScheduledJob(t, ctx, store, job.ID)
	if !storedEnabled.Enabled {
		t.Fatal("stored Enabled = false, want true")
	}
	if !storedEnabled.NextRunAt.Equal(wantNextRun) {
		t.Fatalf("stored NextRunAt = %v, want %v", storedEnabled.NextRunAt, wantNextRun)
	}
}

func TestIntegrationServiceListJobs(t *testing.T) {
	ctx := context.Background()
	svc, _, _, _ := setupSchedulerStoreTest(t)
	workspaceRoot := filepath.ToSlash(t.TempDir())

	_, err := svc.CreateJob(ctx, CreateJobInput{
		Name: "job-a", WorkspaceRoot: workspaceRoot, OwnerSessionID: "s1", Prompt: "p", EveryMinutes: 10,
	})
	if err != nil {
		t.Fatalf("CreateJob: %v", err)
	}
	_, err = svc.CreateJob(ctx, CreateJobInput{
		Name: "job-b", WorkspaceRoot: workspaceRoot, OwnerSessionID: "s1", Prompt: "p", EveryMinutes: 20,
	})
	if err != nil {
		t.Fatalf("CreateJob: %v", err)
	}

	jobs, err := svc.ListJobs(ctx, workspaceRoot)
	if err != nil {
		t.Fatalf("ListJobs: %v", err)
	}
	if len(jobs) < 2 {
		t.Fatalf("len(jobs) = %d, want >= 2", len(jobs))
	}
}

func TestIntegrationServiceGetJob(t *testing.T) {
	ctx := context.Background()
	svc, _, _, _ := setupSchedulerStoreTest(t)

	job, err := svc.CreateJob(ctx, CreateJobInput{
		Name: "get-me", WorkspaceRoot: filepath.ToSlash(t.TempDir()), OwnerSessionID: "s1", Prompt: "p", EveryMinutes: 30,
	})
	if err != nil {
		t.Fatalf("CreateJob: %v", err)
	}

	got, ok, err := svc.GetJob(ctx, job.ID)
	if err != nil {
		t.Fatalf("GetJob: %v", err)
	}
	if !ok {
		t.Fatal("GetJob returned ok=false")
	}
	if got.Name != "get-me" {
		t.Fatalf("Name = %q, want %q", got.Name, "get-me")
	}

	_, ok, err = svc.GetJob(ctx, "no-such-id")
	if err != nil {
		t.Fatalf("GetJob: %v", err)
	}
	if ok {
		t.Fatal("GetJob should return ok=false for missing job")
	}
}

func TestIntegrationListDueScheduledJobs(t *testing.T) {
	ctx := context.Background()
	_, store, _, now := setupSchedulerStoreTest(t)

	workspaceRoot := filepath.ToSlash(t.TempDir())
	baseTime := now()
	queryNow := baseTime.Add(30 * time.Minute)

	jobs := []types.ScheduledJob{
		{
			ID:             "job-due-early",
			Name:           "Due early",
			WorkspaceRoot:  workspaceRoot,
			OwnerSessionID: "session-1",
			Prompt:         "run early",
			Kind:           types.ScheduleKindEvery,
			EveryMinutes:   15,
			Enabled:        true,
			NextRunAt:      queryNow.Add(-time.Minute),
			CreatedAt:      baseTime,
			UpdatedAt:      baseTime,
		},
		{
			ID:             "job-due-now",
			Name:           "Due now",
			WorkspaceRoot:  workspaceRoot,
			OwnerSessionID: "session-1",
			Prompt:         "run now",
			Kind:           types.ScheduleKindEvery,
			EveryMinutes:   15,
			Enabled:        true,
			NextRunAt:      queryNow,
			CreatedAt:      baseTime,
			UpdatedAt:      baseTime,
		},
		{
			ID:             "job-future",
			Name:           "Future",
			WorkspaceRoot:  workspaceRoot,
			OwnerSessionID: "session-1",
			Prompt:         "run later",
			Kind:           types.ScheduleKindEvery,
			EveryMinutes:   15,
			Enabled:        true,
			NextRunAt:      queryNow.Add(time.Minute),
			CreatedAt:      baseTime,
			UpdatedAt:      baseTime,
		},
		{
			ID:             "job-disabled",
			Name:           "Disabled",
			WorkspaceRoot:  workspaceRoot,
			OwnerSessionID: "session-1",
			Prompt:         "disabled",
			Kind:           types.ScheduleKindEvery,
			EveryMinutes:   15,
			Enabled:        false,
			NextRunAt:      queryNow.Add(-2 * time.Minute),
			CreatedAt:      baseTime,
			UpdatedAt:      baseTime,
		},
	}

	for _, job := range jobs {
		if err := store.UpsertScheduledJob(ctx, job); err != nil {
			t.Fatalf("UpsertScheduledJob(%q): %v", job.ID, err)
		}
	}

	dueJobs, err := store.ListDueScheduledJobs(ctx, queryNow)
	if err != nil {
		t.Fatalf("ListDueScheduledJobs: %v", err)
	}
	if len(dueJobs) != 2 {
		t.Fatalf("len(dueJobs) = %d, want 2", len(dueJobs))
	}
	if dueJobs[0].ID != "job-due-early" {
		t.Fatalf("dueJobs[0].ID = %q, want %q", dueJobs[0].ID, "job-due-early")
	}
	if dueJobs[1].ID != "job-due-now" {
		t.Fatalf("dueJobs[1].ID = %q, want %q", dueJobs[1].ID, "job-due-now")
	}
}
