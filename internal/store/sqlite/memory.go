package sqlite

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"go-agent/internal/types"
)

func (s *Store) InsertMemoryEntry(ctx context.Context, entry types.MemoryEntry) error {
	return s.UpsertMemoryEntry(ctx, entry)
}

func (s *Store) UpsertMemoryEntry(ctx context.Context, entry types.MemoryEntry) error {
	rawRefs, err := json.Marshal(entry.SourceRefs)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = now
	}
	if entry.UpdatedAt.IsZero() {
		entry.UpdatedAt = now
	}

	_, err = s.db.ExecContext(ctx, `
		insert into memory_entries (id, scope, workspace_id, content, source_refs, confidence, created_at, updated_at)
		values (?, ?, ?, ?, ?, ?, ?, ?)
		on conflict(id) do update set
			scope = excluded.scope,
			workspace_id = excluded.workspace_id,
			content = excluded.content,
			source_refs = excluded.source_refs,
			confidence = excluded.confidence,
			updated_at = excluded.updated_at`,
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
		where workspace_id = ? or scope = ?
		order by
			case when scope = ? then 0 else 1 end,
			updated_at desc,
			created_at desc
	`, workspaceID, types.MemoryScopeGlobal, types.MemoryScopeGlobal)
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

func (s *Store) DeleteMemoryEntries(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}

	filtered := make([]string, 0, len(ids))
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		filtered = append(filtered, id)
	}
	if len(filtered) == 0 {
		return nil
	}

	args := make([]any, 0, len(filtered))
	placeholders := make([]string, 0, len(filtered))
	for _, id := range filtered {
		args = append(args, id)
		placeholders = append(placeholders, "?")
	}

	_, err := s.db.ExecContext(ctx,
		fmt.Sprintf("delete from memory_entries where id in (%s)", strings.Join(placeholders, ", ")),
		args...,
	)
	return err
}
