package sqlite

import (
	"context"
	"encoding/json"

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
		entry.CreatedAt.Format(timeLayout),
		entry.UpdatedAt.Format(timeLayout),
	)

	return err
}
