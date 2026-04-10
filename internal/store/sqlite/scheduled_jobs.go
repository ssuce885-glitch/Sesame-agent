package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"

	"go-agent/internal/types"
)

func (s *Store) UpsertScheduledJob(ctx context.Context, job types.ScheduledJob) error {
	job = normalizeScheduledJob(job)
	payload, err := json.Marshal(job)
	if err != nil {
		return err
	}

	_, err = s.db.ExecContext(ctx, `
		insert into scheduled_jobs (
			id, workspace_root, owner_session_id, kind, enabled, next_run_at, last_status, payload, created_at, updated_at
		)
		values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		on conflict(id) do update set
			workspace_root = excluded.workspace_root,
			owner_session_id = excluded.owner_session_id,
			kind = excluded.kind,
			enabled = excluded.enabled,
			next_run_at = excluded.next_run_at,
			last_status = excluded.last_status,
			payload = excluded.payload,
			updated_at = excluded.updated_at
	`,
		job.ID,
		job.WorkspaceRoot,
		job.OwnerSessionID,
		job.Kind,
		boolToInt(job.Enabled),
		formatPendingOptionalTime(job.NextRunAt),
		job.LastStatus,
		string(payload),
		job.CreatedAt.Format(timeLayout),
		job.UpdatedAt.Format(timeLayout),
	)
	return err
}

func (s *Store) GetScheduledJob(ctx context.Context, id string) (types.ScheduledJob, bool, error) {
	rows, err := s.db.QueryContext(ctx, `
		select payload, created_at, updated_at
		from scheduled_jobs
		where id = ?
	`, strings.TrimSpace(id))
	if err != nil {
		return types.ScheduledJob{}, false, err
	}
	defer rows.Close()

	jobs, err := scanScheduledJobs(rows)
	if err != nil {
		return types.ScheduledJob{}, false, err
	}
	if len(jobs) == 0 {
		return types.ScheduledJob{}, false, nil
	}
	return jobs[0], true, nil
}

func (s *Store) ListScheduledJobs(ctx context.Context) ([]types.ScheduledJob, error) {
	rows, err := s.db.QueryContext(ctx, `
		select payload, created_at, updated_at
		from scheduled_jobs
		order by updated_at desc, created_at desc, id asc
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanScheduledJobs(rows)
}

func (s *Store) ListScheduledJobsByWorkspace(ctx context.Context, workspaceRoot string) ([]types.ScheduledJob, error) {
	rows, err := s.db.QueryContext(ctx, `
		select payload, created_at, updated_at
		from scheduled_jobs
		where workspace_root = ?
		order by updated_at desc, created_at desc, id asc
	`, strings.TrimSpace(workspaceRoot))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanScheduledJobs(rows)
}

func (s *Store) ListDueScheduledJobs(ctx context.Context, now time.Time) ([]types.ScheduledJob, error) {
	rows, err := s.db.QueryContext(ctx, `
		select payload, created_at, updated_at
		from scheduled_jobs
		where enabled = 1 and next_run_at != '' and next_run_at <= ?
		order by next_run_at asc, created_at asc, id asc
	`, now.UTC().Format(timeLayout))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanScheduledJobs(rows)
}

func (s *Store) DeleteScheduledJob(ctx context.Context, id string) (bool, error) {
	result, err := s.db.ExecContext(ctx, `
		delete from scheduled_jobs
		where id = ?
	`, strings.TrimSpace(id))
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return affected > 0, nil
}

func scanScheduledJobs(rows *sql.Rows) ([]types.ScheduledJob, error) {
	out := make([]types.ScheduledJob, 0)
	for rows.Next() {
		var (
			payload   string
			createdAt string
			updatedAt string
		)
		if err := rows.Scan(&payload, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		var job types.ScheduledJob
		if err := json.Unmarshal([]byte(payload), &job); err != nil {
			return nil, err
		}
		if parsed, err := parsePendingOptionalTime(createdAt); err == nil {
			job.CreatedAt = parsed
		}
		if parsed, err := parsePendingOptionalTime(updatedAt); err == nil {
			job.UpdatedAt = parsed
		}
		out = append(out, normalizeScheduledJob(job))
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func normalizeScheduledJob(job types.ScheduledJob) types.ScheduledJob {
	now := time.Now().UTC()
	job.ID = strings.TrimSpace(job.ID)
	if job.ID == "" {
		job.ID = types.NewID("cron")
	}
	job.Name = strings.TrimSpace(job.Name)
	job.WorkspaceRoot = strings.TrimSpace(job.WorkspaceRoot)
	job.OwnerSessionID = strings.TrimSpace(job.OwnerSessionID)
	job.Kind = types.ScheduleKind(strings.TrimSpace(string(job.Kind)))
	job.Prompt = strings.TrimSpace(job.Prompt)
	job.CronExpr = strings.TrimSpace(job.CronExpr)
	job.Timezone = strings.TrimSpace(job.Timezone)
	job.LastError = strings.TrimSpace(job.LastError)
	job.LastTaskID = strings.TrimSpace(job.LastTaskID)
	if job.CreatedAt.IsZero() {
		job.CreatedAt = now
	} else {
		job.CreatedAt = job.CreatedAt.UTC()
	}
	if job.UpdatedAt.IsZero() {
		job.UpdatedAt = job.CreatedAt
	} else {
		job.UpdatedAt = job.UpdatedAt.UTC()
	}
	if !job.RunAt.IsZero() {
		job.RunAt = job.RunAt.UTC()
	}
	if !job.NextRunAt.IsZero() {
		job.NextRunAt = job.NextRunAt.UTC()
	}
	if !job.LastRunAt.IsZero() {
		job.LastRunAt = job.LastRunAt.UTC()
	}
	if !job.LastSkipAt.IsZero() {
		job.LastSkipAt = job.LastSkipAt.UTC()
	}
	if job.TimeoutSeconds < 0 {
		job.TimeoutSeconds = 0
	}
	return job
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
