package store

import (
	"context"
	"database/sql"
	"go-agent/internal/v2/contracts"
	"strings"
)

type memoryRepo struct {
	db *sql.DB
	tx *sql.Tx
}

var _ contracts.MemoryRepository = (*memoryRepo)(nil)

func (r *memoryRepo) execer() execer { return repoExec(r.db, r.tx) }

func (r *memoryRepo) Create(ctx context.Context, m contracts.Memory) error {
	m = normalizeMemory(m)
	_, err := r.execer().Exec(`
INSERT INTO v2_memories (id, workspace_root, kind, content, source, owner, visibility, confidence, importance_score, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
	workspace_root = excluded.workspace_root,
	kind = excluded.kind,
	content = excluded.content,
	source = excluded.source,
	owner = excluded.owner,
	visibility = excluded.visibility,
	confidence = excluded.confidence,
	importance_score = excluded.importance_score,
	updated_at = excluded.updated_at`,
		m.ID, m.WorkspaceRoot, m.Kind, m.Content, m.Source, m.Owner, m.Visibility, m.Confidence, m.ImportanceScore, timeString(m.CreatedAt), timeString(m.UpdatedAt))
	return err
}

func (r *memoryRepo) Get(ctx context.Context, id string) (contracts.Memory, error) {
	return scanMemory(r.execer().QueryRow(`
SELECT id, workspace_root, kind, content, source, owner, visibility, confidence, importance_score, created_at, updated_at
FROM v2_memories WHERE id = ?`, id))
}

func (r *memoryRepo) Search(ctx context.Context, workspaceRoot, query string, limit int) ([]contracts.Memory, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return r.ListByWorkspace(ctx, workspaceRoot, limit)
	}
	if r.ftsTableExists() {
		match := ftsMatchQuery(query)
		if match != "" {
			sqlQuery := `
SELECT id, workspace_root, kind, content, source, owner, visibility, confidence, importance_score, created_at, updated_at
FROM v2_memories
WHERE workspace_root = ? AND rowid IN (
	SELECT rowid FROM v2_memories_fts WHERE v2_memories_fts MATCH ?
)
ORDER BY updated_at DESC`
			args := []any{workspaceRoot, match}
			if limit > 0 {
				sqlQuery += ` LIMIT ?`
				args = append(args, limit)
			}
			return r.queryMemories(sqlQuery, args...)
		}
	}

	sqlQuery := `
SELECT id, workspace_root, kind, content, source, owner, visibility, confidence, importance_score, created_at, updated_at
FROM v2_memories
WHERE workspace_root = ? AND (content LIKE ? OR kind LIKE ? OR source LIKE ?)
ORDER BY updated_at DESC`
	like := "%" + query + "%"
	args := []any{workspaceRoot, like, like, like}
	if limit > 0 {
		sqlQuery += ` LIMIT ?`
		args = append(args, limit)
	}
	return r.queryMemories(sqlQuery, args...)
}

func (r *memoryRepo) Delete(ctx context.Context, id string) error {
	_, err := r.execer().Exec(`DELETE FROM v2_memories WHERE id = ?`, id)
	return err
}

func (r *memoryRepo) ListByWorkspace(ctx context.Context, workspaceRoot string, limit int) ([]contracts.Memory, error) {
	sqlQuery := `
SELECT id, workspace_root, kind, content, source, owner, visibility, confidence, importance_score, created_at, updated_at
FROM v2_memories
WHERE workspace_root = ?
ORDER BY updated_at DESC`
	args := []any{workspaceRoot}
	if limit > 0 {
		sqlQuery += ` LIMIT ?`
		args = append(args, limit)
	}
	return r.queryMemories(sqlQuery, args...)
}

func (r *memoryRepo) Count(ctx context.Context, workspaceRoot string) (int, error) {
	var count int
	err := r.execer().QueryRow(`SELECT COUNT(*) FROM v2_memories WHERE workspace_root = ?`, workspaceRoot).Scan(&count)
	return count, err
}

func (r *memoryRepo) queryMemories(sqlQuery string, args ...any) ([]contracts.Memory, error) {
	rows, err := r.execer().Query(sqlQuery, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var memories []contracts.Memory
	for rows.Next() {
		m, err := scanMemory(rows)
		if err != nil {
			return nil, err
		}
		memories = append(memories, m)
	}
	return memories, rows.Err()
}

func (r *memoryRepo) ftsTableExists() bool {
	var name string
	err := r.execer().QueryRow(`
SELECT name FROM sqlite_master
WHERE type = 'table' AND name = 'v2_memories_fts'`).Scan(&name)
	return err == nil
}

func ftsMatchQuery(query string) string {
	parts := strings.Fields(query)
	terms := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		part = strings.ReplaceAll(part, `"`, `""`)
		terms = append(terms, `"`+part+`"`)
	}
	return strings.Join(terms, " AND ")
}

func scanMemory(row interface {
	Scan(dest ...any) error
}) (contracts.Memory, error) {
	var m contracts.Memory
	var createdAt, updatedAt string
	err := row.Scan(&m.ID, &m.WorkspaceRoot, &m.Kind, &m.Content, &m.Source, &m.Owner, &m.Visibility, &m.Confidence, &m.ImportanceScore, &createdAt, &updatedAt)
	if err != nil {
		return contracts.Memory{}, err
	}
	m.CreatedAt, err = parseTime(createdAt)
	if err != nil {
		return contracts.Memory{}, err
	}
	m.UpdatedAt, err = parseTime(updatedAt)
	if err != nil {
		return contracts.Memory{}, err
	}
	return normalizeMemory(m), nil
}

func normalizeMemory(m contracts.Memory) contracts.Memory {
	m.ID = strings.TrimSpace(m.ID)
	m.WorkspaceRoot = strings.TrimSpace(m.WorkspaceRoot)
	m.Kind = strings.TrimSpace(m.Kind)
	if m.Kind == "" {
		m.Kind = "note"
	}
	m.Source = strings.TrimSpace(m.Source)
	m.Owner = strings.TrimSpace(m.Owner)
	if m.Owner == "" {
		m.Owner = "workspace"
	}
	m.Visibility = strings.TrimSpace(m.Visibility)
	if m.Visibility == "" {
		m.Visibility = "workspace"
	}
	if m.Confidence == 0 {
		m.Confidence = 1
	}
	return m
}
