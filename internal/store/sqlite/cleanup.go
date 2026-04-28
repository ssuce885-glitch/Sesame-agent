package sqlite

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"go-agent/internal/types"
)

// CleanupDeprecatedMemories physically deletes deprecated memories whose last use
// or creation time is at or before the threshold.
func (s *Store) CleanupDeprecatedMemories(ctx context.Context, olderThan time.Time) (int64, error) {
	cutoff := olderThan.UTC().Format(timeLayout)
	result, err := s.db.ExecContext(ctx, `
		delete from memory_entries
		where status = ?
			and (
				(last_used_at != '' and last_used_at <= ?)
				or (last_used_at = '' and created_at <= ?)
			)
	`, types.MemoryStatusDeprecated, cutoff, cutoff)
	return rowsAffected(result, err)
}

// CleanupOldReports deletes reports older than the threshold.
func (s *Store) CleanupOldReports(ctx context.Context, workspaceRoot string, olderThan time.Time) (int64, error) {
	result, err := s.db.ExecContext(ctx, `
		delete from reports
		where workspace_root = ? and observed_at < ?
	`, strings.TrimSpace(workspaceRoot), olderThan.UTC().Format(timeLayout))
	return rowsAffected(result, err)
}

// CleanupOldDigestRecords deletes digest records older than the threshold.
func (s *Store) CleanupOldDigestRecords(ctx context.Context, workspaceRoot string, olderThan time.Time) (int64, error) {
	result, err := s.db.ExecContext(ctx, `
		delete from digest_records
		where window_end < ?
			and session_id in (
				select id
				from sessions
				where workspace_root = ?
			)
	`, olderThan.UTC().Format(timeLayout), strings.TrimSpace(workspaceRoot))
	return rowsAffected(result, err)
}

// CleanupOldReportDeliveries deletes report deliveries older than the threshold.
func (s *Store) CleanupOldReportDeliveries(ctx context.Context, workspaceRoot string, olderThan time.Time) (int64, error) {
	result, err := s.db.ExecContext(ctx, `
		delete from report_deliveries
		where workspace_root = ? and observed_at < ?
	`, strings.TrimSpace(workspaceRoot), olderThan.UTC().Format(timeLayout))
	return rowsAffected(result, err)
}

// CleanupOldChildAgentResults deletes child agent results older than the threshold.
func (s *Store) CleanupOldChildAgentResults(ctx context.Context, olderThan time.Time) (int64, error) {
	result, err := s.db.ExecContext(ctx, `
		delete from child_agent_results
		where observed_at < ?
	`, olderThan.UTC().Format(timeLayout))
	return rowsAffected(result, err)
}

// CleanupOldConversationCompactions deletes compactions keeping only the most recent N per (session_id, context_head_id).
func (s *Store) CleanupOldConversationCompactions(ctx context.Context, keepCount int) (int64, error) {
	if keepCount < 0 {
		keepCount = 0
	}
	result, err := s.db.ExecContext(ctx, `
		delete from conversation_compactions
		where rowid not in (
			select rowid
			from (
				select rowid,
					row_number() over (
						partition by session_id, context_head_id
						order by end_position desc, created_at desc, id desc
					) as retention_rank
				from conversation_compactions
			)
			where retention_rank <= ?
		)
	`, keepCount)
	return rowsAffected(result, err)
}

func rowsAffected(result sql.Result, err error) (int64, error) {
	if err != nil {
		return 0, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	return affected, nil
}
