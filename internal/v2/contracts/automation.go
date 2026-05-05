package contracts

import "time"

type Automation struct {
	ID            string    `json:"id"`
	WorkspaceRoot string    `json:"workspace_root"`
	Title         string    `json:"title"`
	Goal          string    `json:"goal"`
	State         string    `json:"state"` // "active", "paused"
	Owner         string    `json:"owner"` // "role:<role_id>" or "main"
	WorkflowID    string    `json:"workflow_id,omitempty"`
	WatcherPath   string    `json:"watcher_path"`
	WatcherCron   string    `json:"watcher_cron"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type AutomationRun struct {
	AutomationID  string    `json:"automation_id"`
	DedupeKey     string    `json:"dedupe_key"`
	TaskID        string    `json:"task_id"`
	WorkflowRunID string    `json:"workflow_run_id,omitempty"`
	Status        string    `json:"status"`
	Summary       string    `json:"summary"`
	CreatedAt     time.Time `json:"created_at"`
}
