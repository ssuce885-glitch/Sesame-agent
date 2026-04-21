package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"go-agent/internal/store/sqlite"
	"go-agent/internal/task"
	"go-agent/internal/types"
)

const defaultTaskTimeoutSeconds = 3600

type Store interface {
	UpsertScheduledJob(context.Context, types.ScheduledJob) error
	GetScheduledJob(context.Context, string) (types.ScheduledJob, bool, error)
	ListScheduledJobs(context.Context) ([]types.ScheduledJob, error)
	ListScheduledJobsByWorkspace(context.Context, string) ([]types.ScheduledJob, error)
	ListDueScheduledJobs(context.Context, time.Time) ([]types.ScheduledJob, error)
	DeleteScheduledJob(context.Context, string) (bool, error)
	UpsertChildAgentSpec(context.Context, types.ChildAgentSpec) error
	DeleteChildAgentSpec(context.Context, string) (bool, error)
	GetReportGroup(context.Context, string) (types.ReportGroup, bool, error)
	UpsertReportGroup(context.Context, types.ReportGroup) error
}

type CreateJobInput struct {
	Name                    string
	WorkspaceRoot           string
	OwnerSessionID          string
	Prompt                  string
	ActivatedSkillNames     []string
	ReportGroupID           string
	ReportGroupTitle        string
	ReportGroupRunAt        string
	ReportGroupEveryMinutes int
	ReportGroupCron         string
	ReportGroupTimezone     string
	RunAt                   time.Time
	DelayMinutes            int
	EveryMinutes            int
	CronExpr                string
	Timezone                string
	TimeoutSeconds          int
	SkipIfRunning           *bool
}

type Service struct {
	store        Store
	taskManager  *task.Manager
	now          func() time.Time
	pollInterval time.Duration
	logger       *slog.Logger
}

func NewService(store Store, taskManager *task.Manager) *Service {
	return &Service{
		store:        store,
		taskManager:  taskManager,
		now:          func() time.Time { return time.Now().UTC() },
		pollInterval: time.Second,
		logger:       slog.Default(),
	}
}

func (s *Service) SetClock(now func() time.Time) {
	if now != nil {
		s.now = now
	}
}

func (s *Service) SetPollInterval(interval time.Duration) {
	if interval > 0 {
		s.pollInterval = interval
	}
}

func (s *Service) SetLogger(logger *slog.Logger) {
	if logger != nil {
		s.logger = logger
	}
}

func (s *Service) CreateJob(ctx context.Context, in CreateJobInput) (types.ScheduledJob, error) {
	if s == nil || s.store == nil {
		return types.ScheduledJob{}, fmt.Errorf("scheduler service is not configured")
	}

	now := s.currentTime()
	if err := validateReportGroupScheduleInput(in); err != nil {
		return types.ScheduledJob{}, err
	}
	job, err := buildScheduledJob(now, in)
	if err != nil {
		return types.ScheduledJob{}, err
	}

	if txStore, ok := s.store.(interface {
		WithTx(context.Context, func(sqlite.RuntimeTx) error) error
	}); ok {
		if err := txStore.WithTx(ctx, func(tx sqlite.RuntimeTx) error {
			if err := s.validateReportGroupForJob(ctx, tx, job); err != nil {
				return err
			}
			if err := tx.UpsertScheduledJob(ctx, job); err != nil {
				return err
			}
			if err := tx.UpsertChildAgentSpec(ctx, childAgentSpecFromScheduledJob(job)); err != nil {
				return err
			}
			return s.upsertReportGroupForJob(ctx, tx, job, in, now)
		}); err != nil {
			return types.ScheduledJob{}, err
		}
		return job, nil
	}

	if err := s.validateReportGroupForJob(ctx, s.store, job); err != nil {
		return types.ScheduledJob{}, err
	}
	if err := s.store.UpsertScheduledJob(ctx, job); err != nil {
		return types.ScheduledJob{}, err
	}
	if err := s.store.UpsertChildAgentSpec(ctx, childAgentSpecFromScheduledJob(job)); err != nil {
		return types.ScheduledJob{}, err
	}
	if err := s.upsertReportGroupForJob(ctx, s.store, job, in, now); err != nil {
		return types.ScheduledJob{}, err
	}
	return job, nil
}

