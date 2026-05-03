package schema

var Migration002 = Migration{
	Version: 2,
	Name:    "tasks_and_reports",
	Up: `
CREATE TABLE IF NOT EXISTS v2_tasks (
    id TEXT PRIMARY KEY,
    workspace_root TEXT NOT NULL,
    session_id TEXT NOT NULL DEFAULT '',
    turn_id TEXT NOT NULL DEFAULT '',
    kind TEXT NOT NULL DEFAULT 'shell',
    state TEXT NOT NULL DEFAULT 'pending',
    prompt TEXT NOT NULL DEFAULT '',
    output_path TEXT NOT NULL DEFAULT '',
    final_text TEXT NOT NULL DEFAULT '',
    outcome TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_v2_tasks_workspace ON v2_tasks(workspace_root);

CREATE TABLE IF NOT EXISTS v2_reports (
    id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL,
    source_kind TEXT NOT NULL DEFAULT 'task_result',
    source_id TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT '',
    severity TEXT NOT NULL DEFAULT 'info',
    title TEXT NOT NULL DEFAULT '',
    summary TEXT NOT NULL DEFAULT '',
    delivered INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_v2_reports_session ON v2_reports(session_id);
`,
}
