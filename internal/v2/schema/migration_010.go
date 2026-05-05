package schema

var Migration010 = Migration{
	Version: 10,
	Name:    "approvals",
	Up: `
CREATE TABLE IF NOT EXISTS v2_approvals (
    id TEXT PRIMARY KEY,
    workflow_run_id TEXT NOT NULL,
    workspace_root TEXT NOT NULL,
    requested_action TEXT NOT NULL DEFAULT '',
    risk_level TEXT NOT NULL DEFAULT '',
    summary TEXT NOT NULL DEFAULT '',
    proposed_payload TEXT NOT NULL DEFAULT '',
    state TEXT NOT NULL DEFAULT 'pending',
    decided_by TEXT NOT NULL DEFAULT '',
    decided_at TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    FOREIGN KEY (workflow_run_id) REFERENCES v2_workflow_runs(id) ON UPDATE RESTRICT ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_v2_approvals_workspace ON v2_approvals(workspace_root, created_at);
CREATE INDEX IF NOT EXISTS idx_v2_approvals_state ON v2_approvals(workspace_root, state, created_at);
CREATE INDEX IF NOT EXISTS idx_v2_approvals_run ON v2_approvals(workflow_run_id, created_at);
`,
}
