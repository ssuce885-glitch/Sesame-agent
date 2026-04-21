package scheduler

import (
	"context"
	"strings"
	"time"

	"go-agent/internal/task"
	"go-agent/internal/types"
)

func (s *Service) Run(ctx context.Context) error {
	ticker := time.NewTicker(s.pollInterval)
	defer ticker.Stop()

	if err := s.Tick(ctx); err != nil {
		s.log("scheduler initial tick failed", "error", err)
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := s.Tick(ctx); err != nil {
				s.log("scheduler tick failed", "error", err)
			}
		}
	}
}

func (s *Service) Tick(ctx context.Context) error {
	if s == nil || s.store == nil || s.taskManager == nil {
		return nil
	}
	now := s.currentTime()
	jobs, err := s.store.ListDueScheduledJobs(ctx, now)
	if err != nil {
		return err
	}
	for _, job := range jobs {
		if err := s.handleDueJob(ctx, job, now); err != nil {
			s.log("scheduled job dispatch failed", "job_id", job.ID, "error", err)
		}
	}
	return nil
}

func (s *Service) handleDueJob(ctx context.Context, job types.ScheduledJob, now time.Time) error {
	if strings.TrimSpace(job.ID) == "" || !job.Enabled || job.NextRunAt.IsZero() || job.NextRunAt.After(now) {
		return nil
	}

	active, err := s.jobHasActiveTask(job)
	if err != nil {
		return err
	}
	if active {
		if !job.SkipIfRunning {
			return nil
		}
		nextRun, enabled, err := nextRunAfterDispatch(job, now)
		if err != nil {
			return err
		}
		job.NextRunAt = nextRun
		job.Enabled = enabled
		job.LastStatus = types.ScheduledJobStatusSkipped
		job.LastSkipAt = now
		job.SkipCount++
		job.LastError = "previous run still active"
		job.UpdatedAt = now
		return s.store.UpsertScheduledJob(ctx, job)
	}

	nextRun, enabled, err := nextRunAfterDispatch(job, now)
	if err != nil {
		return err
	}

	created, err := s.taskManager.Create(ctx, task.CreateTaskInput{
		Type:                task.TaskTypeAgent,
		Command:             job.Prompt,
		Description:         firstNonEmpty(job.Name, job.Prompt),
		Owner:               "scheduled_job",
		Kind:                scheduledTaskKind(job),
		ScheduledJobID:      job.ID,
		ActivatedSkillNames: append([]string(nil), job.ActivatedSkillNames...),
		ParentSessionID:     job.OwnerSessionID,
		WorkspaceRoot:       job.WorkspaceRoot,
		TimeoutSeconds:      normalizedTimeout(job.TimeoutSeconds),
		Start:               true,
	})
	if err != nil {
		job.NextRunAt = nextRun
		job.Enabled = enabled
		job.LastStatus = types.ScheduledJobStatusFailed
		job.LastError = err.Error()
		job.FailCount++
		job.UpdatedAt = now
		return s.store.UpsertScheduledJob(ctx, job)
	}

	job.NextRunAt = nextRun
	job.Enabled = enabled
	job.LastStatus = types.ScheduledJobStatusRunning
	job.LastTaskID = created.ID
	job.LastError = ""
	job.TotalRuns++
	job.UpdatedAt = now
	return s.store.UpsertScheduledJob(ctx, job)
}

func (s *Service) jobHasActiveTask(job types.ScheduledJob) (bool, error) {
	taskID := strings.TrimSpace(job.LastTaskID)
	if taskID == "" {
		return false, nil
	}
	current, ok, err := s.taskManager.Get(taskID, job.WorkspaceRoot)
	if err != nil {
		return false, err
	}
	if !ok {
		return false, nil
	}
	switch current.Status {
	case task.TaskStatusPending, task.TaskStatusRunning:
		return true, nil
	default:
		return false, nil
	}
}

func (s *Service) currentTime() time.Time {
	if s != nil && s.now != nil {
		return s.now().UTC()
	}
	return time.Now().UTC()
}

func (s *Service) log(msg string, args ...any) {
	if s != nil && s.logger != nil {
		s.logger.Info(msg, args...)
	}
}
