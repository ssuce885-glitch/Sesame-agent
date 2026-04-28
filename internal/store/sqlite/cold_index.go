package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"go-agent/internal/types"
)

const (
	defaultColdSearchLimit = 20
	maxColdSearchLimit     = 100
	coldSearchHalfLife     = 30 * 24 * time.Hour
)

// InsertColdIndexEntry upserts a cold index entry and refreshes its FTS5 content.
func (s *Store) InsertColdIndexEntry(ctx context.Context, entry types.ColdIndexEntry) error {
	entry.ID = strings.TrimSpace(entry.ID)
	entry.WorkspaceID = strings.TrimSpace(entry.WorkspaceID)
	entry.OwnerRoleID = strings.TrimSpace(entry.OwnerRoleID)
	entry.SourceType = strings.TrimSpace(entry.SourceType)
	entry.SourceID = strings.TrimSpace(entry.SourceID)
	if entry.ID == "" {
		return fmt.Errorf("cold index id is required")
	}
	if entry.WorkspaceID == "" {
		return fmt.Errorf("cold index workspace_id is required")
	}
	if entry.SourceType == "" {
		return fmt.Errorf("cold index source_type is required")
	}
	if entry.SourceID == "" {
		return fmt.Errorf("cold index source_id is required")
	}
	if entry.Visibility == "" {
		entry.Visibility = types.MemoryVisibilityShared
	}
	now := time.Now().UTC()
	if entry.OccurredAt.IsZero() {
		entry.OccurredAt = now
	}
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = now
	}

	filesChanged, err := marshalColdStringArray(entry.FilesChanged)
	if err != nil {
		return err
	}
	toolsUsed, err := marshalColdStringArray(entry.ToolsUsed)
	if err != nil {
		return err
	}
	errorTypes, err := marshalColdStringArray(entry.ErrorTypes)
	if err != nil {
		return err
	}
	contextRef, err := json.Marshal(entry.ContextRef)
	if err != nil {
		return err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var previousRowID int64
	switch err := tx.QueryRowContext(ctx, `select rowid from cold_index where id = ?`, entry.ID).Scan(&previousRowID); {
	case err == nil:
		if _, err := tx.ExecContext(ctx, `delete from cold_index_fts where rowid = ?`, previousRowID); err != nil {
			return err
		}
	case err == sql.ErrNoRows:
	default:
		return err
	}

	if _, err := tx.ExecContext(ctx, `
		insert or replace into cold_index (
			id, workspace_id, owner_role_id, visibility, source_type, source_id,
			search_text, summary_line, files_changed, tools_used, error_types,
			occurred_at, created_at, context_ref
		) values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		entry.ID,
		entry.WorkspaceID,
		entry.OwnerRoleID,
		string(entry.Visibility),
		entry.SourceType,
		entry.SourceID,
		entry.SearchText,
		entry.SummaryLine,
		filesChanged,
		toolsUsed,
		errorTypes,
		entry.OccurredAt.UTC().Format(timeLayout),
		entry.CreatedAt.UTC().Format(timeLayout),
		string(contextRef),
	); err != nil {
		return err
	}

	var rowID int64
	if err := tx.QueryRowContext(ctx, `select rowid from cold_index where id = ?`, entry.ID).Scan(&rowID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		`insert into cold_index_fts(rowid, search_text) values (?, ?)`,
		rowID,
		coldIndexFTSText(entry),
	); err != nil {
		return err
	}

	return tx.Commit()
}

// SearchColdIndex executes a structured search with FTS5 + filters + scoring.
func (s *Store) SearchColdIndex(ctx context.Context, query types.ColdSearchQuery) ([]types.ColdIndexEntry, int, error) {
	query.WorkspaceID = strings.TrimSpace(query.WorkspaceID)
	if query.WorkspaceID == "" {
		return nil, 0, fmt.Errorf("workspace_id is required")
	}
	limit := query.Limit
	if limit <= 0 {
		limit = defaultColdSearchLimit
	}
	if limit > maxColdSearchLimit {
		limit = maxColdSearchLimit
	}
	offset := query.Offset
	if offset < 0 {
		offset = 0
	}

	ftsQuery := buildColdFTSQuery(query.TextQuery)
	conditions, args := buildColdIndexConditions(query, ftsQuery != "")
	selectSQL := `
		select ci.id, ci.workspace_id, ci.owner_role_id, ci.visibility, ci.source_type, ci.source_id,
			ci.search_text, ci.summary_line, ci.files_changed, ci.tools_used, ci.error_types,
			ci.occurred_at, ci.created_at, ci.context_ref, 0.0 as text_rank
		from cold_index ci`
	if ftsQuery != "" {
		selectSQL = `
			select ci.id, ci.workspace_id, ci.owner_role_id, ci.visibility, ci.source_type, ci.source_id,
				ci.search_text, ci.summary_line, ci.files_changed, ci.tools_used, ci.error_types,
				ci.occurred_at, ci.created_at, ci.context_ref, bm25(cold_index_fts) as text_rank
			from cold_index ci
			join cold_index_fts on ci.rowid = cold_index_fts.rowid`
	}
	rows, err := s.db.QueryContext(ctx, selectSQL+" where "+strings.Join(conditions, " and "), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	candidates, err := scanColdIndexSearchRows(rows)
	if err != nil {
		return nil, 0, err
	}
	total := len(candidates)
	if total == 0 || offset >= total {
		return []types.ColdIndexEntry{}, total, nil
	}

	scoreColdIndexRows(candidates, ftsQuery != "", time.Now().UTC())
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].score == candidates[j].score {
			return candidates[i].entry.OccurredAt.After(candidates[j].entry.OccurredAt)
		}
		return candidates[i].score > candidates[j].score
	})

	end := offset + limit
	if end > total {
		end = total
	}
	out := make([]types.ColdIndexEntry, 0, end-offset)
	for _, row := range candidates[offset:end] {
		out = append(out, row.entry)
	}
	return out, total, nil
}

// GetColdIndexEntry retrieves a single entry by ID.
func (s *Store) GetColdIndexEntry(ctx context.Context, id string) (types.ColdIndexEntry, bool, error) {
	row := s.db.QueryRowContext(ctx, `
		select id, workspace_id, owner_role_id, visibility, source_type, source_id,
			search_text, summary_line, files_changed, tools_used, error_types,
			occurred_at, created_at, context_ref, 0.0 as text_rank
		from cold_index
		where id = ?
	`, strings.TrimSpace(id))

	searchRow, err := scanColdIndexSearchRow(row)
	if err == sql.ErrNoRows {
		return types.ColdIndexEntry{}, false, nil
	}
	if err != nil {
		return types.ColdIndexEntry{}, false, err
	}
	return searchRow.entry, true, nil
}

type coldIndexSearchRow struct {
	entry     types.ColdIndexEntry
	textRank  float64
	textScore float64
	score     float64
}

type coldIndexScanner interface {
	Scan(dest ...any) error
}

func buildColdIndexConditions(query types.ColdSearchQuery, includeFTS bool) ([]string, []any) {
	conditions := []string{`ci.workspace_id = ?`}
	args := []any{strings.TrimSpace(query.WorkspaceID)}
	if includeFTS {
		conditions = append(conditions, `cold_index_fts match ?`)
		args = append(args, buildColdFTSQuery(query.TextQuery))
	}
	conditions = append(conditions, `(ci.owner_role_id = '' or ci.owner_role_id = ? or ci.visibility in (?, ?))`)
	args = append(args, strings.TrimSpace(query.RoleID), types.MemoryVisibilityShared, types.MemoryVisibilityPromoted)
	if !query.Since.IsZero() {
		conditions = append(conditions, `ci.occurred_at >= ?`)
		args = append(args, query.Since.UTC().Format(timeLayout))
	}
	if !query.Until.IsZero() {
		conditions = append(conditions, `ci.occurred_at <= ?`)
		args = append(args, query.Until.UTC().Format(timeLayout))
	}
	appendColdJSONAnyFilter(&conditions, &args, "files_changed", query.FilesTouched)
	appendColdJSONAnyFilter(&conditions, &args, "tools_used", query.ToolsUsed)
	appendColdJSONAnyFilter(&conditions, &args, "error_types", query.ErrorTypes)
	appendColdTextAnyFilter(&conditions, &args, "ci.source_type", query.SourceTypes)
	return conditions, args
}

func appendColdJSONAnyFilter(conditions *[]string, args *[]any, column string, values []string) {
	filtered := uniqueTrimmedStrings(values)
	if len(filtered) == 0 {
		return
	}
	placeholders := make([]string, 0, len(filtered))
	for _, value := range filtered {
		placeholders = append(placeholders, "?")
		*args = append(*args, value)
	}
	*conditions = append(*conditions, fmt.Sprintf(
		`exists (select 1 from json_each(ci.%s) where value in (%s))`,
		column,
		strings.Join(placeholders, ", "),
	))
}

func appendColdTextAnyFilter(conditions *[]string, args *[]any, column string, values []string) {
	filtered := uniqueTrimmedStrings(values)
	if len(filtered) == 0 {
		return
	}
	placeholders := make([]string, 0, len(filtered))
	for _, value := range filtered {
		placeholders = append(placeholders, "?")
		*args = append(*args, value)
	}
	*conditions = append(*conditions, fmt.Sprintf(`%s in (%s)`, column, strings.Join(placeholders, ", ")))
}

func scanColdIndexSearchRows(rows *sql.Rows) ([]coldIndexSearchRow, error) {
	var out []coldIndexSearchRow
	for rows.Next() {
		row, err := scanColdIndexSearchRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func scanColdIndexSearchRow(scanner coldIndexScanner) (coldIndexSearchRow, error) {
	var row coldIndexSearchRow
	var visibility string
	var filesChanged string
	var toolsUsed string
	var errorTypes string
	var occurredAt string
	var createdAt string
	var contextRef string
	if err := scanner.Scan(
		&row.entry.ID,
		&row.entry.WorkspaceID,
		&row.entry.OwnerRoleID,
		&visibility,
		&row.entry.SourceType,
		&row.entry.SourceID,
		&row.entry.SearchText,
		&row.entry.SummaryLine,
		&filesChanged,
		&toolsUsed,
		&errorTypes,
		&occurredAt,
		&createdAt,
		&contextRef,
		&row.textRank,
	); err != nil {
		return coldIndexSearchRow{}, err
	}
	row.entry.Visibility = types.MemoryVisibility(visibility)
	row.entry.FilesChanged = unmarshalColdStringArray(filesChanged)
	row.entry.ToolsUsed = unmarshalColdStringArray(toolsUsed)
	row.entry.ErrorTypes = unmarshalColdStringArray(errorTypes)
	if strings.TrimSpace(contextRef) != "" {
		if err := json.Unmarshal([]byte(contextRef), &row.entry.ContextRef); err != nil {
			return coldIndexSearchRow{}, err
		}
	}
	var err error
	row.entry.OccurredAt, err = time.Parse(timeLayout, occurredAt)
	if err != nil {
		return coldIndexSearchRow{}, err
	}
	row.entry.CreatedAt, err = time.Parse(timeLayout, createdAt)
	if err != nil {
		return coldIndexSearchRow{}, err
	}
	return row, nil
}

func scoreColdIndexRows(rows []coldIndexSearchRow, hasTextQuery bool, now time.Time) {
	minRank := 0.0
	maxRank := 0.0
	if hasTextQuery && len(rows) > 0 {
		minRank = rows[0].textRank
		maxRank = rows[0].textRank
		for _, row := range rows[1:] {
			if row.textRank < minRank {
				minRank = row.textRank
			}
			if row.textRank > maxRank {
				maxRank = row.textRank
			}
		}
	}

	for i := range rows {
		textScore := 0.0
		if hasTextQuery {
			textScore = 1.0
			if maxRank > minRank {
				textScore = 1 - ((rows[i].textRank - minRank) / (maxRank - minRank))
			}
			if textScore < 0 {
				textScore = 0
			}
		}
		age := now.Sub(rows[i].entry.OccurredAt)
		if age < 0 {
			age = 0
		}
		timeDecay := math.Exp(-float64(age) / float64(coldSearchHalfLife))
		sourceWeight := coldSourceWeight(rows[i].entry.SourceType)
		rows[i].textScore = textScore
		rows[i].score = textScore*0.6 + timeDecay*0.25 + sourceWeight*0.15
	}
}

func coldSourceWeight(sourceType string) float64 {
	switch strings.TrimSpace(sourceType) {
	case "archive":
		return 1.0
	case "memory_deprecated":
		return 0.8
	case "report":
		return 0.6
	case "digest":
		return 0.5
	default:
		return 0.5
	}
}

func coldIndexFTSText(entry types.ColdIndexEntry) string {
	parts := []string{entry.SearchText, entry.SummaryLine}
	parts = append(parts, entry.FilesChanged...)
	parts = append(parts, entry.ToolsUsed...)
	parts = append(parts, entry.ErrorTypes...)
	return strings.Join(parts, " ")
}

func buildColdFTSQuery(raw string) string {
	fields := strings.Fields(strings.TrimSpace(raw))
	if len(fields) == 0 {
		return ""
	}
	parts := make([]string, 0, len(fields))
	for _, field := range fields {
		field = strings.Trim(field, " \t\r\n")
		if field == "" {
			continue
		}
		field = strings.ReplaceAll(field, `"`, `""`)
		parts = append(parts, `"`+field+`"`)
	}
	return strings.Join(parts, " OR ")
}

func marshalColdStringArray(values []string) (string, error) {
	filtered := uniqueTrimmedStrings(values)
	if filtered == nil {
		filtered = []string{}
	}
	raw, err := json.Marshal(filtered)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func unmarshalColdStringArray(raw string) []string {
	var values []string
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		return nil
	}
	return values
}

func uniqueTrimmedStrings(values []string) []string {
	filtered := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		filtered = append(filtered, value)
	}
	return filtered
}
