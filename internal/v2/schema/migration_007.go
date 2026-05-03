package schema

var Migration007 = Migration{
	Version: 7,
	Name:    "task_parent_trace_fields",
	Up: `
ALTER TABLE v2_tasks ADD COLUMN parent_session_id TEXT NOT NULL DEFAULT '';
ALTER TABLE v2_tasks ADD COLUMN parent_turn_id TEXT NOT NULL DEFAULT '';
ALTER TABLE v2_tasks ADD COLUMN report_session_id TEXT NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_v2_tasks_parent_session ON v2_tasks(parent_session_id, created_at);
CREATE INDEX IF NOT EXISTS idx_v2_tasks_report_session ON v2_tasks(report_session_id, created_at);
`,
}
