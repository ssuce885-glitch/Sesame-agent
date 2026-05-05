package store

import (
	"context"
	"database/sql"
	"go-agent/internal/v2/contracts"
)

type automationRepo struct {
	db *sql.DB
	tx *sql.Tx
}

var _ contracts.AutomationRepository = (*automationRepo)(nil)

func (r *automationRepo) execer() execer { return repoExec(r.db, r.tx) }

func (r *automationRepo) Create(ctx context.Context, a contracts.Automation) error {
	_, err := r.execer().Exec(`
INSERT INTO v2_automations (id, workspace_root, title, goal, state, owner, workflow_id, watcher_path, watcher_cron, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		a.ID, a.WorkspaceRoot, a.Title, a.Goal, a.State, a.Owner, a.WorkflowID, a.WatcherPath, a.WatcherCron, timeString(a.CreatedAt), timeString(a.UpdatedAt))
	return err
}

func (r *automationRepo) Get(ctx context.Context, id string) (contracts.Automation, error) {
	return scanAutomation(r.execer().QueryRow(`
SELECT id, workspace_root, title, goal, state, owner, workflow_id, watcher_path, watcher_cron, created_at, updated_at
FROM v2_automations WHERE id = ?`, id))
}

func (r *automationRepo) Update(ctx context.Context, a contracts.Automation) error {
	_, err := r.execer().Exec(`
UPDATE v2_automations
SET workspace_root = ?, title = ?, goal = ?, state = ?, owner = ?, workflow_id = ?, watcher_path = ?, watcher_cron = ?, updated_at = ?
WHERE id = ?`,
		a.WorkspaceRoot, a.Title, a.Goal, a.State, a.Owner, a.WorkflowID, a.WatcherPath, a.WatcherCron, timeString(a.UpdatedAt), a.ID)
	return err
}

func (r *automationRepo) ListByWorkspace(ctx context.Context, workspaceRoot string) ([]contracts.Automation, error) {
	if workspaceRoot == "" {
		rows, err := r.execer().Query(`
SELECT id, workspace_root, title, goal, state, owner, workflow_id, watcher_path, watcher_cron, created_at, updated_at
FROM v2_automations ORDER BY created_at ASC`)
		if err != nil {
			return nil, err
		}
		defer rows.Close()

		var automations []contracts.Automation
		for rows.Next() {
			a, err := scanAutomation(rows)
			if err != nil {
				return nil, err
			}
			automations = append(automations, a)
		}
		return automations, rows.Err()
	}
	rows, err := r.execer().Query(`
SELECT id, workspace_root, title, goal, state, owner, workflow_id, watcher_path, watcher_cron, created_at, updated_at
FROM v2_automations WHERE workspace_root = ? ORDER BY created_at ASC`, workspaceRoot)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var automations []contracts.Automation
	for rows.Next() {
		a, err := scanAutomation(rows)
		if err != nil {
			return nil, err
		}
		automations = append(automations, a)
	}
	return automations, rows.Err()
}

func (r *automationRepo) CreateRun(ctx context.Context, run contracts.AutomationRun) error {
	_, err := r.execer().Exec(`
INSERT INTO v2_automation_runs (automation_id, dedupe_key, task_id, workflow_run_id, status, summary, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?)`,
		run.AutomationID, run.DedupeKey, run.TaskID, run.WorkflowRunID, run.Status, run.Summary, timeString(run.CreatedAt))
	return err
}

func (r *automationRepo) GetRunByDedupeKey(ctx context.Context, automationID, dedupeKey string) (contracts.AutomationRun, error) {
	return scanAutomationRun(r.execer().QueryRow(`
SELECT automation_id, dedupe_key, task_id, workflow_run_id, status, summary, created_at
FROM v2_automation_runs WHERE automation_id = ? AND dedupe_key = ?`, automationID, dedupeKey))
}

func (r *automationRepo) ListRunsByAutomation(ctx context.Context, automationID string, limit int) ([]contracts.AutomationRun, error) {
	if limit <= 0 {
		limit = 10
	}
	rows, err := r.execer().Query(`
SELECT automation_id, dedupe_key, task_id, workflow_run_id, status, summary, created_at
FROM v2_automation_runs
WHERE automation_id = ?
ORDER BY created_at DESC
LIMIT ?`, automationID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var runs []contracts.AutomationRun
	for rows.Next() {
		run, err := scanAutomationRun(rows)
		if err != nil {
			return nil, err
		}
		runs = append(runs, run)
	}
	return runs, rows.Err()
}

func scanAutomation(row interface {
	Scan(dest ...any) error
}) (contracts.Automation, error) {
	var a contracts.Automation
	var createdAt, updatedAt string
	err := row.Scan(&a.ID, &a.WorkspaceRoot, &a.Title, &a.Goal, &a.State, &a.Owner, &a.WorkflowID, &a.WatcherPath, &a.WatcherCron, &createdAt, &updatedAt)
	if err != nil {
		return contracts.Automation{}, err
	}
	a.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return contracts.Automation{}, err
	}
	a.UpdatedAt, err = parseTime(updatedAt)
	if err != nil {
		return contracts.Automation{}, err
	}
	return a, nil
}

func scanAutomationRun(row interface {
	Scan(dest ...any) error
}) (contracts.AutomationRun, error) {
	var run contracts.AutomationRun
	var createdAt string
	err := row.Scan(&run.AutomationID, &run.DedupeKey, &run.TaskID, &run.WorkflowRunID, &run.Status, &run.Summary, &createdAt)
	if err != nil {
		return contracts.AutomationRun{}, err
	}
	run.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return contracts.AutomationRun{}, err
	}
	return run, nil
}
