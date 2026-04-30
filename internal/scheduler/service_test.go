package scheduler

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"go-agent/internal/task"
	"go-agent/internal/types"
)

type mockStore struct {
	jobs         map[string]types.ScheduledJob
	childSpecs   map[string]types.ChildAgentSpec
	reportGroups map[string]types.ReportGroup
}

func newMockStore() *mockStore {
	return &mockStore{
		jobs:         make(map[string]types.ScheduledJob),
		childSpecs:   make(map[string]types.ChildAgentSpec),
		reportGroups: make(map[string]types.ReportGroup),
	}
}

func (m *mockStore) UpsertScheduledJob(_ context.Context, job types.ScheduledJob) error {
	m.jobs[job.ID] = job
	return nil
}

func (m *mockStore) GetScheduledJob(_ context.Context, id string) (types.ScheduledJob, bool, error) {
	job, ok := m.jobs[id]
	return job, ok, nil
}

func (m *mockStore) ListScheduledJobs(_ context.Context) ([]types.ScheduledJob, error) {
	out := make([]types.ScheduledJob, 0, len(m.jobs))
	for _, job := range m.jobs {
		out = append(out, job)
	}
	slices.SortFunc(out, func(a, b types.ScheduledJob) int {
		return strings.Compare(a.ID, b.ID)
	})
	return out, nil
}

func (m *mockStore) ListScheduledJobsByWorkspace(_ context.Context, workspaceRoot string) ([]types.ScheduledJob, error) {
	out := make([]types.ScheduledJob, 0)
	for _, job := range m.jobs {
		if job.WorkspaceRoot == workspaceRoot {
			out = append(out, job)
		}
	}
	slices.SortFunc(out, func(a, b types.ScheduledJob) int {
		return strings.Compare(a.ID, b.ID)
	})
	return out, nil
}

func (m *mockStore) ListDueScheduledJobs(_ context.Context, now time.Time) ([]types.ScheduledJob, error) {
	out := make([]types.ScheduledJob, 0)
	for _, job := range m.jobs {
		if job.Enabled && !job.NextRunAt.IsZero() && !job.NextRunAt.After(now) {
			out = append(out, job)
		}
	}
	slices.SortFunc(out, func(a, b types.ScheduledJob) int {
		if a.NextRunAt.Before(b.NextRunAt) {
			return -1
		}
		if a.NextRunAt.After(b.NextRunAt) {
			return 1
		}
		return strings.Compare(a.ID, b.ID)
	})
	return out, nil
}

func (m *mockStore) DeleteScheduledJob(_ context.Context, id string) (bool, error) {
	if _, ok := m.jobs[id]; !ok {
		return false, nil
	}
	delete(m.jobs, id)
	return true, nil
}

func (m *mockStore) UpsertChildAgentSpec(_ context.Context, spec types.ChildAgentSpec) error {
	m.childSpecs[spec.AgentID] = spec
	return nil
}

func (m *mockStore) DeleteChildAgentSpec(_ context.Context, id string) (bool, error) {
	if _, ok := m.childSpecs[id]; !ok {
		return false, nil
	}
	delete(m.childSpecs, id)
	return true, nil
}

func (m *mockStore) GetReportGroup(_ context.Context, id string) (types.ReportGroup, bool, error) {
	group, ok := m.reportGroups[id]
	return group, ok, nil
}

func (m *mockStore) UpsertReportGroup(_ context.Context, group types.ReportGroup) error {
	m.reportGroups[group.GroupID] = group
	return nil
}

type mockTxStore struct {
	*mockStore
}

func (m *mockTxStore) WithTx(ctx context.Context, fn func(RuntimeTx) error) error {
	return fn(m.mockStore)
}

type errorGetJobStore struct {
	*mockStore
	err error
}

func (m *errorGetJobStore) GetScheduledJob(_ context.Context, _ string) (types.ScheduledJob, bool, error) {
	return types.ScheduledJob{}, false, m.err
}

type mockWorkspaceStore struct {
	tasks map[string][]task.Task
	todos map[string][]task.TodoItem
}

func newMockWorkspaceStore() *mockWorkspaceStore {
	return &mockWorkspaceStore{
		tasks: make(map[string][]task.Task),
		todos: make(map[string][]task.TodoItem),
	}
}

func (m *mockWorkspaceStore) ListWorkspaceTasks(_ context.Context, workspaceRoot string) ([]task.Task, error) {
	tasks := m.tasks[workspaceRoot]
	out := make([]task.Task, len(tasks))
	for i, item := range tasks {
		out[i] = copySchedulerTask(item)
	}
	return out, nil
}

func (m *mockWorkspaceStore) UpsertWorkspaceTask(_ context.Context, item task.Task) error {
	tasks := m.tasks[item.WorkspaceRoot]
	for i := range tasks {
		if tasks[i].ID == item.ID {
			tasks[i] = copySchedulerTask(item)
			m.tasks[item.WorkspaceRoot] = tasks
			return nil
		}
	}
	m.tasks[item.WorkspaceRoot] = append(tasks, copySchedulerTask(item))
	return nil
}

func (m *mockWorkspaceStore) GetWorkspaceTodos(_ context.Context, workspaceRoot string) ([]task.TodoItem, error) {
	todos := m.todos[workspaceRoot]
	if len(todos) == 0 {
		return nil, nil
	}
	out := make([]task.TodoItem, len(todos))
	copy(out, todos)
	return out, nil
}

func (m *mockWorkspaceStore) ReplaceWorkspaceTodos(_ context.Context, workspaceRoot string, todos []task.TodoItem) error {
	if len(todos) == 0 {
		m.todos[workspaceRoot] = nil
		return nil
	}
	out := make([]task.TodoItem, len(todos))
	copy(out, todos)
	m.todos[workspaceRoot] = out
	return nil
}

type mockAgentExecutor struct {
	runFn func(ctx context.Context, taskID, workspaceRoot, prompt string, activatedSkillNames []string, targetRole string, observer task.AgentTaskObserver) error
}

func (m mockAgentExecutor) RunTask(ctx context.Context, taskID, workspaceRoot, prompt string, activatedSkillNames []string, targetRole string, observer task.AgentTaskObserver) error {
	if m.runFn != nil {
		return m.runFn(ctx, taskID, workspaceRoot, prompt, activatedSkillNames, targetRole, observer)
	}
	return nil
}

func copySchedulerTask(in task.Task) task.Task {
	out := in
	out.ActivatedSkillNames = append([]string(nil), in.ActivatedSkillNames...)
	if in.EndTime != nil {
		end := *in.EndTime
		out.EndTime = &end
	}
	if in.FinalResultReadyAt != nil {
		readyAt := *in.FinalResultReadyAt
		out.FinalResultReadyAt = &readyAt
	}
	if in.CompletionNotifiedAt != nil {
		notifiedAt := *in.CompletionNotifiedAt
		out.CompletionNotifiedAt = &notifiedAt
	}
	return out
}

func newSchedulerService(t *testing.T, store Store, executor task.AgentExecutor, now time.Time) (*Service, *task.Manager) {
	t.Helper()

	manager := task.NewManager(task.Config{
		WorkspaceStore: newMockWorkspaceStore(),
	}, nil, executor)
	service := NewService(store, manager)
	service.SetClock(func() time.Time {
		return now
	})
	return service, manager
}

func baseCreateJobInput(workspaceRoot string) CreateJobInput {
	return CreateJobInput{
		Name:                "Daily report",
		WorkspaceRoot:       workspaceRoot,
		OwnerSessionID:      "session-1",
		Prompt:              "Summarize the workspace status",
		ActivatedSkillNames: []string{"plan", "report"},
	}
}

