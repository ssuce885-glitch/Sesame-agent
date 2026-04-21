package scheduler

import (
	"fmt"
	"strings"
	"time"

	"github.com/robfig/cron/v3"

	"go-agent/internal/types"
)

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
