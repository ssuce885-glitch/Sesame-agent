package types

import "time"

type ScheduledJobStatus string

const (
	ScheduledJobStatusPending   ScheduledJobStatus = "pending"
	ScheduledJobStatusRunning   ScheduledJobStatus = "running"
	ScheduledJobStatusSucceeded ScheduledJobStatus = "succeeded"
	ScheduledJobStatusFailed    ScheduledJobStatus = "failed"
	ScheduledJobStatusSkipped   ScheduledJobStatus = "skipped"
)

type ScheduledJob struct {
	ID                  string             `json:"id"`
	Name                string             `json:"name"`
	WorkspaceRoot       string             `json:"workspace_root"`
	OwnerSessionID      string             `json:"owner_session_id"`
	Kind                ScheduleKind       `json:"kind"`
	Prompt              string             `json:"prompt"`
	ActivatedSkillNames []string           `json:"activated_skill_names,omitempty"`
	PermissionProfile   string             `json:"permission_profile,omitempty"`
	CronExpr            string             `json:"cron_expr,omitempty"`
	EveryMinutes        int                `json:"every_minutes,omitempty"`
	Timezone            string             `json:"timezone,omitempty"`
	RunAt               time.Time          `json:"run_at,omitempty"`
	Enabled             bool               `json:"enabled"`
	SkipIfRunning       bool               `json:"skip_if_running"`
	TimeoutSeconds      int                `json:"timeout_seconds,omitempty"`
	NextRunAt           time.Time          `json:"next_run_at,omitempty"`
	LastRunAt           time.Time          `json:"last_run_at,omitempty"`
	LastStatus          ScheduledJobStatus `json:"last_status,omitempty"`
	LastError           string             `json:"last_error,omitempty"`
	LastSkipAt          time.Time          `json:"last_skip_at,omitempty"`
	LastTaskID          string             `json:"last_task_id,omitempty"`
	TotalRuns           int                `json:"total_runs,omitempty"`
	SuccessCount        int                `json:"success_count,omitempty"`
	FailCount           int                `json:"fail_count,omitempty"`
	SkipCount           int                `json:"skip_count,omitempty"`
	CreatedAt           time.Time          `json:"created_at,omitempty"`
	UpdatedAt           time.Time          `json:"updated_at,omitempty"`
}

type ListScheduledJobsResponse struct {
	Jobs []ScheduledJob `json:"jobs"`
}