func TestBuildScheduledJob(t *testing.T) {
	now := time.Date(2026, 4, 29, 12, 0, 0, 0, time.UTC)
	workspaceRoot := filepath.ToSlash("/tmp/workspace")

	t.Run("valid at job", func(t *testing.T) {
		runAt := now.Add(2 * time.Hour)
		job, err := buildScheduledJob(now, CreateJobInput{
			Name:           "At job",
			WorkspaceRoot:  workspaceRoot,
			OwnerSessionID: "session-1",
			Prompt:         "Run once later",
			RunAt:          runAt,
		})
		if err != nil {
			t.Errorf("buildScheduledJob returned error: %v", err)
			return
		}
		if job.Kind != types.ScheduleKindAt {
			t.Errorf("Kind = %q, want %q", job.Kind, types.ScheduleKindAt)
		}
		if !job.Enabled {
			t.Errorf("Enabled = false, want true")
		}
		if !job.NextRunAt.Equal(runAt) {
			t.Errorf("NextRunAt = %v, want %v", job.NextRunAt, runAt)
		}
	})

	t.Run("valid every job", func(t *testing.T) {
		job, err := buildScheduledJob(now, CreateJobInput{
			Name:           "Every job",
			WorkspaceRoot:  workspaceRoot,
			OwnerSessionID: "session-1",
			Prompt:         "Run every 30 minutes",
			EveryMinutes:   30,
		})
		if err != nil {
			t.Errorf("buildScheduledJob returned error: %v", err)
			return
		}
		if job.Kind != types.ScheduleKindEvery {
			t.Errorf("Kind = %q, want %q", job.Kind, types.ScheduleKindEvery)
		}
	})

	t.Run("valid cron job", func(t *testing.T) {
		job, err := buildScheduledJob(now, CreateJobInput{
			Name:           "Cron job",
			WorkspaceRoot:  workspaceRoot,
			OwnerSessionID: "session-1",
			Prompt:         "Run every morning",
			CronExpr:       "0 9 * * *",
		})
		if err != nil {
			t.Errorf("buildScheduledJob returned error: %v", err)
			return
		}
		if job.Kind != types.ScheduleKindCron {
			t.Errorf("Kind = %q, want %q", job.Kind, types.ScheduleKindCron)
		}
	})

	t.Run("valid delay job", func(t *testing.T) {
		job, err := buildScheduledJob(now, CreateJobInput{
			Name:           "Delay job",
			WorkspaceRoot:  workspaceRoot,
			OwnerSessionID: "session-1",
			Prompt:         "Run after delay",
			DelayMinutes:   10,
		})
		if err != nil {
			t.Errorf("buildScheduledJob returned error: %v", err)
			return
		}
		want := now.Add(10 * time.Minute)
		if job.Kind != types.ScheduleKindAt {
			t.Errorf("Kind = %q, want %q", job.Kind, types.ScheduleKindAt)
		}
		if !job.RunAt.Equal(want) {
			t.Errorf("RunAt = %v, want %v", job.RunAt, want)
		}
	})

	t.Run("missing workspace root", func(t *testing.T) {
		_, err := buildScheduledJob(now, CreateJobInput{
			OwnerSessionID: "session-1",
			Prompt:         "Prompt",
			EveryMinutes:   5,
		})
		if err == nil || !strings.Contains(err.Error(), "workspace_root is required") {
			t.Errorf("error = %v, want workspace_root error", err)
		}
	})

	t.Run("missing owner session id", func(t *testing.T) {
		_, err := buildScheduledJob(now, CreateJobInput{
			WorkspaceRoot: workspaceRoot,
			Prompt:        "Prompt",
			EveryMinutes:  5,
		})
		if err == nil || !strings.Contains(err.Error(), "owner_session_id is required") {
			t.Errorf("error = %v, want owner_session_id error", err)
		}
	})

	t.Run("missing prompt", func(t *testing.T) {
		_, err := buildScheduledJob(now, CreateJobInput{
			WorkspaceRoot:  workspaceRoot,
			OwnerSessionID: "session-1",
			EveryMinutes:   5,
		})
		if err == nil || !strings.Contains(err.Error(), "prompt is required") {
			t.Errorf("error = %v, want prompt error", err)
		}
	})

	t.Run("no schedule selector", func(t *testing.T) {
		_, err := buildScheduledJob(now, CreateJobInput{
			WorkspaceRoot:  workspaceRoot,
			OwnerSessionID: "session-1",
			Prompt:         "Prompt",
		})
		if err == nil || !strings.Contains(err.Error(), "one of delay_minutes, run_at, every_minutes, or cron is required") {
			t.Errorf("error = %v, want missing selector error", err)
		}
	})

	t.Run("multiple schedule selectors", func(t *testing.T) {
		_, err := buildScheduledJob(now, CreateJobInput{
			WorkspaceRoot:  workspaceRoot,
			OwnerSessionID: "session-1",
			Prompt:         "Prompt",
			RunAt:          now.Add(time.Hour),
			EveryMinutes:   30,
		})
		if err == nil || !strings.Contains(err.Error(), "exactly one schedule selector is allowed") {
			t.Errorf("error = %v, want multiple selector error", err)
		}
	})

	t.Run("run at in the past", func(t *testing.T) {
		_, err := buildScheduledJob(now, CreateJobInput{
			WorkspaceRoot:  workspaceRoot,
			OwnerSessionID: "session-1",
			Prompt:         "Prompt",
			RunAt:          now.Add(-time.Minute),
		})
		if err == nil || !strings.Contains(err.Error(), "run_at must not be in the past") {
			t.Errorf("error = %v, want run_at past error", err)
		}
	})

	t.Run("skip if running explicit false", func(t *testing.T) {
		skipIfRunning := false
		job, err := buildScheduledJob(now, CreateJobInput{
			WorkspaceRoot:  workspaceRoot,
			OwnerSessionID: "session-1",
			Prompt:         "Prompt",
			EveryMinutes:   15,
			SkipIfRunning:  &skipIfRunning,
		})
		if err != nil {
			t.Errorf("buildScheduledJob returned error: %v", err)
			return
		}
		if job.SkipIfRunning {
			t.Errorf("SkipIfRunning = true, want false")
		}
	})

	t.Run("name auto generated from prompt", func(t *testing.T) {
		prompt := "This prompt is intentionally longer than forty eight runes so it gets truncated"
		job, err := buildScheduledJob(now, CreateJobInput{
			WorkspaceRoot:  workspaceRoot,
			OwnerSessionID: "session-1",
			Prompt:         prompt,
			EveryMinutes:   15,
		})
		if err != nil {
			t.Errorf("buildScheduledJob returned error: %v", err)
			return
		}
		if job.Name != clampJobName(prompt) {
			t.Errorf("Name = %q, want %q", job.Name, clampJobName(prompt))
		}
	})

	t.Run("empty timezone defaults to UTC", func(t *testing.T) {
		job, err := buildScheduledJob(now, CreateJobInput{
			WorkspaceRoot:  workspaceRoot,
			OwnerSessionID: "session-1",
			Prompt:         "Prompt",
			CronExpr:       "0 9 * * *",
		})
		if err != nil {
			t.Errorf("buildScheduledJob returned error: %v", err)
			return
		}
		if job.Timezone != "UTC" {
			t.Errorf("Timezone = %q, want UTC", job.Timezone)
		}
	})

	t.Run("timeout seconds zero defaults", func(t *testing.T) {
		job, err := buildScheduledJob(now, CreateJobInput{
			WorkspaceRoot:  workspaceRoot,
			OwnerSessionID: "session-1",
			Prompt:         "Prompt",
			EveryMinutes:   5,
			TimeoutSeconds: 0,
		})
		if err != nil {
			t.Errorf("buildScheduledJob returned error: %v", err)
			return
		}
		if job.TimeoutSeconds != defaultTaskTimeoutSeconds {
			t.Errorf("TimeoutSeconds = %d, want %d", job.TimeoutSeconds, defaultTaskTimeoutSeconds)
		}
	})
}

