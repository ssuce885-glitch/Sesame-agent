package schema

var Migration001 = Migration{
	Version: 1,
	Name:    "core_tables",
	Up: `
CREATE TABLE IF NOT EXISTS v2_sessions (
    id TEXT PRIMARY KEY,
    workspace_root TEXT NOT NULL,
    system_prompt TEXT NOT NULL DEFAULT '',
    permission_profile TEXT NOT NULL DEFAULT '',
    state TEXT NOT NULL DEFAULT 'idle',
    active_turn_id TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS v2_turns (
    id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL,
    kind TEXT NOT NULL DEFAULT 'user_message',
    state TEXT NOT NULL DEFAULT 'created',
    user_message TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_v2_turns_session ON v2_turns(session_id);

CREATE TABLE IF NOT EXISTS v2_messages (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id TEXT NOT NULL,
    turn_id TEXT NOT NULL DEFAULT '',
    role TEXT NOT NULL,
    content TEXT NOT NULL DEFAULT '',
    tool_call_id TEXT NOT NULL DEFAULT '',
    position INTEGER NOT NULL,
    created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_v2_messages_session_pos ON v2_messages(session_id, position);

CREATE TABLE IF NOT EXISTS v2_message_snapshots (
    id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL,
    label TEXT NOT NULL DEFAULT '',
    start_position INTEGER NOT NULL,
    end_position INTEGER NOT NULL,
    summary TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS v2_events (
    seq INTEGER PRIMARY KEY AUTOINCREMENT,
    id TEXT NOT NULL,
    session_id TEXT NOT NULL,
    turn_id TEXT NOT NULL DEFAULT '',
    type TEXT NOT NULL,
    time TEXT NOT NULL,
    payload TEXT NOT NULL DEFAULT '{}'
);
CREATE INDEX IF NOT EXISTS idx_v2_events_session_seq ON v2_events(session_id, seq);

CREATE TABLE IF NOT EXISTS v2_tasks (
    id TEXT PRIMARY KEY,
    workspace_root TEXT NOT NULL,
    session_id TEXT NOT NULL DEFAULT '',
    turn_id TEXT NOT NULL DEFAULT '',
    kind TEXT NOT NULL DEFAULT '',
    state TEXT NOT NULL DEFAULT 'pending',
    prompt TEXT NOT NULL DEFAULT '',
    output_path TEXT NOT NULL DEFAULT '',
    final_text TEXT NOT NULL DEFAULT '',
    outcome TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_v2_tasks_workspace ON v2_tasks(workspace_root);
CREATE INDEX IF NOT EXISTS idx_v2_tasks_runnable ON v2_tasks(state, created_at);

CREATE TABLE IF NOT EXISTS v2_reports (
    id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL,
    source_kind TEXT NOT NULL DEFAULT '',
    source_id TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT '',
    severity TEXT NOT NULL DEFAULT 'info',
    title TEXT NOT NULL DEFAULT '',
    summary TEXT NOT NULL DEFAULT '',
    delivered INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_v2_reports_session ON v2_reports(session_id, created_at);

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
CREATE INDEX IF NOT EXISTS idx_v2_automations_workspace ON v2_automations(workspace_root);

CREATE TABLE IF NOT EXISTS v2_automation_runs (
    automation_id TEXT NOT NULL,
    dedupe_key TEXT NOT NULL,
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
    confidence REAL NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_v2_memories_workspace ON v2_memories(workspace_root);

CREATE TABLE IF NOT EXISTS v2_settings (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL DEFAULT '',
    updated_at TEXT NOT NULL
);
`,
}
