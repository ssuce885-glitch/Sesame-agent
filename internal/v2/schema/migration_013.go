package schema

var Migration013 = Migration{
	Version: 13,
	Name:    "memory_scope_metadata",
	Up: `
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
ALTER TABLE v2_memories ADD COLUMN owner TEXT NOT NULL DEFAULT 'workspace';
ALTER TABLE v2_memories ADD COLUMN visibility TEXT NOT NULL DEFAULT 'workspace';
ALTER TABLE v2_memories ADD COLUMN importance_score REAL NOT NULL DEFAULT 0;
`,
}