func TestInitialNextRun(t *testing.T) {
	now := time.Date(2026, 4, 29, 8, 30, 0, 0, time.UTC)

	t.Run("schedule kind at", func(t *testing.T) {
		runAt := now.Add(2 * time.Hour)
		next, enabled, err := initialNextRun(types.ScheduledJob{
			Kind:  types.ScheduleKindAt,
			RunAt: runAt,
		}, now)
		if err != nil {
			t.Errorf("initialNextRun returned error: %v", err)
			return
		}
		if !next.Equal(runAt) {
			t.Errorf("next = %v, want %v", next, runAt)
		}
		if !enabled {
			t.Errorf("enabled = false, want true")
		}
	})

	t.Run("schedule kind at with zero run at", func(t *testing.T) {
		_, _, err := initialNextRun(types.ScheduledJob{Kind: types.ScheduleKindAt}, now)
		if err == nil || !strings.Contains(err.Error(), "run_at is required for at jobs") {
			t.Errorf("error = %v, want run_at error", err)
		}
	})

	t.Run("schedule kind every", func(t *testing.T) {
		next, enabled, err := initialNextRun(types.ScheduledJob{
			Kind:         types.ScheduleKindEvery,
			EveryMinutes: 30,
		}, now)
		if err != nil {
			t.Errorf("initialNextRun returned error: %v", err)
			return
		}
		want := now.Add(30 * time.Minute)
		if !next.Equal(want) {
			t.Errorf("next = %v, want %v", next, want)
		}
		if !enabled {
			t.Errorf("enabled = false, want true")
		}
	})

	t.Run("schedule kind every with zero minutes", func(t *testing.T) {
		_, _, err := initialNextRun(types.ScheduledJob{
			Kind: types.ScheduleKindEvery,
		}, now)
		if err == nil || !strings.Contains(err.Error(), "every_minutes must be greater than zero") {
			t.Errorf("error = %v, want every_minutes error", err)
		}
	})

	t.Run("schedule kind cron", func(t *testing.T) {
		next, enabled, err := initialNextRun(types.ScheduledJob{
			Kind:     types.ScheduleKindCron,
			CronExpr: "0 9 * * *",
			Timezone: "UTC",
		}, now)
		if err != nil {
			t.Errorf("initialNextRun returned error: %v", err)
			return
		}
		want := time.Date(2026, 4, 29, 9, 0, 0, 0, time.UTC)
		if !next.Equal(want) {
			t.Errorf("next = %v, want %v", next, want)
		}
		if !enabled {
			t.Errorf("enabled = false, want true")
		}
	})

	t.Run("schedule kind cron with empty expression", func(t *testing.T) {
		_, _, err := initialNextRun(types.ScheduledJob{
			Kind: types.ScheduleKindCron,
		}, now)
		if err == nil || !strings.Contains(err.Error(), "cron expression is required") {
			t.Errorf("error = %v, want cron expression error", err)
		}
	})

	t.Run("schedule kind cron with invalid expression", func(t *testing.T) {
		_, _, err := initialNextRun(types.ScheduledJob{
			Kind:     types.ScheduleKindCron,
			CronExpr: "not-a-cron",
		}, now)
		if err == nil {
			t.Errorf("error = nil, want invalid cron error")
		}
	})

	t.Run("schedule kind cron with invalid timezone", func(t *testing.T) {
		_, _, err := initialNextRun(types.ScheduledJob{
			Kind:     types.ScheduleKindCron,
			CronExpr: "0 9 * * *",
			Timezone: "Not/A_Real_Zone",
		}, now)
		if err == nil {
			t.Errorf("error = nil, want timezone error")
		}
	})

	t.Run("unsupported kind", func(t *testing.T) {
		_, _, err := initialNextRun(types.ScheduledJob{
			Kind: types.ScheduleKind("unknown"),
		}, now)
		if err == nil || !strings.Contains(err.Error(), "unsupported schedule kind") {
			t.Errorf("error = %v, want unsupported kind error", err)
		}
	})
}

func TestNextRunAfterDispatch(t *testing.T) {
	now := time.Date(2026, 4, 29, 8, 30, 0, 0, time.UTC)

	t.Run("schedule kind at", func(t *testing.T) {
		next, enabled, err := nextRunAfterDispatch(types.ScheduledJob{
			Kind:  types.ScheduleKindAt,
			RunAt: now,
		}, now)
		if err != nil {
			t.Errorf("nextRunAfterDispatch returned error: %v", err)
			return
		}
		if !next.IsZero() {
			t.Errorf("next = %v, want zero time", next)
		}
		if enabled {
			t.Errorf("enabled = true, want false")
		}
	})

	t.Run("schedule kind every", func(t *testing.T) {
		job := types.ScheduledJob{
			Kind:         types.ScheduleKindEvery,
			EveryMinutes: 30,
			NextRunAt:    now,
		}
		next, enabled, err := nextRunAfterDispatch(job, now)
		if err != nil {
			t.Errorf("nextRunAfterDispatch returned error: %v", err)
			return
		}
		want := now.Add(30 * time.Minute)
		if !next.Equal(want) {
			t.Errorf("next = %v, want %v", next, want)
		}
		if !enabled {
			t.Errorf("enabled = false, want true")
		}
	})

	t.Run("schedule kind every with base in past", func(t *testing.T) {
		job := types.ScheduledJob{
			Kind:         types.ScheduleKindEvery,
			EveryMinutes: 15,
			NextRunAt:    now.Add(-40 * time.Minute),
		}
		next, enabled, err := nextRunAfterDispatch(job, now)
		if err != nil {
			t.Errorf("nextRunAfterDispatch returned error: %v", err)
			return
		}
		want := now.Add(5 * time.Minute)
		if !next.Equal(want) {
			t.Errorf("next = %v, want %v", next, want)
		}
		if !enabled {
			t.Errorf("enabled = false, want true")
		}
	})

	t.Run("schedule kind cron", func(t *testing.T) {
		next, enabled, err := nextRunAfterDispatch(types.ScheduledJob{
			Kind:     types.ScheduleKindCron,
			CronExpr: "0 9 * * *",
			Timezone: "UTC",
		}, now)
		if err != nil {
			t.Errorf("nextRunAfterDispatch returned error: %v", err)
			return
		}
		want := time.Date(2026, 4, 29, 9, 0, 0, 0, time.UTC)
		if !next.Equal(want) {
			t.Errorf("next = %v, want %v", next, want)
		}
		if !enabled {
			t.Errorf("enabled = false, want true")
		}
	})

	t.Run("schedule kind every with zero every minutes", func(t *testing.T) {
		_, _, err := nextRunAfterDispatch(types.ScheduledJob{
			Kind: types.ScheduleKindEvery,
		}, now)
		if err == nil || !strings.Contains(err.Error(), "every_minutes must be greater than zero") {
			t.Errorf("error = %v, want every_minutes error", err)
		}
	})

	t.Run("schedule kind cron with empty expression", func(t *testing.T) {
		_, _, err := nextRunAfterDispatch(types.ScheduledJob{
			Kind: types.ScheduleKindCron,
		}, now)
		if err == nil || !strings.Contains(err.Error(), "cron expression is required") {
			t.Errorf("error = %v, want cron expression error", err)
		}
	})

	t.Run("schedule kind cron with invalid timezone", func(t *testing.T) {
		_, _, err := nextRunAfterDispatch(types.ScheduledJob{
			Kind:     types.ScheduleKindCron,
			CronExpr: "0 9 * * *",
			Timezone: "Not/A_Real_Zone",
		}, now)
		if err == nil {
			t.Errorf("error = nil, want timezone error")
		}
	})

	t.Run("unsupported kind", func(t *testing.T) {
		_, _, err := nextRunAfterDispatch(types.ScheduledJob{
			Kind: types.ScheduleKind("unknown"),
		}, now)
		if err == nil || !strings.Contains(err.Error(), "unsupported schedule kind") {
			t.Errorf("error = %v, want unsupported kind error", err)
		}
	})
}

