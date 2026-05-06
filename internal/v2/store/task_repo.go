package store

import (
	"context"
	"database/sql"
	"go-agent/internal/v2/contracts"
)

type taskRepo struct {
	db *sql.DB
	tx *sql.Tx
}

var _ contracts.TaskRepository = (*taskRepo)(nil)

func (r *taskRepo) execer() execer { return repoExec(r.db, r.tx) }

func (r *taskRepo) Create(ctx context.Context, t contracts.Task) error {
	_, err := r.execer().Exec(`
INSERT INTO v2_tasks (id, workspace_root, session_id, role_id, turn_id, parent_session_id, parent_turn_id, report_session_id, kind, state, prompt, output_path, final_text, outcome, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		t.ID, t.WorkspaceRoot, t.SessionID, t.RoleID, t.TurnID, t.ParentSessionID, t.ParentTurnID, t.ReportSessionID, t.Kind, t.State, t.Prompt, t.OutputPath, t.FinalText, t.Outcome, timeString(t.CreatedAt), timeString(t.UpdatedAt))
	return err
}

func (r *taskRepo) Get(ctx context.Context, id string) (contracts.Task, error) {
	return scanTask(r.execer().QueryRow(`
SELECT id, workspace_root, session_id, role_id, turn_id, parent_session_id, parent_turn_id, report_session_id, kind, state, prompt, output_path, final_text, outcome, created_at, updated_at
FROM v2_tasks WHERE id = ?`, id))
}

func (r *taskRepo) Update(ctx context.Context, t contracts.Task) error {
	_, err := r.execer().Exec(`
UPDATE v2_tasks
SET workspace_root = ?, session_id = ?, role_id = ?, turn_id = ?, parent_session_id = ?, parent_turn_id = ?, report_session_id = ?, kind = ?, state = ?, prompt = ?, output_path = ?, final_text = ?, outcome = ?, updated_at = ?
WHERE id = ?`,
		t.WorkspaceRoot, t.SessionID, t.RoleID, t.TurnID, t.ParentSessionID, t.ParentTurnID, t.ReportSessionID, t.Kind, t.State, t.Prompt, t.OutputPath, t.FinalText, t.Outcome, timeString(t.UpdatedAt), t.ID)
	return err
}

func (r *taskRepo) ListByWorkspace(ctx context.Context, workspaceRoot string) ([]contracts.Task, error) {
	rows, err := r.execer().Query(`
SELECT id, workspace_root, session_id, role_id, turn_id, parent_session_id, parent_turn_id, report_session_id, kind, state, prompt, output_path, final_text, outcome, created_at, updated_at
FROM v2_tasks WHERE workspace_root = ? ORDER BY created_at ASC`, workspaceRoot)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTaskList(rows)
}

func (r *taskRepo) ListRunnable(ctx context.Context) ([]contracts.Task, error) {
	rows, err := r.execer().Query(`
SELECT id, workspace_root, session_id, role_id, turn_id, parent_session_id, parent_turn_id, report_session_id, kind, state, prompt, output_path, final_text, outcome, created_at, updated_at
FROM v2_tasks WHERE state = 'pending' ORDER BY created_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTaskList(rows)
}

func (r *taskRepo) ListRunning(ctx context.Context) ([]contracts.Task, error) {
	rows, err := r.execer().Query(`
SELECT id, workspace_root, session_id, role_id, turn_id, parent_session_id, parent_turn_id, report_session_id, kind, state, prompt, output_path, final_text, outcome, created_at, updated_at
FROM v2_tasks WHERE state = 'running' ORDER BY created_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTaskList(rows)
}

func scanTask(row interface {
	Scan(dest ...any) error
}) (contracts.Task, error) {
	var t contracts.Task
	var createdAt, updatedAt string
	err := row.Scan(&t.ID, &t.WorkspaceRoot, &t.SessionID, &t.RoleID, &t.TurnID, &t.ParentSessionID, &t.ParentTurnID, &t.ReportSessionID, &t.Kind, &t.State, &t.Prompt, &t.OutputPath, &t.FinalText, &t.Outcome, &createdAt, &updatedAt)
	if err != nil {
		return contracts.Task{}, err
	}
	t.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return contracts.Task{}, err
	}
	t.UpdatedAt, err = parseTime(updatedAt)
	if err != nil {
		return contracts.Task{}, err
	}
	return t, nil
}

func scanTaskList(rows *sql.Rows) ([]contracts.Task, error) {
	var tasks []contracts.Task
	for rows.Next() {
		t, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, t)
	}
	return tasks, rows.Err()
}
