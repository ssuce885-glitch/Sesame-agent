package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"go-agent/internal/types"
)

func (s *Store) InsertConversationArchiveEntry(ctx context.Context, entry types.ConversationArchiveEntry) error {
	decisions, err := marshalArchiveStringArray(entry.Decisions)
	if err != nil {
		return err
	}
	filesChanged, err := marshalArchiveStringArray(entry.FilesChanged)
	if err != nil {
		return err
	}
	errorsAndFixes, err := marshalArchiveStringArray(entry.ErrorsAndFixes)
	if err != nil {
		return err
	}
	toolsUsed, err := marshalArchiveStringArray(entry.ToolsUsed)
	if err != nil {
		return err
	}
	keywords, err := marshalArchiveStringArray(entry.Keywords)
	if err != nil {
		return err
	}
	createdAt := entry.CreatedAt
	if createdAt == "" {
		createdAt = time.Now().UTC().Format(timeLayout)
	}

	_, err = s.db.ExecContext(ctx, `
		insert into conversation_archive_entries (
			id, session_id, range_label, turn_start, turn_end, item_count, summary,
			decisions, files_changed, errors_and_fixes, tools_used, keywords, is_computed, created_at
		) values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		entry.ID,
		entry.SessionID,
		entry.RangeLabel,
		entry.TurnStart,
		entry.TurnEnd,
		entry.ItemCount,
		entry.Summary,
		decisions,
		filesChanged,
		errorsAndFixes,
		toolsUsed,
		keywords,
		boolInt(entry.IsComputed),
		createdAt,
	)
	return err
}

func (s *Store) ListConversationArchiveEntries(ctx context.Context, sessionID string) ([]types.ConversationArchiveEntry, error) {
	rows, err := s.db.QueryContext(ctx, `
		select id, session_id, range_label, turn_start, turn_end, item_count, summary,
			decisions, files_changed, errors_and_fixes, tools_used, keywords, is_computed, created_at
		from conversation_archive_entries
		where session_id = ?
		order by created_at desc, id asc
	`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanConversationArchiveEntries(rows)
}

func (s *Store) SearchConversationArchiveEntries(ctx context.Context, sessionID string, query string) ([]types.ConversationArchiveEntry, error) {
	if query == "" {
		return s.ListConversationArchiveEntries(ctx, sessionID)
	}

	like := "%" + query + "%"
	rows, err := s.db.QueryContext(ctx, `
		select id, session_id, range_label, turn_start, turn_end, item_count, summary,
			decisions, files_changed, errors_and_fixes, tools_used, keywords, is_computed, created_at
		from conversation_archive_entries
		where session_id = ? and (
			range_label like ? or
			summary like ? or
			decisions like ? or
			files_changed like ? or
			errors_and_fixes like ? or
			tools_used like ? or
			keywords like ?
		)
		order by created_at desc, id asc
	`, sessionID, like, like, like, like, like, like, like)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanConversationArchiveEntries(rows)
}

func scanConversationArchiveEntries(rows *sql.Rows) ([]types.ConversationArchiveEntry, error) {
	var out []types.ConversationArchiveEntry
	for rows.Next() {
		var entry types.ConversationArchiveEntry
		var decisions string
		var filesChanged string
		var errorsAndFixes string
		var toolsUsed string
		var keywords string
		var isComputed int
		if err := rows.Scan(
			&entry.ID,
			&entry.SessionID,
			&entry.RangeLabel,
			&entry.TurnStart,
			&entry.TurnEnd,
			&entry.ItemCount,
			&entry.Summary,
			&decisions,
			&filesChanged,
			&errorsAndFixes,
			&toolsUsed,
			&keywords,
			&isComputed,
			&entry.CreatedAt,
		); err != nil {
			return nil, err
		}
		entry.Decisions = unmarshalArchiveStringArray(decisions)
		entry.FilesChanged = unmarshalArchiveStringArray(filesChanged)
		entry.ErrorsAndFixes = unmarshalArchiveStringArray(errorsAndFixes)
		entry.ToolsUsed = unmarshalArchiveStringArray(toolsUsed)
		entry.Keywords = unmarshalArchiveStringArray(keywords)
		entry.IsComputed = isComputed != 0
		out = append(out, entry)
	}
	return out, rows.Err()
}

func marshalArchiveStringArray(values []string) (string, error) {
	if values == nil {
		values = []string{}
	}
	raw, err := json.Marshal(values)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func unmarshalArchiveStringArray(raw string) []string {
	var values []string
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		return nil
	}
	return values
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