func TestHandleDueJob(t *testing.T) {
	now := time.Date(2026, 4, 29, 12, 0, 0, 0, time.UTC)

	t.Run("empty job id", func(t *testing.T) {
		store := newMockStore()
		service, _ := newSchedulerService(t, store, mockAgentExecutor{}, now)
		err := service.handleDueJob(context.Background(), types.ScheduledJob{}, now)
		if err != nil {
			t.Errorf("handleDueJob returned error: %v", err)
		}
	})

	t.Run("disabled job", func(t *testing.T) {
		store := newMockStore()
		service, _ := newSchedulerService(t, store, mockAgentExecutor{}, now)
		err := service.handleDueJob(context.Background(), types.ScheduledJob{
			ID:        "job-1",
			Enabled:   false,
			NextRunAt: now.Add(-time.Minute),
		}, now)
		if err != nil {
			t.Errorf("handleDueJob returned error: %v", err)
		}
	})

	t.Run("job with zero next run at", func(t *testing.T) {
		store := newMockStore()
		service, _ := newSchedulerService(t, store, mockAgentExecutor{}, now)
		err := service.handleDueJob(context.Background(), types.ScheduledJob{
			ID:      "job-1",
			Enabled: true,
		}, now)
		if err != nil {
			t.Errorf("handleDueJob returned error: %v", err)
		}
	})

	t.Run("job with next run at in the future", func(t *testing.T) {
		store := newMockStore()
		service, _ := newSchedulerService(t, store, mockAgentExecutor{}, now)
		err := service.handleDueJob(context.Background(), types.ScheduledJob{
			ID:        "job-1",
			Enabled:   true,
			NextRunAt: now.Add(time.Minute),
		}, now)
		if err != nil {
			t.Errorf("handleDueJob returned error: %v", err)
		}
	})

	t.Run("active previous task with skip if running true", func(t *testing.T) {
		store := newMockStore()
		service, manager := newSchedulerService(t, store, mockAgentExecutor{}, now)
		workspaceRoot := filepath.ToSlash(t.TempDir())

		activeTask, err := manager.Create(context.Background(), task.CreateTaskInput{
			ID:            "task-1",
			Type:          task.TaskTypeAgent,
			Command:       "run",
			WorkspaceRoot: workspaceRoot,
		})
		if err != nil {
			t.Fatalf("Create returned error: %v", err)
		}

		job := types.ScheduledJob{
			ID:             "job-1",
			Name:           "Every job",
			WorkspaceRoot:  workspaceRoot,
			OwnerSessionID: "session-1",
			Prompt:         "Run every 30 minutes",
			Kind:           types.ScheduleKindEvery,
			EveryMinutes:   30,
			Enabled:        true,
			SkipIfRunning:  true,
			NextRunAt:      now.Add(-time.Minute),
			LastTaskID:     activeTask.ID,
		}
		if err := store.UpsertScheduledJob(context.Background(), job); err != nil {
			t.Fatalf("UpsertScheduledJob returned error: %v", err)
		}

		err = service.handleDueJob(context.Background(), job, now)
		if err != nil {
			t.Errorf("handleDueJob returned error: %v", err)
			return
		}

		updated := store.jobs[job.ID]
		if updated.LastStatus != types.ScheduledJobStatusSkipped {
			t.Errorf("LastStatus = %q, want %q", updated.LastStatus, types.ScheduledJobStatusSkipped)
		}
		if updated.SkipCount != 1 {
			t.Errorf("SkipCount = %d, want 1", updated.SkipCount)
		}
		if !updated.LastSkipAt.Equal(now) {
			t.Errorf("LastSkipAt = %v, want %v", updated.LastSkipAt, now)
		}
		if updated.LastError != "previous run still active" {
			t.Errorf("LastError = %q, want %q", updated.LastError, "previous run still active")
		}
		wantNext := now.Add(29 * time.Minute)
		if !updated.NextRunAt.Equal(wantNext) {
			t.Errorf("NextRunAt = %v, want %v", updated.NextRunAt, wantNext)
		}
	})

	t.Run("active previous task with skip if running false", func(t *testing.T) {
		store := newMockStore()
		service, manager := newSchedulerService(t, store, mockAgentExecutor{}, now)
		workspaceRoot := filepath.ToSlash(t.TempDir())

		activeTask, err := manager.Create(context.Background(), task.CreateTaskInput{
			ID:            "task-1",
			Type:          task.TaskTypeAgent,
			Command:       "run",
			WorkspaceRoot: workspaceRoot,
		})
		if err != nil {
			t.Fatalf("Create returned error: %v", err)
		}

		job := types.ScheduledJob{
			ID:             "job-1",
			Name:           "Every job",
			WorkspaceRoot:  workspaceRoot,
			OwnerSessionID: "session-1",
			Prompt:         "Run every 30 minutes",
			Kind:           types.ScheduleKindEvery,
			EveryMinutes:   30,
			Enabled:        true,
			SkipIfRunning:  false,
			NextRunAt:      now.Add(-time.Minute),
			LastTaskID:     activeTask.ID,
		}
		if err := store.UpsertScheduledJob(context.Background(), job); err != nil {
			t.Fatalf("UpsertScheduledJob returned error: %v", err)
		}

		err = service.handleDueJob(context.Background(), job, now)
		if err != nil {
			t.Errorf("handleDueJob returned error: %v", err)
			return
		}

		updated := store.jobs[job.ID]
		if updated.SkipCount != 0 {
			t.Errorf("SkipCount = %d, want 0", updated.SkipCount)
		}
		if updated.LastStatus != "" {
			t.Errorf("LastStatus = %q, want empty", updated.LastStatus)
		}
		if !updated.NextRunAt.Equal(job.NextRunAt) {
			t.Errorf("NextRunAt = %v, want %v", updated.NextRunAt, job.NextRunAt)
		}
	})

	t.Run("successful dispatch", func(t *testing.T) {
		store := newMockStore()
		service, manager := newSchedulerService(t, store, mockAgentExecutor{}, now)
		workspaceRoot := filepath.ToSlash(t.TempDir())

		job := types.ScheduledJob{
			ID:                  "job-1",
			Name:                "Every job",
			WorkspaceRoot:       workspaceRoot,
			OwnerSessionID:      "session-1",
			Prompt:              "Run every 30 minutes",
			Kind:                types.ScheduleKindEvery,
			EveryMinutes:        30,
			Enabled:             true,
			SkipIfRunning:       true,
			TimeoutSeconds:      120,
			ActivatedSkillNames: []string{"plan"},
			NextRunAt:           now.Add(-time.Minute),
		}
		if err := store.UpsertScheduledJob(context.Background(), job); err != nil {
			t.Fatalf("UpsertScheduledJob returned error: %v", err)
		}

		err := service.handleDueJob(context.Background(), job, now)
		if err != nil {
			t.Errorf("handleDueJob returned error: %v", err)
			return
		}

		updated := store.jobs[job.ID]
		if updated.LastStatus != types.ScheduledJobStatusRunning {
			t.Errorf("LastStatus = %q, want %q", updated.LastStatus, types.ScheduledJobStatusRunning)
		}
		if updated.TotalRuns != 1 {
			t.Errorf("TotalRuns = %d, want 1", updated.TotalRuns)
		}
		if updated.LastTaskID == "" {
			t.Errorf("LastTaskID = empty, want task ID")
		}
		wantNext := now.Add(29 * time.Minute)
		if !updated.NextRunAt.Equal(wantNext) {
			t.Errorf("NextRunAt = %v, want %v", updated.NextRunAt, wantNext)
		}
		if updated.LastError != "" {
			t.Errorf("LastError = %q, want empty", updated.LastError)
		}
		createdTask, ok, err := manager.Get(updated.LastTaskID, workspaceRoot)
		if err != nil {
			t.Errorf("Get returned error: %v", err)
		} else if !ok {
			t.Errorf("Get returned ok=false, want true")
		} else if createdTask.ID != updated.LastTaskID {
			t.Errorf("task ID = %q, want %q", createdTask.ID, updated.LastTaskID)
		}
	})

	t.Run("task creation fails", func(t *testing.T) {
		store := newMockStore()
		service, _ := newSchedulerService(t, store, nil, now)
		workspaceRoot := filepath.ToSlash(t.TempDir())

		job := types.ScheduledJob{
			ID:             "job-1",
			Name:           "Every job",
			WorkspaceRoot:  workspaceRoot,
			OwnerSessionID: "session-1",
			Prompt:         "Run every 30 minutes",
			Kind:           types.ScheduleKindEvery,
			EveryMinutes:   30,
			Enabled:        true,
			SkipIfRunning:  true,
			NextRunAt:      now.Add(-time.Minute),
		}
		if err := store.UpsertScheduledJob(context.Background(), job); err != nil {
			t.Fatalf("UpsertScheduledJob returned error: %v", err)
		}

		err := service.handleDueJob(context.Background(), job, now)
		if err != nil {
			t.Errorf("handleDueJob returned error: %v", err)
			return
		}

		updated := store.jobs[job.ID]
		if updated.LastStatus != types.ScheduledJobStatusFailed {
			t.Errorf("LastStatus = %q, want %q", updated.LastStatus, types.ScheduledJobStatusFailed)
		}
		if updated.FailCount != 1 {
			t.Errorf("FailCount = %d, want 1", updated.FailCount)
		}
		if !strings.Contains(updated.LastError, "not supported") {
			t.Errorf("LastError = %q, want not supported error", updated.LastError)
		}
	})
}

