package schema

var Migration004 = Migration{
	Version: 4,
	Name:    "memory_fts",
	Up: `
CREATE VIRTUAL TABLE IF NOT EXISTS v2_memories_fts USING fts5(
    content,
    kind,
    source,
    content='v2_memories',
    content_rowid='rowid'
);

-- Populate FTS with existing data
INSERT INTO v2_memories_fts(rowid, content, kind, source)
SELECT rowid, content, kind, source FROM v2_memories;

-- Triggers to keep FTS in sync
CREATE TRIGGER IF NOT EXISTS v2_memories_ai AFTER INSERT ON v2_memories BEGIN
    INSERT INTO v2_memories_fts(rowid, content, kind, source)
    VALUES (new.rowid, new.content, new.kind, new.source);
END;

CREATE TRIGGER IF NOT EXISTS v2_memories_ad AFTER DELETE ON v2_memories BEGIN
    INSERT INTO v2_memories_fts(v2_memories_fts, rowid, content, kind, source)
    VALUES ('delete', old.rowid, old.content, old.kind, old.source);
END;

CREATE TRIGGER IF NOT EXISTS v2_memories_au AFTER UPDATE ON v2_memories BEGIN
    INSERT INTO v2_memories_fts(v2_memories_fts, rowid, content, kind, source)
    VALUES ('delete', old.rowid, old.content, old.kind, old.source);
    INSERT INTO v2_memories_fts(rowid, content, kind, source)
    VALUES (new.rowid, new.content, new.kind, new.source);
END;
`,
}
