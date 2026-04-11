package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/robfig/cron/v3"

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

func buildScheduledJob(now time.Time, in CreateJobInput) (types.ScheduledJob, error) {
	job := types.ScheduledJob{
		ID:                  types.NewID("cron"),
		Name:                strings.TrimSpace(in.Name),
		WorkspaceRoot:       strings.TrimSpace(in.WorkspaceRoot),
		OwnerSessionID:      strings.TrimSpace(in.OwnerSessionID),
		ActivatedSkillNames: normalizeStringList(in.ActivatedSkillNames),
		ReportGroupID:       strings.TrimSpace(in.ReportGroupID),
		ReportGroupTitle:    strings.TrimSpace(in.ReportGroupTitle),
		Prompt:              strings.TrimSpace(in.Prompt),
		CronExpr:            strings.TrimSpace(in.CronExpr),
		EveryMinutes:        in.EveryMinutes,
		Timezone:            strings.TrimSpace(in.Timezone),
		RunAt:               in.RunAt.UTC(),
		Enabled:             true,
		SkipIfRunning:       true,
		TimeoutSeconds:      normalizedTimeout(in.TimeoutSeconds),
		LastStatus:          types.ScheduledJobStatusPending,
		CreatedAt:           now,
		UpdatedAt:           now,
	}
	if in.SkipIfRunning != nil {
		job.SkipIfRunning = *in.SkipIfRunning
	}
	if strings.TrimSpace(job.WorkspaceRoot) == "" {
		return types.ScheduledJob{}, fmt.Errorf("workspace_root is required")
	}
	if strings.TrimSpace(job.OwnerSessionID) == "" {
		return types.ScheduledJob{}, fmt.Errorf("owner_session_id is required")
	}
	if strings.TrimSpace(job.Prompt) == "" {
		return types.ScheduledJob{}, fmt.Errorf("prompt is required")
	}
	if strings.TrimSpace(job.Name) == "" {
		job.Name = clampJobName(job.Prompt)
	}
	if job.Timezone == "" {
		job.Timezone = "UTC"
	}
	scheduleSelectionCount := 0
	if in.DelayMinutes > 0 {
		scheduleSelectionCount++
	}
	if !in.RunAt.IsZero() {
		scheduleSelectionCount++
	}
	if in.EveryMinutes > 0 {
		scheduleSelectionCount++
	}
	if strings.TrimSpace(in.CronExpr) != "" {
		scheduleSelectionCount++
	}
	if scheduleSelectionCount == 0 {
		return types.ScheduledJob{}, fmt.Errorf("one of delay_minutes, run_at, every_minutes, or cron is required")
	}
	if scheduleSelectionCount > 1 {
		return types.ScheduledJob{}, fmt.Errorf("exactly one schedule selector is allowed")
	}
	if in.DelayMinutes > 0 {
		job.Kind = types.ScheduleKindAt
		job.RunAt = now.Add(time.Duration(in.DelayMinutes) * time.Minute).UTC()
	}
	switch {
	case job.CronExpr != "":
		job.Kind = types.ScheduleKindCron
	case job.EveryMinutes > 0:
		job.Kind = types.ScheduleKindEvery
	case !job.RunAt.IsZero():
		job.Kind = types.ScheduleKindAt
	}
	if job.Kind == "" {
		return types.ScheduledJob{}, fmt.Errorf("one of delay_minutes, run_at, every_minutes, or cron is required")
	}
	if job.Kind == types.ScheduleKindAt && !job.RunAt.IsZero() && job.RunAt.Before(now) {
		return types.ScheduledJob{}, fmt.Errorf("run_at must not be in the past")
	}
	nextRun, enabled, err := initialNextRun(job, now)
	if err != nil {
		return types.ScheduledJob{}, err
	}
	job.NextRunAt = nextRun
	job.Enabled = enabled
	return job, nil
}