func TestRecordTaskTerminal(t *testing.T) {
	now := time.Date(2026, 4, 29, 12, 0, 0, 0, time.UTC)

	baseJob := types.ScheduledJob{
		ID:             "job-1",
		Name:           "Scheduled job",
		WorkspaceRoot:  filepath.ToSlash(t.TempDir()),
		OwnerSessionID: "session-1",
		Prompt:         "Prompt",
	}

	t.Run("completed task", func(t *testing.T) {
		store := newMockStore()
		service := NewService(store, nil)
		service.SetClock(func() time.Time { return now })
		if err := store.UpsertScheduledJob(context.Background(), baseJob); err != nil {
			t.Fatalf("UpsertScheduledJob returned error: %v", err)
		}

		endTime := now.Add(time.Minute)
		err := service.RecordTaskTerminal(context.Background(), task.Task{
			ID:             "task-1",
			ScheduledJobID: baseJob.ID,
			Status:         task.TaskStatusCompleted,
			EndTime:        &endTime,
		})
		if err != nil {
			t.Errorf("RecordTaskTerminal returned error: %v", err)
			return
		}

		job := store.jobs[baseJob.ID]
		if job.LastStatus != types.ScheduledJobStatusSucceeded {
			t.Errorf("LastStatus = %q, want %q", job.LastStatus, types.ScheduledJobStatusSucceeded)
		}
		if job.SuccessCount != 1 {
			t.Errorf("SuccessCount = %d, want 1", job.SuccessCount)
		}
		if job.LastError != "" {
			t.Errorf("LastError = %q, want empty", job.LastError)
		}
	})

	t.Run("failed task", func(t *testing.T) {
		store := newMockStore()
		service := NewService(store, nil)
		if err := store.UpsertScheduledJob(context.Background(), baseJob); err != nil {
			t.Fatalf("UpsertScheduledJob returned error: %v", err)
		}

		err := service.RecordTaskTerminal(context.Background(), task.Task{
			ID:             "task-1",
			ScheduledJobID: baseJob.ID,
			Status:         task.TaskStatusFailed,
			Error:          "boom",
		})
		if err != nil {
			t.Errorf("RecordTaskTerminal returned error: %v", err)
			return
		}

		job := store.jobs[baseJob.ID]
		if job.LastStatus != types.ScheduledJobStatusFailed {
			t.Errorf("LastStatus = %q, want %q", job.LastStatus, types.ScheduledJobStatusFailed)
		}
		if job.FailCount != 1 {
			t.Errorf("FailCount = %d, want 1", job.FailCount)
		}
		if job.LastError != "boom" {
			t.Errorf("LastError = %q, want %q", job.LastError, "boom")
		}
	})

	t.Run("stopped task", func(t *testing.T) {
		store := newMockStore()
		service := NewService(store, nil)
		if err := store.UpsertScheduledJob(context.Background(), baseJob); err != nil {
			t.Fatalf("UpsertScheduledJob returned error: %v", err)
		}

		err := service.RecordTaskTerminal(context.Background(), task.Task{
			ID:             "task-1",
			ScheduledJobID: baseJob.ID,
			Status:         task.TaskStatusStopped,
		})
		if err != nil {
			t.Errorf("RecordTaskTerminal returned error: %v", err)
			return
		}

		job := store.jobs[baseJob.ID]
		if job.LastStatus != types.ScheduledJobStatusFailed {
			t.Errorf("LastStatus = %q, want %q", job.LastStatus, types.ScheduledJobStatusFailed)
		}
		if job.FailCount != 1 {
			t.Errorf("FailCount = %d, want 1", job.FailCount)
		}
		if job.LastError != "task stopped" {
			t.Errorf("LastError = %q, want %q", job.LastError, "task stopped")
		}
	})

	t.Run("empty scheduled job id", func(t *testing.T) {
		store := newMockStore()
		service := NewService(store, nil)
		if err := store.UpsertScheduledJob(context.Background(), baseJob); err != nil {
			t.Fatalf("UpsertScheduledJob returned error: %v", err)
		}

		err := service.RecordTaskTerminal(context.Background(), task.Task{
			ID:     "task-1",
			Status: task.TaskStatusCompleted,
		})
		if err != nil {
			t.Errorf("RecordTaskTerminal returned error: %v", err)
		}
		job := store.jobs[baseJob.ID]
		if job.LastStatus != "" {
			t.Errorf("LastStatus = %q, want empty", job.LastStatus)
		}
	})

	t.Run("job not found returns store error", func(t *testing.T) {
		expectedErr := errors.New("lookup failed")
		service := NewService(&errorGetJobStore{
			mockStore: newMockStore(),
			err:       expectedErr,
		}, nil)

		err := service.RecordTaskTerminal(context.Background(), task.Task{
			ID:             "task-1",
			ScheduledJobID: "missing",
			Status:         task.TaskStatusCompleted,
		})
		if !errors.Is(err, expectedErr) {
			t.Errorf("error = %v, want %v", err, expectedErr)
		}
	})

	t.Run("pending task status", func(t *testing.T) {
		store := newMockStore()
		service := NewService(store, nil)
		if err := store.UpsertScheduledJob(context.Background(), baseJob); err != nil {
			t.Fatalf("UpsertScheduledJob returned error: %v", err)
		}

		err := service.RecordTaskTerminal(context.Background(), task.Task{
			ID:             "task-1",
			ScheduledJobID: baseJob.ID,
			Status:         task.TaskStatusPending,
		})
		if err != nil {
			t.Errorf("RecordTaskTerminal returned error: %v", err)
		}
		job := store.jobs[baseJob.ID]
		if job.LastStatus != "" {
			t.Errorf("LastStatus = %q, want empty", job.LastStatus)
		}
	})

	t.Run("uses end time when set", func(t *testing.T) {
		store := newMockStore()
		service := NewService(store, nil)
		if err := store.UpsertScheduledJob(context.Background(), baseJob); err != nil {
			t.Fatalf("UpsertScheduledJob returned error: %v", err)
		}

		endTime := now.Add(2 * time.Minute)
		readyAt := now.Add(3 * time.Minute)
		err := service.RecordTaskTerminal(context.Background(), task.Task{
			ID:                 "task-1",
			ScheduledJobID:     baseJob.ID,
			Status:             task.TaskStatusCompleted,
			EndTime:            &endTime,
			FinalResultReadyAt: &readyAt,
		})
		if err != nil {
			t.Errorf("RecordTaskTerminal returned error: %v", err)
			return
		}
		job := store.jobs[baseJob.ID]
		if !job.LastRunAt.Equal(endTime) {
			t.Errorf("LastRunAt = %v, want %v", job.LastRunAt, endTime)
		}
	})

	t.Run("falls back to final result ready at", func(t *testing.T) {
		store := newMockStore()
		service := NewService(store, nil)
		if err := store.UpsertScheduledJob(context.Background(), baseJob); err != nil {
			t.Fatalf("UpsertScheduledJob returned error: %v", err)
		}

		readyAt := now.Add(4 * time.Minute)
		err := service.RecordTaskTerminal(context.Background(), task.Task{
			ID:                 "task-1",
			ScheduledJobID:     baseJob.ID,
			Status:             task.TaskStatusCompleted,
			FinalResultReadyAt: &readyAt,
		})
		if err != nil {
			t.Errorf("RecordTaskTerminal returned error: %v", err)
			return
		}
		job := store.jobs[baseJob.ID]
		if !job.LastRunAt.Equal(readyAt) {
			t.Errorf("LastRunAt = %v, want %v", job.LastRunAt, readyAt)
		}
	})
}

