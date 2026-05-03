package schema

var Migration003 = Migration{
	Version: 3,
	Name:    "automation_memory_settings",
	Up: `
CREATE TABLE IF NOT EXISTS v2_automations (
    id TEXT PRIMARY KEY,
    workspace_root TEXT NOT NULL,
    title TEXT NOT NULL DEFAULT '',
    goal TEXT NOT NULL DEFAULT '',
    state TEXT NOT NULL DEFAULT 'active',
    owner TEXT NOT NULL DEFAULT '',
    watcher_path TEXT NOT NULL DEFAULT '',
    watcher_cron TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS v2_automation_runs (
    automation_id TEXT NOT NULL,
    dedupe_key TEXT NOT NULL DEFAULT '',
    task_id TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT '',
    summary TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    PRIMARY KEY (automation_id, dedupe_key)
);

CREATE TABLE IF NOT EXISTS v2_memories (
    id TEXT PRIMARY KEY,
    workspace_root TEXT NOT NULL,
    kind TEXT NOT NULL DEFAULT 'note',
    content TEXT NOT NULL DEFAULT '',
    source TEXT NOT NULL DEFAULT '',
    confidence REAL NOT NULL DEFAULT 1.0,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_v2_memories_workspace ON v2_memories(workspace_root);
`,
}