func initialNextRun(job types.ScheduledJob, now time.Time) (time.Time, bool, error) {
	now = now.UTC()
	switch job.Kind {
	case types.ScheduleKindAt:
		runAt := job.RunAt.UTC()
		if runAt.IsZero() {
			return time.Time{}, false, fmt.Errorf("run_at is required for at jobs")
		}
		return runAt, true, nil
	case types.ScheduleKindEvery:
		if job.EveryMinutes <= 0 {
			return time.Time{}, false, fmt.Errorf("every_minutes must be greater than zero")
		}
		return now.Add(time.Duration(job.EveryMinutes) * time.Minute), true, nil
	case types.ScheduleKindCron:
		if strings.TrimSpace(job.CronExpr) == "" {
			return time.Time{}, false, fmt.Errorf("cron expression is required")
		}
		loc, err := time.LoadLocation(defaultTimezone(job.Timezone))
		if err != nil {
			return time.Time{}, false, err
		}
		schedule, err := cron.ParseStandard(job.CronExpr)
		if err != nil {
			return time.Time{}, false, err
		}
		return schedule.Next(now.In(loc)).UTC(), true, nil
	default:
		return time.Time{}, false, fmt.Errorf("unsupported schedule kind %q", job.Kind)
	}
}

func nextRunAfterDispatch(job types.ScheduledJob, now time.Time) (time.Time, bool, error) {
	now = now.UTC()
	switch job.Kind {
	case types.ScheduleKindAt:
		if job.RunAt.IsZero() {
			return time.Time{}, false, fmt.Errorf("run_at is required for at jobs")
		}
		return time.Time{}, false, nil
	case types.ScheduleKindEvery:
		if job.EveryMinutes <= 0 {
			return time.Time{}, false, fmt.Errorf("every_minutes must be greater than zero")
		}
		base := job.NextRunAt.UTC()
		if base.IsZero() {
			base = now
		}
		next := base.Add(time.Duration(job.EveryMinutes) * time.Minute)
		for !next.After(now) {
			next = next.Add(time.Duration(job.EveryMinutes) * time.Minute)
		}
		return next, true, nil
	case types.ScheduleKindCron:
		if strings.TrimSpace(job.CronExpr) == "" {
			return time.Time{}, false, fmt.Errorf("cron expression is required")
		}
		loc, err := time.LoadLocation(defaultTimezone(job.Timezone))
		if err != nil {
			return time.Time{}, false, err
		}
		schedule, err := cron.ParseStandard(job.CronExpr)
		if err != nil {
			return time.Time{}, false, err
		}
		base := now.In(loc)
		if !job.NextRunAt.IsZero() {
			base = job.NextRunAt.In(loc)
		}
		next := schedule.Next(base)
		for !next.After(now.In(loc)) {
			next = schedule.Next(next)
		}
		return next.UTC(), true, nil
	default:
		return time.Time{}, false, fmt.Errorf("unsupported schedule kind %q", job.Kind)
	}
}

func normalizedTimeout(timeoutSeconds int) int {
	if timeoutSeconds <= 0 {
		return defaultTaskTimeoutSeconds
	}
	return timeoutSeconds
}

func clampJobName(prompt string) string {
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return "Scheduled report"
	}
	runes := []rune(prompt)
	if len(runes) <= 48 {
		return prompt
	}
	return string(runes[:48]) + "..."
}

func defaultTimezone(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "UTC"
	}
	return value
}

func scheduledTaskKind(job types.ScheduledJob) string {
	switch job.Kind {
	case types.ScheduleKindCron, types.ScheduleKindEvery:
		return "scheduled_report"
	default:
		return "report"
	}
}

func childAgentSpecFromScheduledJob(job types.ScheduledJob) types.ChildAgentSpec {
	reportGroups := []string{}
	if strings.TrimSpace(job.ReportGroupID) != "" {
		reportGroups = append(reportGroups, strings.TrimSpace(job.ReportGroupID))
	}
	return types.ChildAgentSpec{
		AgentID:             strings.TrimSpace(job.ID),
		SessionID:           strings.TrimSpace(job.OwnerSessionID),
		Purpose:             firstNonEmpty(job.Name, job.Prompt),
		Mode:                types.ChildAgentModeBackgroundWorker,
		ActivatedSkillNames: append([]string(nil), job.ActivatedSkillNames...),
		ReportGroups:        reportGroups,
		Schedule:            scheduleSpecFromScheduledJob(job),
		CreatedAt:           job.CreatedAt,
		UpdatedAt:           job.UpdatedAt,
	}
}