func TestCreateJob(t *testing.T) {
	now := time.Date(2026, 4, 29, 12, 0, 0, 0, time.UTC)
	workspaceRoot := filepath.ToSlash(t.TempDir())

	t.Run("successful creation with non transactional store", func(t *testing.T) {
		store := newMockStore()
		service, _ := newSchedulerService(t, store, mockAgentExecutor{}, now)

		job, err := service.CreateJob(context.Background(), CreateJobInput{
			Name:                "Every job",
			WorkspaceRoot:       workspaceRoot,
			OwnerSessionID:      "session-1",
			Prompt:              "Run every 15 minutes",
			ActivatedSkillNames: []string{"plan", "PLAN"},
			EveryMinutes:        15,
		})
		if err != nil {
			t.Errorf("CreateJob returned error: %v", err)
			return
		}
		storedJob, ok := store.jobs[job.ID]
		if !ok {
			t.Errorf("job %q not stored", job.ID)
		} else if storedJob.ID != job.ID {
			t.Errorf("stored job ID = %q, want %q", storedJob.ID, job.ID)
		}
		spec, ok := store.childSpecs[job.ID]
		if !ok {
			t.Errorf("child spec %q not stored", job.ID)
		} else if spec.AgentID != job.ID {
			t.Errorf("child spec AgentID = %q, want %q", spec.AgentID, job.ID)
		}
	})

	t.Run("successful creation with transactional store", func(t *testing.T) {
		store := &mockTxStore{mockStore: newMockStore()}
		service, _ := newSchedulerService(t, store, mockAgentExecutor{}, now)

		job, err := service.CreateJob(context.Background(), CreateJobInput{
			Name:           "Every job",
			WorkspaceRoot:  workspaceRoot,
			OwnerSessionID: "session-1",
			Prompt:         "Run every 15 minutes",
			EveryMinutes:   15,
		})
		if err != nil {
			t.Errorf("CreateJob returned error: %v", err)
			return
		}
		if _, ok := store.jobs[job.ID]; !ok {
			t.Errorf("job %q not stored", job.ID)
		}
		if _, ok := store.childSpecs[job.ID]; !ok {
			t.Errorf("child spec %q not stored", job.ID)
		}
	})

	t.Run("validation error", func(t *testing.T) {
		store := newMockStore()
		service, _ := newSchedulerService(t, store, mockAgentExecutor{}, now)

		_, err := service.CreateJob(context.Background(), CreateJobInput{
			Name:           "Invalid job",
			WorkspaceRoot:  workspaceRoot,
			OwnerSessionID: "session-1",
			EveryMinutes:   15,
		})
		if err == nil {
			t.Errorf("error = nil, want validation error")
		}
		if len(store.jobs) != 0 {
			t.Errorf("stored jobs = %d, want 0", len(store.jobs))
		}
		if len(store.childSpecs) != 0 {
			t.Errorf("stored child specs = %d, want 0", len(store.childSpecs))
		}
	})

	t.Run("report group validation", func(t *testing.T) {
		store := newMockStore()
		service, _ := newSchedulerService(t, store, mockAgentExecutor{}, now)

		job, err := service.CreateJob(context.Background(), CreateJobInput{
			Name:                    "Every job",
			WorkspaceRoot:           workspaceRoot,
			OwnerSessionID:          "session-1",
			Prompt:                  "Run every 15 minutes",
			EveryMinutes:            15,
			ReportGroupID:           "group-1",
			ReportGroupTitle:        "Reports",
			ReportGroupEveryMinutes: 60,
		})
		if err != nil {
			t.Errorf("CreateJob returned error: %v", err)
			return
		}

		group, ok := store.reportGroups["group-1"]
		if !ok {
			t.Errorf("report group not stored")
			return
		}
		if group.SessionID != "session-1" {
			t.Errorf("SessionID = %q, want %q", group.SessionID, "session-1")
		}
		if !slices.Equal(group.Sources, []string{job.ID}) {
			t.Errorf("Sources = %v, want [%s]", group.Sources, job.ID)
		}
		if group.Schedule.Kind != types.ScheduleKindEvery {
			t.Errorf("Schedule.Kind = %q, want %q", group.Schedule.Kind, types.ScheduleKindEvery)
		}
		if group.Schedule.EveryMinutes != 60 {
			t.Errorf("Schedule.EveryMinutes = %d, want 60", group.Schedule.EveryMinutes)
		}
		if !slices.Equal(group.Delivery.Channels, []string{string(types.ReportChannelAgent)}) {
			t.Errorf("Delivery.Channels = %v, want [%s]", group.Delivery.Channels, types.ReportChannelAgent)
		}
	})
}

