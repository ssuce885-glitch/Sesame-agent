package tools

import (
	"context"
	"reflect"
	"testing"
	"time"

	"go-agent/internal/permissions"
	"go-agent/internal/runtimegraph"
	"go-agent/internal/scheduler"
	"go-agent/internal/types"
)

type captureScheduledJobStore struct {
	jobs []types.ScheduledJob
}

func (s *captureScheduledJobStore) UpsertScheduledJob(_ context.Context, job types.ScheduledJob) error {
	s.jobs = append(s.jobs, job)
	return nil
}

func (s *captureScheduledJobStore) GetScheduledJob(context.Context, string) (types.ScheduledJob, bool, error) {
	return types.ScheduledJob{}, false, nil
}

func (s *captureScheduledJobStore) ListScheduledJobs(context.Context) ([]types.ScheduledJob, error) {
	return nil, nil
}

func (s *captureScheduledJobStore) ListScheduledJobsByWorkspace(context.Context, string) ([]types.ScheduledJob, error) {
	return nil, nil
}

func (s *captureScheduledJobStore) ListDueScheduledJobs(context.Context, time.Time) ([]types.ScheduledJob, error) {
	return nil, nil
}

func (s *captureScheduledJobStore) DeleteScheduledJob(context.Context, string) (bool, error) {
	return false, nil
}

var _ scheduler.Store = (*captureScheduledJobStore)(nil)

func TestScheduleReportStoresExplicitActiveSkillNames(t *testing.T) {
	store := &captureScheduledJobStore{}
	service := scheduler.NewService(store, nil)
	service.SetClock(func() time.Time {
		return time.Date(2026, time.April, 11, 10, 0, 0, 0, time.UTC)
	})

	tool := scheduleReportTool{}
	decoded, err := tool.Decode(Call{
		Name: "schedule_report",
		Input: map[string]any{
			"prompt":        "child prompt mentions $brainstorming but should not be scanned",
			"delay_minutes": 15,
		},
	})
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}

	_, err = tool.ExecuteDecoded(context.Background(), decoded, ExecContext{
		WorkspaceRoot:    t.TempDir(),
		SchedulerService: service,
		ActiveSkillNames: []string{
			"brainstorming",
			"",
			"writing-plans",
			"brainstorming",
		},
		TurnContext: &runtimegraph.TurnContext{
			CurrentSessionID: "session-schedule-report",
		},
	})
	if err != nil {
		t.Fatalf("ExecuteDecoded() error = %v", err)
	}

	if len(store.jobs) != 1 {
		t.Fatalf("len(store.jobs) = %d, want 1", len(store.jobs))
	}

	want := []string{"brainstorming", "writing-plans"}
	if !reflect.DeepEqual(store.jobs[0].ActivatedSkillNames, want) {
		t.Fatalf("job.ActivatedSkillNames = %v, want %v", store.jobs[0].ActivatedSkillNames, want)
	}
}

func TestScheduleReportStoresExplicitPermissionProfile(t *testing.T) {
	store := &captureScheduledJobStore{}
	service := scheduler.NewService(store, nil)
	service.SetClock(func() time.Time {
		return time.Date(2026, time.April, 11, 10, 0, 0, 0, time.UTC)
	})

	tool := scheduleReportTool{}
	decoded, err := tool.Decode(Call{
		Name: "schedule_report",
		Input: map[string]any{
			"prompt":        "child prompt",
			"delay_minutes": 15,
		},
	})
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}

	_, err = tool.ExecuteDecoded(context.Background(), decoded, ExecContext{
		WorkspaceRoot:    t.TempDir(),
		SchedulerService: service,
		PermissionEngine: permissions.NewEngine("trusted_local"),
		TurnContext: &runtimegraph.TurnContext{
			CurrentSessionID: "session-schedule-report",
		},
	})
	if err != nil {
		t.Fatalf("ExecuteDecoded() error = %v", err)
	}

	if len(store.jobs) != 1 {
		t.Fatalf("len(store.jobs) = %d, want 1", len(store.jobs))
	}
	if got, want := store.jobs[0].PermissionProfile, "trusted_local"; got != want {
		t.Fatalf("job.PermissionProfile = %q, want %q", got, want)
	}
}
