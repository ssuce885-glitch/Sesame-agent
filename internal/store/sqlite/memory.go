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
	if entry.LastUsedAt.IsZero() {
		entry.LastUsedAt = now
	}
	if entry.Visibility == "" {
		entry.Visibility = types.MemoryVisibilityShared
	}
	if entry.Status == "" {
		entry.Status = types.MemoryStatusActive
	}

	_, err = s.db.ExecContext(ctx, `
		insert into memory_entries (
			id, scope, workspace_id, kind, source_session_id, source_context_head_id,
			owner_role_id, visibility, status, content, source_refs, confidence,
			last_used_at, usage_count, created_at, updated_at
		)
		values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		on conflict(id) do update set
			scope = excluded.scope,
			workspace_id = excluded.workspace_id,
			kind = excluded.kind,
			source_session_id = excluded.source_session_id,
			source_context_head_id = excluded.source_context_head_id,
			owner_role_id = excluded.owner_role_id,
			visibility = excluded.visibility,
			status = excluded.status,
			content = excluded.content,
			source_refs = excluded.source_refs,
			confidence = excluded.confidence,
			last_used_at = excluded.last_used_at,
			usage_count = excluded.usage_count,
			updated_at = excluded.updated_at`,
		entry.ID,
		entry.Scope,
		entry.WorkspaceID,
		entry.Kind,
		entry.SourceSessionID,
		entry.SourceContextHeadID,
		entry.OwnerRoleID,
		entry.Visibility,
		entry.Status,
		entry.Content,
		string(rawRefs),
		entry.Confidence,
		entry.LastUsedAt.UTC().Format(timeLayout),
		entry.UsageCount,
		entry.CreatedAt.UTC().Format(timeLayout),
		entry.UpdatedAt.UTC().Format(timeLayout),
	)

	return err
}

// ListVisibleMemoryEntries returns workspace-scoped and global memory entries
// that are visible to the given role. Visibility rules:
//
//	roleID == "" (main agent): unowned + shared + promoted entries only
//	roleID != "" (role agent): unowned + own (any visibility) + shared + promoted
//
// Global-scope entries are always included (they have no role ownership by
// convention). Superseded and deprecated entries are excluded.
func (s *Store) ListVisibleMemoryEntries(ctx context.Context, workspaceID, roleID string) ([]types.MemoryEntry, error) {
	rows, err := s.db.QueryContext(ctx, `
		select id, scope, workspace_id, kind, source_session_id, source_context_head_id,
			owner_role_id, visibility, status, content, source_refs, confidence,
			last_used_at, usage_count, created_at, updated_at
		from memory_entries
		where (workspace_id = ? or scope = ?)
			and status = ?
			and (
				owner_role_id = ''
				or owner_role_id = ?
				or visibility in (?, ?)
			)
		order by
			case when scope = ? then 0 else 1 end,
			updated_at desc,
			created_at desc
	`, workspaceID, types.MemoryScopeGlobal, types.MemoryStatusActive, roleID, types.MemoryVisibilityShared, types.MemoryVisibilityPromoted, types.MemoryScopeGlobal)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []types.MemoryEntry
	for rows.Next() {
		var entry types.MemoryEntry
		var scope string
		var kind string
		var sourceSessionID string
		var sourceContextHeadID string
		var ownerRoleID string
		var visibility string
		var status string
		var rawRefs string
		var lastUsedAt string
		var usageCount int
		var createdAt string
		var updatedAt string
		if err := rows.Scan(&entry.ID, &scope, &entry.WorkspaceID, &kind, &sourceSessionID, &sourceContextHeadID,
			&ownerRoleID, &visibility, &status, &entry.Content, &rawRefs, &entry.Confidence,
			&lastUsedAt, &usageCount, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		entry.Scope = types.MemoryScope(scope)
		entry.Kind = types.MemoryKind(kind)
		entry.SourceSessionID = sourceSessionID
		entry.SourceContextHeadID = sourceContextHeadID
		entry.OwnerRoleID = ownerRoleID
		entry.Visibility = types.MemoryVisibility(visibility)
		entry.Status = types.MemoryStatus(status)
		entry.UsageCount = usageCount
		if err := json.Unmarshal([]byte(rawRefs), &entry.SourceRefs); err != nil {
			return nil, err
		}
		if lastUsedAt != "" {
			entry.LastUsedAt, err = time.Parse(timeLayout, lastUsedAt)
			if err != nil {
				return nil, err
			}
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

func (s *Store) MarkMemoryEntriesUsed(ctx context.Context, ids []string, usedAt time.Time) error {
	filtered := uniqueTrimmedMemoryIDs(ids)
	if len(filtered) == 0 {
		return nil
	}
	if usedAt.IsZero() {
		usedAt = time.Now().UTC()
	}

	args := make([]any, 0, 1+len(filtered))
	placeholders := make([]string, 0, len(filtered))
	args = append(args, usedAt.UTC().Format(timeLayout))
	for _, id := range filtered {
		args = append(args, id)
		placeholders = append(placeholders, "?")
	}

	_, err := s.db.ExecContext(ctx,
		fmt.Sprintf(`
			update memory_entries
			set last_used_at = ?,
				usage_count = usage_count + 1
			where id in (%s)`, strings.Join(placeholders, ", ")),
		args...,
	)
	return err
}

func (s *Store) DeleteMemoryEntries(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}

	filtered := uniqueTrimmedMemoryIDs(ids)
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

func uniqueTrimmedMemoryIDs(ids []string) []string {
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
	return filtered
}