func TestDeleteJob(t *testing.T) {
	now := time.Date(2026, 4, 29, 12, 0, 0, 0, time.UTC)
	workspaceRoot := filepath.ToSlash(t.TempDir())

	t.Run("delete existing job", func(t *testing.T) {
		store := newMockStore()
		service, _ := newSchedulerService(t, store, mockAgentExecutor{}, now)
		job := types.ScheduledJob{
			ID:             "job-1",
			WorkspaceRoot:  workspaceRoot,
			OwnerSessionID: "session-1",
			Prompt:         "Prompt",
		}
		if err := store.UpsertScheduledJob(context.Background(), job); err != nil {
			t.Fatalf("UpsertScheduledJob returned error: %v", err)
		}
		if err := store.UpsertChildAgentSpec(context.Background(), types.ChildAgentSpec{AgentID: job.ID}); err != nil {
			t.Fatalf("UpsertChildAgentSpec returned error: %v", err)
		}

		deleted, err := service.DeleteJob(context.Background(), job.ID)
		if err != nil {
			t.Errorf("DeleteJob returned error: %v", err)
			return
		}
		if !deleted {
			t.Errorf("deleted = false, want true")
		}
		if _, ok := store.jobs[job.ID]; ok {
			t.Errorf("job %q still present", job.ID)
		}
		if _, ok := store.childSpecs[job.ID]; ok {
			t.Errorf("child spec %q still present", job.ID)
		}
	})

	t.Run("delete non existent job", func(t *testing.T) {
		store := newMockStore()
		service, _ := newSchedulerService(t, store, mockAgentExecutor{}, now)

		deleted, err := service.DeleteJob(context.Background(), "missing")
		if err != nil {
			t.Errorf("DeleteJob returned error: %v", err)
		}
		if deleted {
			t.Errorf("deleted = true, want false")
		}
	})

	t.Run("delete with transactional store", func(t *testing.T) {
		store := &mockTxStore{mockStore: newMockStore()}
		service, _ := newSchedulerService(t, store, mockAgentExecutor{}, now)
		job := types.ScheduledJob{
			ID:             "job-1",
			WorkspaceRoot:  workspaceRoot,
			OwnerSessionID: "session-1",
			Prompt:         "Prompt",
		}
		if err := store.UpsertScheduledJob(context.Background(), job); err != nil {
			t.Fatalf("UpsertScheduledJob returned error: %v", err)
		}
		if err := store.UpsertChildAgentSpec(context.Background(), types.ChildAgentSpec{AgentID: job.ID}); err != nil {
			t.Fatalf("UpsertChildAgentSpec returned error: %v", err)
		}

		deleted, err := service.DeleteJob(context.Background(), job.ID)
		if err != nil {
			t.Errorf("DeleteJob returned error: %v", err)
			return
		}
		if !deleted {
			t.Errorf("deleted = false, want true")
		}
	})

	t.Run("delete with report group", func(t *testing.T) {
		store := newMockStore()
		service, _ := newSchedulerService(t, store, mockAgentExecutor{}, now)
		job := types.ScheduledJob{
			ID:             "job-1",
			WorkspaceRoot:  workspaceRoot,
			OwnerSessionID: "session-1",
			Prompt:         "Prompt",
			ReportGroupID:  "group-1",
		}
		if err := store.UpsertScheduledJob(context.Background(), job); err != nil {
			t.Fatalf("UpsertScheduledJob returned error: %v", err)
		}
		if err := store.UpsertChildAgentSpec(context.Background(), types.ChildAgentSpec{AgentID: job.ID}); err != nil {
			t.Fatalf("UpsertChildAgentSpec returned error: %v", err)
		}
		if err := store.UpsertReportGroup(context.Background(), types.ReportGroup{
			GroupID:   "group-1",
			SessionID: "session-1",
			Sources:   []string{"job-1", "job-2"},
		}); err != nil {
			t.Fatalf("UpsertReportGroup returned error: %v", err)
		}

		deleted, err := service.DeleteJob(context.Background(), job.ID)
		if err != nil {
			t.Errorf("DeleteJob returned error: %v", err)
			return
		}
		if !deleted {
			t.Errorf("deleted = false, want true")
		}
		group := store.reportGroups["group-1"]
		if !slices.Equal(group.Sources, []string{"job-2"}) {
			t.Errorf("Sources = %v, want [job-2]", group.Sources)
		}
	})
}

func TestSetJobEnabled(t *testing.T) {
	now := time.Date(2026, 4, 29, 12, 0, 0, 0, time.UTC)
	workspaceRoot := filepath.ToSlash(t.TempDir())

	t.Run("enable disabled job", func(t *testing.T) {
		store := newMockStore()
		service, _ := newSchedulerService(t, store, mockAgentExecutor{}, now)
		job := types.ScheduledJob{
			ID:             "job-1",
			WorkspaceRoot:  workspaceRoot,
			OwnerSessionID: "session-1",
			Prompt:         "Prompt",
			Kind:           types.ScheduleKindEvery,
			EveryMinutes:   15,
			Enabled:        false,
		}
		if err := store.UpsertScheduledJob(context.Background(), job); err != nil {
			t.Fatalf("UpsertScheduledJob returned error: %v", err)
		}

		updated, ok, err := service.SetJobEnabled(context.Background(), job.ID, true)
		if err != nil {
			t.Errorf("SetJobEnabled returned error: %v", err)
			return
		}
		if !ok {
			t.Errorf("ok = false, want true")
		}
		if !updated.Enabled {
			t.Errorf("Enabled = false, want true")
		}
		want := now.Add(15 * time.Minute)
		if !updated.NextRunAt.Equal(want) {
			t.Errorf("NextRunAt = %v, want %v", updated.NextRunAt, want)
		}
	})

	t.Run("disable enabled job", func(t *testing.T) {
		store := newMockStore()
		service, _ := newSchedulerService(t, store, mockAgentExecutor{}, now)
		job := types.ScheduledJob{
			ID:             "job-1",
			WorkspaceRoot:  workspaceRoot,
			OwnerSessionID: "session-1",
			Prompt:         "Prompt",
			Kind:           types.ScheduleKindEvery,
			EveryMinutes:   15,
			Enabled:        true,
			NextRunAt:      now.Add(15 * time.Minute),
		}
		if err := store.UpsertScheduledJob(context.Background(), job); err != nil {
			t.Fatalf("UpsertScheduledJob returned error: %v", err)
		}

		updated, ok, err := service.SetJobEnabled(context.Background(), job.ID, false)
		if err != nil {
			t.Errorf("SetJobEnabled returned error: %v", err)
			return
		}
		if !ok {
			t.Errorf("ok = false, want true")
		}
		if updated.Enabled {
			t.Errorf("Enabled = true, want false")
		}
	})
}

func TestListJobsAndGetJob(t *testing.T) {
	now := time.Date(2026, 4, 29, 12, 0, 0, 0, time.UTC)
	store := newMockStore()
	service, _ := newSchedulerService(t, store, mockAgentExecutor{}, now)
	workspaceOne := filepath.ToSlash(t.TempDir())
	workspaceTwo := filepath.ToSlash(t.TempDir())

	jobOne := types.ScheduledJob{ID: "job-1", WorkspaceRoot: workspaceOne}
	jobTwo := types.ScheduledJob{ID: "job-2", WorkspaceRoot: workspaceTwo}
	if err := store.UpsertScheduledJob(context.Background(), jobOne); err != nil {
		t.Fatalf("UpsertScheduledJob returned error: %v", err)
	}
	if err := store.UpsertScheduledJob(context.Background(), jobTwo); err != nil {
		t.Fatalf("UpsertScheduledJob returned error: %v", err)
	}

	t.Run("list jobs with empty workspace", func(t *testing.T) {
		jobs, err := service.ListJobs(context.Background(), "")
		if err != nil {
			t.Errorf("ListJobs returned error: %v", err)
			return
		}
		if len(jobs) != 2 {
			t.Errorf("len(jobs) = %d, want 2", len(jobs))
		}
	})

	t.Run("list jobs with workspace filter", func(t *testing.T) {
		jobs, err := service.ListJobs(context.Background(), workspaceOne)
		if err != nil {
			t.Errorf("ListJobs returned error: %v", err)
			return
		}
		if len(jobs) != 1 {
			t.Errorf("len(jobs) = %d, want 1", len(jobs))
			return
		}
		if jobs[0].ID != jobOne.ID {
			t.Errorf("jobs[0].ID = %q, want %q", jobs[0].ID, jobOne.ID)
		}
	})

	t.Run("get job found", func(t *testing.T) {
		job, ok, err := service.GetJob(context.Background(), jobOne.ID)
		if err != nil {
			t.Errorf("GetJob returned error: %v", err)
			return
		}
		if !ok {
			t.Errorf("ok = false, want true")
			return
		}
		if job.ID != jobOne.ID {
			t.Errorf("ID = %q, want %q", job.ID, jobOne.ID)
		}
	})

	t.Run("get job not found", func(t *testing.T) {
		_, ok, err := service.GetJob(context.Background(), "missing")
		if err != nil {
			t.Errorf("GetJob returned error: %v", err)
		}
		if ok {
			t.Errorf("ok = true, want false")
		}
	})
}

