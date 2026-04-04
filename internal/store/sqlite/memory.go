package sqlite

import (
	"context"
	"encoding/json"
	"time"

	"go-agent/internal/types"
)

func (s *Store) InsertMemoryEntry(ctx context.Context, entry types.MemoryEntry) error {
	rawRefs, err := json.Marshal(entry.SourceRefs)
	if err != nil {
		return err
	}

	_, err = s.db.ExecContext(ctx, `
		insert into memory_entries (id, scope, workspace_id, content, source_refs, confidence, created_at, updated_at)
		values (?, ?, ?, ?, ?, ?, ?, ?)`,
		entry.ID,
		entry.Scope,
		entry.WorkspaceID,
		entry.Content,
		string(rawRefs),
		entry.Confidence,
		entry.CreatedAt.UTC().Format(timeLayout),
		entry.UpdatedAt.UTC().Format(timeLayout),
	)

	return err
}

func (s *Store) ListMemoryEntriesByWorkspace(ctx context.Context, workspaceID string) ([]types.MemoryEntry, error) {
	rows, err := s.db.QueryContext(ctx, `
		select id, scope, workspace_id, content, source_refs, confidence, created_at, updated_at
		from memory_entries
		where workspace_id = ?
		order by updated_at desc, created_at desc
	`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []types.MemoryEntry
	for rows.Next() {
		var entry types.MemoryEntry
		var scope string
		var rawRefs string
		var createdAt string
		var updatedAt string
		if err := rows.Scan(&entry.ID, &scope, &entry.WorkspaceID, &entry.Content, &rawRefs, &entry.Confidence, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		entry.Scope = types.MemoryScope(scope)
		if err := json.Unmarshal([]byte(rawRefs), &entry.SourceRefs); err != nil {
			return nil, err
		}
		entry.CreatedAt, err = time.Parse(timeLayout, createdAt)
		if err != nil {
			return nil, err
		}
		entry.UpdatedAt, err = time.Parse(timeLayout, updatedAt)
		if err != nil {
			return nil, err
		}
		out = append(out, entry)
	}

	return out, rows.Err()
}