func scheduleSpecFromScheduledJob(job types.ScheduledJob) types.ScheduleSpec {
	spec := types.ScheduleSpec{
		Kind:         job.Kind,
		EveryMinutes: job.EveryMinutes,
		Expr:         job.CronExpr,
		Timezone:     job.Timezone,
	}
	if job.Kind == types.ScheduleKindAt && !job.RunAt.IsZero() {
		spec.At = job.RunAt.UTC().Format(time.RFC3339)
	}
	return spec
}

func normalizeStringList(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		key := strings.ToLower(trimmed)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, trimmed)
	}
	return out
}

func validateReportGroupScheduleInput(in CreateJobInput) error {
	groupID := strings.TrimSpace(in.ReportGroupID)
	selectorCount := 0
	if strings.TrimSpace(in.ReportGroupRunAt) != "" {
		selectorCount++
	}
	if in.ReportGroupEveryMinutes > 0 {
		selectorCount++
	}
	if strings.TrimSpace(in.ReportGroupCron) != "" {
		selectorCount++
	}
	if selectorCount == 0 {
		return nil
	}
	schedule, configured := reportGroupScheduleConfigured(in)
	if !configured {
		return nil
	}
	if groupID == "" {
		return fmt.Errorf("report_group_id is required when report group schedule is configured")
	}
	if selectorCount > 1 {
		return fmt.Errorf("exactly one report group schedule selector is allowed")
	}
	return validateScheduleSpec(schedule, "report_group")
}

func reportGroupScheduleConfigured(in CreateJobInput) (types.ScheduleSpec, bool) {
	schedule := reportGroupScheduleFromCreateInput(in)
	return schedule, schedule.Kind != ""
}

func reportGroupScheduleFromCreateInput(in CreateJobInput) types.ScheduleSpec {
	switch {
	case strings.TrimSpace(in.ReportGroupCron) != "":
		return types.ScheduleSpec{
			Kind:     types.ScheduleKindCron,
			Expr:     strings.TrimSpace(in.ReportGroupCron),
			Timezone: strings.TrimSpace(in.ReportGroupTimezone),
		}
	case in.ReportGroupEveryMinutes > 0:
		return types.ScheduleSpec{
			Kind:         types.ScheduleKindEvery,
			EveryMinutes: in.ReportGroupEveryMinutes,
		}
	case strings.TrimSpace(in.ReportGroupRunAt) != "":
		return types.ScheduleSpec{
			Kind: types.ScheduleKindAt,
			At:   strings.TrimSpace(in.ReportGroupRunAt),
		}
	default:
		return types.ScheduleSpec{}
	}
}

type reportGroupStore interface {
	GetReportGroup(context.Context, string) (types.ReportGroup, bool, error)
	UpsertReportGroup(context.Context, types.ReportGroup) error
}

func (s *Service) validateReportGroupForJob(ctx context.Context, store reportGroupStore, job types.ScheduledJob) error {
	groupID := strings.TrimSpace(job.ReportGroupID)
	if groupID == "" || store == nil {
		return nil
	}
	group, ok, err := store.GetReportGroup(ctx, groupID)
	if err != nil || !ok {
		return err
	}
	if strings.TrimSpace(group.SessionID) != "" && strings.TrimSpace(group.SessionID) != strings.TrimSpace(job.OwnerSessionID) {
		return fmt.Errorf("report group %q already belongs to another session", groupID)
	}
	return nil
}

