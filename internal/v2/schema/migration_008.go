package schema

var Migration008 = Migration{
	Version: 8,
	Name:    "context_blocks",
	Up: `
CREATE TABLE IF NOT EXISTS v2_context_blocks (
    id TEXT PRIMARY KEY,
    workspace_root TEXT NOT NULL,
    type TEXT NOT NULL DEFAULT 'fact',
    owner TEXT NOT NULL DEFAULT 'workspace',
    visibility TEXT NOT NULL DEFAULT 'global',
    source_ref TEXT NOT NULL DEFAULT '',
    title TEXT NOT NULL DEFAULT '',
    summary TEXT NOT NULL DEFAULT '',
    evidence TEXT NOT NULL DEFAULT '',
    confidence REAL NOT NULL DEFAULT 1,
    importance_score REAL NOT NULL DEFAULT 0,
    expiry_policy TEXT NOT NULL DEFAULT '',
    expires_at TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_v2_context_blocks_workspace ON v2_context_blocks(workspace_root, importance_score, updated_at);
CREATE INDEX IF NOT EXISTS idx_v2_context_blocks_owner ON v2_context_blocks(workspace_root, owner, visibility);
CREATE INDEX IF NOT EXISTS idx_v2_context_blocks_source ON v2_context_blocks(source_ref);
`,
}
