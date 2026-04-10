package render

import (
	"fmt"
	"strings"
	"time"

	"go-agent/internal/types"
)

func (r *Renderer) RenderCronList(resp types.ListScheduledJobsResponse) {
	if len(resp.Jobs) == 0 {
		fmt.Fprintln(r.out, "No cron jobs.")
		return
	}
	for _, job := range resp.Jobs {
		fmt.Fprintf(r.out, "◌ %s\n", FormatScheduledJobLine(job))
	}
}

func (r *Renderer) RenderCronJob(job types.ScheduledJob) {
	title := firstNonEmpty(job.Name, job.ID)
	r.renderDetail("Cron", title, FormatScheduledJobDetail(job))
}

func FormatScheduledJobLine(job types.ScheduledJob) string {
	parts := []string{firstNonEmpty(job.ID, job.Name)}
	if name := strings.TrimSpace(job.Name); name != "" && name != job.ID {
		parts = append(parts, name)
	}
	if schedule := scheduledJobScheduleLabel(job); schedule != "" {
		parts = append(parts, schedule)
	}
	parts = append(parts, scheduledJobDisplayStatus(job))
	if !job.NextRunAt.IsZero() {
		parts = append(parts, "next "+job.NextRunAt.Format(time.RFC3339))
	}
	return strings.Join(parts, "  ·  ")
}

func FormatScheduledJobDetail(job types.ScheduledJob) string {
	lines := []string{
		"status: " + scheduledJobDisplayStatus(job),
	}
	if schedule := scheduledJobScheduleLabel(job); schedule != "" {
		lines = append(lines, "schedule: "+schedule)
	}
	if strings.TrimSpace(job.Prompt) != "" {
		lines = append(lines, "prompt: "+trimPreview(job.Prompt))
	}
	if !job.NextRunAt.IsZero() {
		lines = append(lines, "next run: "+job.NextRunAt.Format(time.RFC3339))
	}
	if !job.LastRunAt.IsZero() {
		lines = append(lines, "last run: "+job.LastRunAt.Format(time.RFC3339))
	}
	if strings.TrimSpace(job.LastTaskID) != "" {
		lines = append(lines, "last task: "+job.LastTaskID)
	}
	lines = append(lines, fmt.Sprintf("runs: %d success / %d fail / %d skip / %d total", job.SuccessCount, job.FailCount, job.SkipCount, job.TotalRuns))
	if strings.TrimSpace(job.LastError) != "" {
		lines = append(lines, "last error: "+job.LastError)
	}
	return strings.Join(lines, "\n")
}

func scheduledJobDisplayStatus(job types.ScheduledJob) string {
	if !job.Enabled {
		return "paused"
	}
	switch job.LastStatus {
	case types.ScheduledJobStatusRunning:
		return "running"
	case types.ScheduledJobStatusSucceeded:
		return "succeeded"
	case types.ScheduledJobStatusFailed:
		return "failed"
	case types.ScheduledJobStatusSkipped:
		return "skipped"
	default:
		return "pending"
	}
}

func scheduledJobScheduleLabel(job types.ScheduledJob) string {
	switch job.Kind {
	case types.ScheduleKindAt:
		if !job.RunAt.IsZero() {
			return "at " + job.RunAt.Format(time.RFC3339)
		}
		return "at"
	case types.ScheduleKindEvery:
		if job.EveryMinutes > 0 {
			return fmt.Sprintf("every %d min", job.EveryMinutes)
		}
		return "every"
	case types.ScheduleKindCron:
		if strings.TrimSpace(job.CronExpr) == "" {
			return "cron"
		}
		if tz := strings.TrimSpace(job.Timezone); tz != "" {
			return fmt.Sprintf("cron %s (%s)", job.CronExpr, tz)
		}
		return "cron " + job.CronExpr
	default:
		return ""
	}
}
