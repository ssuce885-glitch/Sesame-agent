package schema

var Migration009 = Migration{
	Version: 9,
	Name:    "workflows",
	Up: `
CREATE TABLE IF NOT EXISTS v2_workflows (
    id TEXT PRIMARY KEY,
    workspace_root TEXT NOT NULL,
    name TEXT NOT NULL DEFAULT '',
    trigger TEXT NOT NULL DEFAULT 'manual',
    owner_role TEXT NOT NULL DEFAULT '',
    input_schema TEXT NOT NULL DEFAULT '',
    steps TEXT NOT NULL DEFAULT '',
    required_tools TEXT NOT NULL DEFAULT '',
    approval_policy TEXT NOT NULL DEFAULT '',
    report_policy TEXT NOT NULL DEFAULT '',
    failure_policy TEXT NOT NULL DEFAULT '',
    resume_policy TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    UNIQUE(id, workspace_root)
);
CREATE INDEX IF NOT EXISTS idx_v2_workflows_workspace ON v2_workflows(workspace_root, created_at);

CREATE TABLE IF NOT EXISTS v2_workflow_runs (
    id TEXT PRIMARY KEY,
    workflow_id TEXT NOT NULL,
    workspace_root TEXT NOT NULL,
    state TEXT NOT NULL DEFAULT 'queued',
    trigger_ref TEXT NOT NULL DEFAULT '',
    task_ids TEXT NOT NULL DEFAULT '',
    report_ids TEXT NOT NULL DEFAULT '',
    approval_ids TEXT NOT NULL DEFAULT '',
    trace TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    FOREIGN KEY (workflow_id, workspace_root) REFERENCES v2_workflows(id, workspace_root) ON UPDATE RESTRICT ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_v2_workflow_runs_workspace ON v2_workflow_runs(workspace_root, created_at);
CREATE INDEX IF NOT EXISTS idx_v2_workflow_runs_workflow ON v2_workflow_runs(workflow_id, created_at);
CREATE INDEX IF NOT EXISTS idx_v2_workflow_runs_state ON v2_workflow_runs(workspace_root, state, created_at);
`,
}