func (s *Service) ListJobs(ctx context.Context, workspaceRoot string) ([]types.ScheduledJob, error) {
	if s == nil || s.store == nil {
		return nil, fmt.Errorf("scheduler service is not configured")
	}
	if strings.TrimSpace(workspaceRoot) == "" {
		return s.store.ListScheduledJobs(ctx)
	}
	return s.store.ListScheduledJobsByWorkspace(ctx, workspaceRoot)
}

func (s *Service) GetJob(ctx context.Context, id string) (types.ScheduledJob, bool, error) {
	if s == nil || s.store == nil {
		return types.ScheduledJob{}, false, fmt.Errorf("scheduler service is not configured")
	}
	return s.store.GetScheduledJob(ctx, id)
}

func (s *Service) SetJobEnabled(ctx context.Context, id string, enabled bool) (types.ScheduledJob, bool, error) {
	job, ok, err := s.GetJob(ctx, id)
	if err != nil || !ok {
		return types.ScheduledJob{}, ok, err
	}
	job.Enabled = enabled
	job.UpdatedAt = s.currentTime()
	if enabled {
		next, _, err := initialNextRun(job, s.currentTime())
		if err != nil {
			return types.ScheduledJob{}, false, err
		}
		job.NextRunAt = next
	}
	if err := s.store.UpsertScheduledJob(ctx, job); err != nil {
		return types.ScheduledJob{}, false, err
	}
	return job, true, nil
}

func (s *Service) DeleteJob(ctx context.Context, id string) (bool, error) {
	if s == nil || s.store == nil {
		return false, fmt.Errorf("scheduler service is not configured")
	}

	if txStore, ok := s.store.(interface {
		WithTx(context.Context, func(sqlite.RuntimeTx) error) error
	}); ok {
		deleted := false
		err := txStore.WithTx(ctx, func(tx sqlite.RuntimeTx) error {
			job, ok, err := tx.GetScheduledJob(ctx, id)
			if err != nil {
				return err
			}
			if !ok {
				deleted = false
				return nil
			}
			deleted, err = tx.DeleteScheduledJob(ctx, id)
			if err != nil || !deleted {
				return err
			}
			if _, err := tx.DeleteChildAgentSpec(ctx, id); err != nil {
				return err
			}
			return s.removeJobFromReportGroup(ctx, tx, job)
		})
		return deleted, err
	}

	job, ok, err := s.store.GetScheduledJob(ctx, id)
	if err != nil {
		return false, err
	}
	if !ok {
		return false, nil
	}
	deleted, err := s.store.DeleteScheduledJob(ctx, id)
	if err != nil || !deleted {
		return deleted, err
	}
	if _, err := s.store.DeleteChildAgentSpec(ctx, id); err != nil {
		return true, err
	}
	if err := s.removeJobFromReportGroup(ctx, s.store, job); err != nil {
		return true, err
	}
	return true, nil
}

func (s *Service) RecordTaskTerminal(ctx context.Context, completed task.Task) error {
	if s == nil || s.store == nil {
		return nil
	}
	jobID := strings.TrimSpace(completed.ScheduledJobID)
	if jobID == "" {
		return nil
	}
	job, ok, err := s.store.GetScheduledJob(ctx, jobID)
	if err != nil || !ok {
		return err
	}

	terminalAt := s.currentTime()
	if completed.EndTime != nil && !completed.EndTime.IsZero() {
		terminalAt = completed.EndTime.UTC()
	} else if completed.FinalResultReadyAt != nil && !completed.FinalResultReadyAt.IsZero() {
		terminalAt = completed.FinalResultReadyAt.UTC()
	}

	job.LastRunAt = terminalAt
	job.LastTaskID = strings.TrimSpace(completed.ID)
	job.UpdatedAt = terminalAt

	switch completed.Status {
	case task.TaskStatusCompleted:
		job.LastStatus = types.ScheduledJobStatusSucceeded
		job.SuccessCount++
		job.LastError = ""
	case task.TaskStatusFailed:
		job.LastStatus = types.ScheduledJobStatusFailed
		job.FailCount++
		job.LastError = firstNonEmpty(strings.TrimSpace(completed.Error), "task failed")
	case task.TaskStatusStopped:
		job.LastStatus = types.ScheduledJobStatusFailed
		job.FailCount++
		job.LastError = firstNonEmpty(strings.TrimSpace(completed.Error), "task stopped")
	default:
		return nil
	}

	return s.store.UpsertScheduledJob(ctx, job)
}

type reportGroupStore interface {
	GetReportGroup(context.Context, string) (types.ReportGroup, bool, error)
	UpsertReportGroup(context.Context, types.ReportGroup) error
}