func TestHelperFunctions(t *testing.T) {
	t.Run("normalized timeout", func(t *testing.T) {
		cases := []struct {
			name  string
			input int
			want  int
		}{
			{name: "zero", input: 0, want: defaultTaskTimeoutSeconds},
			{name: "positive", input: 42, want: 42},
			{name: "negative", input: -1, want: defaultTaskTimeoutSeconds},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				got := normalizedTimeout(tc.input)
				if got != tc.want {
					t.Errorf("normalizedTimeout(%d) = %d, want %d", tc.input, got, tc.want)
				}
			})
		}
	})

	t.Run("clamp job name", func(t *testing.T) {
		longPrompt := "This prompt is definitely longer than forty eight runes for truncation"
		cases := []struct {
			name   string
			prompt string
			want   string
		}{
			{name: "short", prompt: "Short prompt", want: "Short prompt"},
			{name: "long", prompt: longPrompt, want: string([]rune(strings.TrimSpace(longPrompt))[:48]) + "..."},
			{name: "empty", prompt: "   ", want: "Scheduled report"},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				got := clampJobName(tc.prompt)
				if got != tc.want {
					t.Errorf("clampJobName(%q) = %q, want %q", tc.prompt, got, tc.want)
				}
			})
		}
	})

	t.Run("normalize string list", func(t *testing.T) {
		got := normalizeStringList([]string{" alpha ", "Alpha", "", "beta", "  ", "BETA", "gamma"})
		want := []string{"alpha", "beta", "gamma"}
		if !slices.Equal(got, want) {
			t.Errorf("normalizeStringList() = %v, want %v", got, want)
		}
		if normalizeStringList(nil) != nil {
			t.Errorf("normalizeStringList(nil) = non-nil, want nil")
		}
	})

	t.Run("first non empty", func(t *testing.T) {
		got := firstNonEmpty("", "  ", "alpha", "beta")
		if got != "alpha" {
			t.Errorf("firstNonEmpty() = %q, want %q", got, "alpha")
		}
	})

	t.Run("append unique string", func(t *testing.T) {
		got := appendUniqueString([]string{"alpha"}, "beta")
		if !slices.Equal(got, []string{"alpha", "beta"}) {
			t.Errorf("appendUniqueString() = %v, want [alpha beta]", got)
		}
		got = appendUniqueString(got, "beta")
		if !slices.Equal(got, []string{"alpha", "beta"}) {
			t.Errorf("appendUniqueString() duplicate = %v, want [alpha beta]", got)
		}
		got = appendUniqueString(got, "  ")
		if !slices.Equal(got, []string{"alpha", "beta"}) {
			t.Errorf("appendUniqueString() empty = %v, want [alpha beta]", got)
		}
	})

	t.Run("validate schedule spec", func(t *testing.T) {
		validAt := types.ScheduleSpec{
			Kind: types.ScheduleKindAt,
			At:   "2026-04-29T12:00:00Z",
		}
		validEvery := types.ScheduleSpec{
			Kind:         types.ScheduleKindEvery,
			EveryMinutes: 15,
		}
		validCron := types.ScheduleSpec{
			Kind:     types.ScheduleKindCron,
			Expr:     "0 9 * * *",
			Timezone: "UTC",
		}
		cases := []struct {
			name     string
			schedule types.ScheduleSpec
			wantErr  bool
		}{
			{name: "valid at", schedule: validAt},
			{name: "invalid at", schedule: types.ScheduleSpec{Kind: types.ScheduleKindAt}, wantErr: true},
			{name: "valid every", schedule: validEvery},
			{name: "invalid every", schedule: types.ScheduleSpec{Kind: types.ScheduleKindEvery}, wantErr: true},
			{name: "valid cron", schedule: validCron},
			{name: "invalid cron", schedule: types.ScheduleSpec{Kind: types.ScheduleKindCron, Expr: "bad"}, wantErr: true},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				err := validateScheduleSpec(tc.schedule, "job")
				if tc.wantErr && err == nil {
					t.Errorf("error = nil, want error")
				}
				if !tc.wantErr && err != nil {
					t.Errorf("error = %v, want nil", err)
				}
			})
		}
	})

	t.Run("child agent spec from scheduled job", func(t *testing.T) {
		createdAt := time.Date(2026, 4, 29, 12, 0, 0, 0, time.UTC)
		updatedAt := createdAt.Add(time.Minute)
		job := types.ScheduledJob{
			ID:                  "job-1",
			Name:                "Daily digest",
			OwnerSessionID:      "session-1",
			Prompt:              "Prompt",
			ActivatedSkillNames: []string{"plan", "report"},
			ReportGroupID:       "group-1",
			Kind:                types.ScheduleKindCron,
			CronExpr:            "0 9 * * *",
			Timezone:            "UTC",
			CreatedAt:           createdAt,
			UpdatedAt:           updatedAt,
		}
		spec := childAgentSpecFromScheduledJob(job)
		if spec.AgentID != job.ID {
			t.Errorf("AgentID = %q, want %q", spec.AgentID, job.ID)
		}
		if spec.SessionID != job.OwnerSessionID {
			t.Errorf("SessionID = %q, want %q", spec.SessionID, job.OwnerSessionID)
		}
		if spec.Purpose != job.Name {
			t.Errorf("Purpose = %q, want %q", spec.Purpose, job.Name)
		}
		if spec.Mode != types.ChildAgentModeBackgroundWorker {
			t.Errorf("Mode = %q, want %q", spec.Mode, types.ChildAgentModeBackgroundWorker)
		}
		if !slices.Equal(spec.ActivatedSkillNames, job.ActivatedSkillNames) {
			t.Errorf("ActivatedSkillNames = %v, want %v", spec.ActivatedSkillNames, job.ActivatedSkillNames)
		}
		if !slices.Equal(spec.ReportGroups, []string{"group-1"}) {
			t.Errorf("ReportGroups = %v, want [group-1]", spec.ReportGroups)
		}
		if spec.Schedule.Kind != job.Kind {
			t.Errorf("Schedule.Kind = %q, want %q", spec.Schedule.Kind, job.Kind)
		}
		if spec.Schedule.Expr != job.CronExpr {
			t.Errorf("Schedule.Expr = %q, want %q", spec.Schedule.Expr, job.CronExpr)
		}
		if !spec.CreatedAt.Equal(createdAt) {
			t.Errorf("CreatedAt = %v, want %v", spec.CreatedAt, createdAt)
		}
		if !spec.UpdatedAt.Equal(updatedAt) {
			t.Errorf("UpdatedAt = %v, want %v", spec.UpdatedAt, updatedAt)
		}
	})
}