func (s *Service) upsertReportGroupForJob(ctx context.Context, store reportGroupStore, job types.ScheduledJob, in CreateJobInput, now time.Time) error {
	groupID := strings.TrimSpace(job.ReportGroupID)
	if groupID == "" || store == nil {
		return nil
	}
	group, ok, err := store.GetReportGroup(ctx, groupID)
	if err != nil {
		return err
	}
	if !ok {
		group = types.ReportGroup{
			GroupID:   groupID,
			SessionID: strings.TrimSpace(job.OwnerSessionID),
			Title:     firstNonEmpty(strings.TrimSpace(job.ReportGroupTitle), strings.TrimSpace(job.Name), groupID),
			Sources:   []string{strings.TrimSpace(job.ID)},
			Schedule:  reportGroupScheduleFromCreateInput(in),
			Delivery: types.DeliveryProfile{
				Channels: []string{string(types.ReportChannelMailbox)},
			},
			CreatedAt: now,
			UpdatedAt: now,
		}
		return store.UpsertReportGroup(ctx, group)
	}
	if strings.TrimSpace(group.SessionID) != "" && strings.TrimSpace(group.SessionID) != strings.TrimSpace(job.OwnerSessionID) {
		return fmt.Errorf("report group %q already belongs to another session", groupID)
	}
	group.SessionID = firstNonEmpty(strings.TrimSpace(group.SessionID), strings.TrimSpace(job.OwnerSessionID))
	group.Title = firstNonEmpty(strings.TrimSpace(group.Title), strings.TrimSpace(job.ReportGroupTitle), strings.TrimSpace(job.Name), groupID)
	group.Sources = appendUniqueString(group.Sources, strings.TrimSpace(job.ID))
	if schedule, ok := reportGroupScheduleConfigured(in); ok {
		group.Schedule = schedule
	}
	group.UpdatedAt = now
	if len(group.Delivery.Channels) == 0 {
		group.Delivery.Channels = []string{string(types.ReportChannelMailbox)}
	}
	return store.UpsertReportGroup(ctx, group)
}

func (s *Service) removeJobFromReportGroup(ctx context.Context, store reportGroupStore, job types.ScheduledJob) error {
	groupID := strings.TrimSpace(job.ReportGroupID)
	if groupID == "" || store == nil {
		return nil
	}
	group, ok, err := store.GetReportGroup(ctx, groupID)
	if err != nil || !ok {
		return err
	}
	filtered := make([]string, 0, len(group.Sources))
	for _, source := range group.Sources {
		if strings.TrimSpace(source) != strings.TrimSpace(job.ID) {
			filtered = append(filtered, source)
		}
	}
	group.Sources = filtered
	group.UpdatedAt = s.currentTime()
	return store.UpsertReportGroup(ctx, group)
}

func validateScheduleSpec(schedule types.ScheduleSpec, label string) error {
	label = firstNonEmpty(strings.TrimSpace(label), "schedule")
	switch schedule.Kind {
	case "":
		return nil
	case types.ScheduleKindAt:
		if strings.TrimSpace(schedule.At) == "" {
			return fmt.Errorf("%s run_at must be RFC3339", label)
		}
		if _, err := time.Parse(time.RFC3339, strings.TrimSpace(schedule.At)); err != nil {
			return fmt.Errorf("%s run_at must be RFC3339: %w", label, err)
		}
		return nil
	case types.ScheduleKindEvery:
		if schedule.EveryMinutes <= 0 {
			return fmt.Errorf("%s every_minutes must be greater than zero", label)
		}
		return nil
	case types.ScheduleKindCron:
		if strings.TrimSpace(schedule.Expr) == "" {
			return fmt.Errorf("%s cron expression is required", label)
		}
		if _, err := time.LoadLocation(defaultTimezone(schedule.Timezone)); err != nil {
			return fmt.Errorf("%s timezone is invalid: %w", label, err)
		}
		if _, err := cron.ParseStandard(strings.TrimSpace(schedule.Expr)); err != nil {
			return fmt.Errorf("%s cron expression is invalid: %w", label, err)
		}
		return nil
	default:
		return fmt.Errorf("%s kind %q is unsupported", label, schedule.Kind)
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func appendUniqueString(values []string, value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return values
	}
	for _, existing := range values {
		if strings.TrimSpace(existing) == value {
			return values
		}
	}
	return append(values, value)
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
