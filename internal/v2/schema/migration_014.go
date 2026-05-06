package schema

var Migration014 = Migration{
	Version: 14,
	Name:    "role_runtime_states",
	Up: `
CREATE TABLE IF NOT EXISTS v2_role_runtime_states (
    workspace_root TEXT NOT NULL,
    role_id TEXT NOT NULL,
    summary TEXT NOT NULL DEFAULT '',
    source_session_id TEXT NOT NULL DEFAULT '',
    source_turn_id TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    PRIMARY KEY (workspace_root, role_id)
);
`,
}
