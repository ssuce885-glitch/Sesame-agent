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

type blockingAgentExecutor struct {
	started                chan struct{}
	release                chan struct{}
	gotActivatedSkillNames []string
}

func (b *blockingAgentExecutor) RunTask(ctx context.Context, workspaceRoot string, prompt string, activatedSkillNames []string, observer task.AgentTaskObserver) error {
	_ = workspaceRoot
	_ = prompt
	b.gotActivatedSkillNames = append([]string(nil), activatedSkillNames...)
	if b.started != nil {
		select {
		case b.started <- struct{}{}:
		default:
		}
	}
	select {
	case <-b.release:
		if observer != nil {
			return observer.SetFinalText("done")
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func TestServiceTickDispatchesOneShotJobAndDisablesIt(t *testing.T) {
	store, err := sqlite.Open(filepath.Join(t.TempDir(), "agentd.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	workspaceRoot := t.TempDir()
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	executor := &blockingAgentExecutor{
		started: started,
		release: release,
	}
	manager := task.NewManager(task.Config{MaxConcurrentTasks: 8, TaskOutputMaxBytes: 1 << 20}, nil, executor)

	service := NewService(store, manager)
	now := time.Date(2026, 4, 10, 10, 0, 0, 0, time.UTC)
	service.SetClock(func() time.Time { return now })

	job, err := service.CreateJob(context.Background(), CreateJobInput{
		Name:                "Shanghai weather",
		WorkspaceRoot:       workspaceRoot,
		OwnerSessionID:      "sess_one_shot",
		Prompt:              "五分钟后汇报上海天气",
		ActivatedSkillNames: []string{"send-email", " send-email "},
		ReportGroupID:       "weather-daily",
		ReportGroupTitle:    "Weather Daily Digest",
		ReportGroupCron:     "0 18 * * *",
		ReportGroupTimezone: "Asia/Shanghai",
		DelayMinutes:        5,
	})
	if err != nil {
		t.Fatalf("CreateJob() error = %v", err)
	}
	if got, want := job.NextRunAt, now.Add(5*time.Minute); !got.Equal(want) {
		t.Fatalf("NextRunAt = %v, want %v", got, want)
	}
	spec, ok, err := store.GetChildAgentSpec(context.Background(), job.ID)
	if err != nil {
		t.Fatalf("GetChildAgentSpec() error = %v", err)
	}
	if !ok {
		t.Fatal("child agent spec not created for scheduled job")
	}
	if spec.AgentID != job.ID {
		t.Fatalf("AgentID = %q, want %q", spec.AgentID, job.ID)
	}
	if spec.Mode != types.ChildAgentModeBackgroundWorker {
		t.Fatalf("Mode = %q, want %q", spec.Mode, types.ChildAgentModeBackgroundWorker)
	}
	if spec.Schedule.Kind != types.ScheduleKindAt {
		t.Fatalf("Schedule.Kind = %q, want %q", spec.Schedule.Kind, types.ScheduleKindAt)
	}
	if len(spec.ReportGroups) != 1 || spec.ReportGroups[0] != "weather-daily" {
		t.Fatalf("ReportGroups = %#v, want weather-daily", spec.ReportGroups)
	}
	if got := spec.ActivatedSkillNames; len(got) != 1 || got[0] != "send-email" {
		t.Fatalf("spec activated skills = %v, want [send-email]", got)
	}
	group, ok, err := store.GetReportGroup(context.Background(), "weather-daily")
	if err != nil {
		t.Fatalf("GetReportGroup() error = %v", err)
	}
	if !ok {
		t.Fatal("report group not created for scheduled job")
	}
	if group.SessionID != "sess_one_shot" {
		t.Fatalf("group session = %q, want sess_one_shot", group.SessionID)
	}
	if len(group.Sources) != 1 || group.Sources[0] != job.ID {
		t.Fatalf("group sources = %#v, want %q", group.Sources, job.ID)
	}
	if group.Schedule.Kind != types.ScheduleKindCron || group.Schedule.Expr != "0 18 * * *" || group.Schedule.Timezone != "Asia/Shanghai" {
		t.Fatalf("group schedule = %#v, want cron 0 18 * * * Asia/Shanghai", group.Schedule)
	}

	now = now.Add(5 * time.Minute)
	if err := service.Tick(context.Background()); err != nil {
		t.Fatalf("Tick() error = %v", err)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("scheduled task did not start")
	}

	persisted, ok, err := store.GetScheduledJob(context.Background(), job.ID)
	if err != nil {
		t.Fatalf("GetScheduledJob() error = %v", err)
	}
	if !ok {
		t.Fatal("scheduled job disappeared after dispatch")
	}
	if persisted.Enabled {
		t.Fatal("one-shot job remained enabled after dispatch")
	}
	if !persisted.NextRunAt.IsZero() {
		t.Fatalf("NextRunAt = %v, want zero for one-shot job after dispatch", persisted.NextRunAt)
	}
	if persisted.LastStatus != types.ScheduledJobStatusRunning {
		t.Fatalf("LastStatus = %q, want %q", persisted.LastStatus, types.ScheduledJobStatusRunning)
	}
	if persisted.TotalRuns != 1 {
		t.Fatalf("TotalRuns = %d, want 1", persisted.TotalRuns)
	}
	if got := persisted.ActivatedSkillNames; len(got) != 1 || got[0] != "send-email" {
		t.Fatalf("persisted activated skills = %v, want [send-email]", got)
	}
	createdTask, ok, err := manager.Get(persisted.LastTaskID, workspaceRoot)
	if err != nil {
		t.Fatalf("task manager Get() error = %v", err)
	}
	if !ok {
		t.Fatal("scheduled task record not found")
	}
	if createdTask.ScheduledJobID != job.ID {
		t.Fatalf("ScheduledJobID = %q, want %q", createdTask.ScheduledJobID, job.ID)
	}
	if createdTask.ParentSessionID != "sess_one_shot" {
		t.Fatalf("ParentSessionID = %q, want sess_one_shot", createdTask.ParentSessionID)
	}
	if got := createdTask.ActivatedSkillNames; len(got) != 1 || got[0] != "send-email" {
		t.Fatalf("task activated skills = %v, want [send-email]", got)
	}

	close(release)
	waitCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, _, err := manager.Wait(waitCtx, createdTask.ID, workspaceRoot); err != nil {
		t.Fatalf("Wait() error = %v", err)
	}
	if got := executor.gotActivatedSkillNames; len(got) != 1 || got[0] != "send-email" {
		t.Fatalf("executor activated skills = %v, want [send-email]", got)
	}

	deleted, err := service.DeleteJob(context.Background(), job.ID)
	if err != nil {
		t.Fatalf("DeleteJob() error = %v", err)
	}
	if !deleted {
		t.Fatal("DeleteJob() = false, want true")
	}
	if _, ok, err := store.GetChildAgentSpec(context.Background(), job.ID); err != nil {
		t.Fatalf("GetChildAgentSpec(after delete) error = %v", err)
	} else if ok {
		t.Fatal("child agent spec still exists after deleting scheduled job")
	}
	group, ok, err = store.GetReportGroup(context.Background(), "weather-daily")
	if err != nil {
		t.Fatalf("GetReportGroup(after delete) error = %v", err)
	}
	if !ok {
		t.Fatal("report group missing after deleting scheduled job")
	}
	if len(group.Sources) != 0 {
		t.Fatalf("group sources after delete = %#v, want empty", group.Sources)
	}
}

func TestServiceTickSkipsRecurringJobWhilePreviousRunIsActive(t *testing.T) {
	store, err := sqlite.Open(filepath.Join(t.TempDir(), "agentd.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	workspaceRoot := t.TempDir()
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	manager := task.NewManager(task.Config{MaxConcurrentTasks: 8, TaskOutputMaxBytes: 1 << 20}, nil, &blockingAgentExecutor{
		started: started,
		release: release,
	})

	service := NewService(store, manager)
	now := time.Date(2026, 4, 10, 10, 0, 0, 0, time.UTC)
	service.SetClock(func() time.Time { return now })

	job, err := service.CreateJob(context.Background(), CreateJobInput{
		Name:           "Recurring weather",
		WorkspaceRoot:  workspaceRoot,
		OwnerSessionID: "sess_recurring",
		Prompt:         "每五分钟汇报上海天气",
		EveryMinutes:   5,
	})
	if err != nil {
		t.Fatalf("CreateJob() error = %v", err)
	}
	if got, want := job.NextRunAt, now.Add(5*time.Minute); !got.Equal(want) {
		t.Fatalf("NextRunAt = %v, want %v", got, want)
	}

	now = now.Add(5 * time.Minute)
	if err := service.Tick(context.Background()); err != nil {
		t.Fatalf("first Tick() error = %v", err)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("first scheduled task did not start")
	}

	firstDispatch, ok, err := store.GetScheduledJob(context.Background(), job.ID)
	if err != nil {
		t.Fatalf("GetScheduledJob(first) error = %v", err)
	}
	if !ok {
		t.Fatal("scheduled job missing after first dispatch")
	}
	if firstDispatch.LastStatus != types.ScheduledJobStatusRunning {
		t.Fatalf("LastStatus after first dispatch = %q, want running", firstDispatch.LastStatus)
	}
	if got, want := firstDispatch.NextRunAt, now.Add(5*time.Minute); !got.Equal(want) {
		t.Fatalf("NextRunAt after first dispatch = %v, want %v", got, want)
	}

	now = now.Add(5 * time.Minute)
	if err := service.Tick(context.Background()); err != nil {
		t.Fatalf("second Tick() error = %v", err)
	}

	skipped, ok, err := store.GetScheduledJob(context.Background(), job.ID)
	if err != nil {
		t.Fatalf("GetScheduledJob(second) error = %v", err)
	}
	if !ok {
		t.Fatal("scheduled job missing after skip")
	}
	if skipped.LastStatus != types.ScheduledJobStatusSkipped {
		t.Fatalf("LastStatus after skip = %q, want skipped", skipped.LastStatus)
	}
	if skipped.SkipCount != 1 {
		t.Fatalf("SkipCount = %d, want 1", skipped.SkipCount)
	}
	if skipped.LastError != "previous run still active" {
		t.Fatalf("LastError = %q, want previous run still active", skipped.LastError)
	}
	if got, want := skipped.NextRunAt, now.Add(5*time.Minute); !got.Equal(want) {
		t.Fatalf("NextRunAt after skip = %v, want %v", got, want)
	}

	close(release)
	waitCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, _, err := manager.Wait(waitCtx, firstDispatch.LastTaskID, workspaceRoot); err != nil {
		t.Fatalf("Wait() error = %v", err)
	}
}

func TestCreateJobRejectsCrossSessionReportGroupReuse(t *testing.T) {
	store, err := sqlite.Open(filepath.Join(t.TempDir(), "agentd.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	service := NewService(store, nil)
	if _, err := service.CreateJob(context.Background(), CreateJobInput{
		Name:             "worker a",
		WorkspaceRoot:    t.TempDir(),
		OwnerSessionID:   "sess_a",
		Prompt:           "汇报 A",
		ReportGroupID:    "shared-group",
		ReportGroupTitle: "Shared Group",
		DelayMinutes:     5,
	}); err != nil {
		t.Fatalf("CreateJob(first) error = %v", err)
	}

	if _, err := service.CreateJob(context.Background(), CreateJobInput{
		Name:           "worker b",
		WorkspaceRoot:  t.TempDir(),
		OwnerSessionID: "sess_b",
		Prompt:         "汇报 B",
		ReportGroupID:  "shared-group",
		DelayMinutes:   5,
	}); err == nil {
		t.Fatal("CreateJob(second) error = nil, want cross-session report group conflict")
	}

	jobs, err := store.ListScheduledJobs(context.Background())
	if err != nil {
		t.Fatalf("ListScheduledJobs() error = %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("len(jobs) = %d, want 1 after rejected create", len(jobs))
	}
	specs, err := store.ListChildAgentSpecs(context.Background())
	if err != nil {
		t.Fatalf("ListChildAgentSpecs() error = %v", err)
	}
	if len(specs) != 1 {
		t.Fatalf("len(specs) = %d, want 1 after rejected create", len(specs))
	}
	group, ok, err := store.GetReportGroup(context.Background(), "shared-group")
	if err != nil {
		t.Fatalf("GetReportGroup() error = %v", err)
	}
	if !ok {
		t.Fatal("report group missing after rejected create")
	}
	if len(group.Sources) != 1 {
		t.Fatalf("len(group.Sources) = %d, want 1 after rejected create", len(group.Sources))
	}
}

func TestCreateJobRejectsInvalidReportGroupScheduleWithoutPersistingJob(t *testing.T) {
	store, err := sqlite.Open(filepath.Join(t.TempDir(), "agentd.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	service := NewService(store, nil)
	_, err = service.CreateJob(context.Background(), CreateJobInput{
		Name:                "worker invalid group",
		WorkspaceRoot:       t.TempDir(),
		OwnerSessionID:      "sess_invalid_group",
		Prompt:              "汇报 C",
		ReportGroupID:       "invalid-group",
		ReportGroupCron:     "not a cron",
		ReportGroupTimezone: "Mars/Base",
		DelayMinutes:        5,
	})
	if err == nil {
		t.Fatal("CreateJob() error = nil, want invalid report group schedule error")
	}

	jobs, err := store.ListScheduledJobs(context.Background())
	if err != nil {
		t.Fatalf("ListScheduledJobs() error = %v", err)
	}
	if len(jobs) != 0 {
		t.Fatalf("len(jobs) = %d, want 0 after rejected create", len(jobs))
	}
	specs, err := store.ListChildAgentSpecs(context.Background())
	if err != nil {
		t.Fatalf("ListChildAgentSpecs() error = %v", err)
	}
	if len(specs) != 0 {
		t.Fatalf("len(specs) = %d, want 0 after rejected create", len(specs))
	}
	if _, ok, err := store.GetReportGroup(context.Background(), "invalid-group"); err != nil {
		t.Fatalf("GetReportGroup() error = %v", err)
	} else if ok {
		t.Fatal("report group persisted despite rejected create")
	}
}
