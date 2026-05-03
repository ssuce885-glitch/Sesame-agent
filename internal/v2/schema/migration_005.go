package schema

var Migration005 = Migration{
	Version: 5,
	Name:    "project_state",
	Up: `
CREATE TABLE IF NOT EXISTS v2_project_state (
    workspace_root TEXT PRIMARY KEY,
    summary TEXT NOT NULL DEFAULT '',
    source_session_id TEXT NOT NULL DEFAULT '',
    source_turn_id TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
`,
}
