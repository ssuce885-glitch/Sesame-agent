package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"go-agent/internal/types"
)

func (s *Store) UpsertPendingTaskCompletion(ctx context.Context, completion types.PendingTaskCompletion) error {
	return upsertPendingTaskCompletionWithExec(ctx, s.db, completion)
}

func (s *Store) UpsertPendingChildReport(ctx context.Context, report types.ChildReport) error {
	return upsertPendingTaskCompletionWithExec(ctx, s.db, types.PendingTaskCompletion(report))
}

func (s *Store) ListPendingTaskCompletions(ctx context.Context, sessionID string) ([]types.PendingTaskCompletion, error) {
	return listPendingTaskCompletionsWithQuery(ctx, s.db, sessionID, "")
}

func (s *Store) ListPendingChildReports(ctx context.Context, sessionID string) ([]types.ChildReport, error) {
	items, err := listPendingTaskCompletionsWithQuery(ctx, s.db, sessionID, "")
	if err != nil {
		return nil, err
	}
	return pendingTaskCompletionsToChildReports(items), nil
}

func (s *Store) CountPendingChildReports(ctx context.Context, sessionID string) (int, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return 0, nil
	}
	var count int
	if err := s.db.QueryRowContext(ctx, `
		select count(1)
		from pending_task_completions
		where session_id = ? and injected_turn_id = ''
	`, sessionID).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func (s *Store) ClaimPendingTaskCompletionsForTurn(ctx context.Context, sessionID, turnID string) ([]types.PendingTaskCompletion, error) {
	turnID = strings.TrimSpace(turnID)
	if strings.TrimSpace(sessionID) == "" || turnID == "" {
		return nil, nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	claimed, err := listPendingTaskCompletionsWithQuery(ctx, tx, sessionID, turnID)
	if err != nil {
		return nil, err
	}

	pending, err := listPendingTaskCompletionsWithQuery(ctx, tx, sessionID, "")
	if err != nil {
		return nil, err
	}
	if len(pending) == 0 {
		if err := tx.Commit(); err != nil {
			return nil, err
		}
		return claimed, nil
	}

	now := time.Now().UTC()
	for index := range pending {
		pending[index].ClaimedTurnID = turnID
		pending[index].ClaimedAt = now
		pending[index].InjectedTurnID = turnID
		pending[index].InjectedAt = now
		pending[index].UpdatedAt = now
		if err := upsertPendingTaskCompletionWithExec(ctx, tx, pending[index]); err != nil {
			return nil, err
		}
	}

	claimed, err = listPendingTaskCompletionsWithQuery(ctx, tx, sessionID, turnID)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return claimed, nil
}

func (s *Store) ClaimPendingChildReportsForTurn(ctx context.Context, sessionID, turnID string) ([]types.ChildReport, error) {
	claimed, err := s.ClaimPendingTaskCompletionsForTurn(ctx, sessionID, turnID)
	if err != nil {
		return nil, err
	}
	return pendingTaskCompletionsToChildReports(claimed), nil
}

func (s *Store) ClaimPendingChildReportsForTurnIncremental(ctx context.Context, sessionID, turnID string, limit int) ([]types.ChildReport, error) {
	turnID = strings.TrimSpace(turnID)
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" || turnID == "" {
		return nil, nil
	}
	if limit < 0 {
		limit = 0
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	pending, err := listPendingTaskCompletionsWithQueryLimit(ctx, tx, sessionID, "", limit)
	if err != nil {
		return nil, err
	}
	if len(pending) == 0 {
		if err := tx.Commit(); err != nil {
			return nil, err
		}
		return nil, nil
	}

	now := time.Now().UTC()
	for i := range pending {
		pending[i].ClaimedTurnID = turnID
		pending[i].ClaimedAt = now
		pending[i].InjectedTurnID = turnID
		pending[i].InjectedAt = now
		pending[i].UpdatedAt = now
		if err := upsertPendingTaskCompletionWithExec(ctx, tx, pending[i]); err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return pendingTaskCompletionsToChildReports(pending), nil
}

func (s *Store) RequeueClaimedChildReportsForTurn(ctx context.Context, turnID string) error {
	turnID = strings.TrimSpace(turnID)
	if turnID == "" {
		return nil
	}
	_, err := s.db.ExecContext(ctx, `
		update pending_task_completions
		set injected_turn_id = '', injected_at = '', updated_at = ?
		where injected_turn_id = ?
	`, time.Now().UTC().Format(timeLayout), turnID)
	return err
}

type pendingTaskCompletionQueryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func listPendingTaskCompletionsWithQuery(ctx context.Context, queryer pendingTaskCompletionQueryer, sessionID, injectedTurnID string) ([]types.PendingTaskCompletion, error) {
	return listPendingTaskCompletionsWithQueryLimit(ctx, queryer, sessionID, injectedTurnID, 0)
}

func listPendingTaskCompletionsWithQueryLimit(ctx context.Context, queryer pendingTaskCompletionQueryer, sessionID, injectedTurnID string, limit int) ([]types.PendingTaskCompletion, error) {
	query := `
		select payload, observed_at, injected_turn_id, injected_at, created_at, updated_at
		from pending_task_completions
		where session_id = ? and injected_turn_id = ?
		order by observed_at asc, created_at asc, id asc
	`
	args := []any{strings.TrimSpace(sessionID), strings.TrimSpace(injectedTurnID)}
	if limit > 0 {
		query += "\n limit ?"
		args = append(args, limit)
	}

	rows, err := queryer.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]types.PendingTaskCompletion, 0)
	for rows.Next() {
		var (
			rawPayload     string
			observedAt     string
			injectedTurnID string
			injectedAt     string
			createdAt      string
			updatedAt      string
		)
		if err := rows.Scan(&rawPayload, &observedAt, &injectedTurnID, &injectedAt, &createdAt, &updatedAt); err != nil {
			return nil, err
		}

		var completion types.PendingTaskCompletion
		if err := json.Unmarshal([]byte(rawPayload), &completion); err != nil {
			return nil, err
		}
		applyPendingTaskCompletionTimes(&completion, observedAt, injectedTurnID, injectedAt, createdAt, updatedAt)
		out = append(out, completion)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func upsertPendingTaskCompletionWithExec(ctx context.Context, execer execContexter, completion types.PendingTaskCompletion) error {
	completion = normalizePendingTaskCompletion(completion)
	payload, err := json.Marshal(completion)
	if err != nil {
		return err
	}

	_, err = execer.ExecContext(ctx, `
		insert into pending_task_completions (
			id, session_id, task_id, observed_at, injected_turn_id, injected_at, payload, created_at, updated_at
		)
		values (?, ?, ?, ?, ?, ?, ?, ?, ?)
		on conflict(id) do update set
			session_id = excluded.session_id,
			task_id = excluded.task_id,
			observed_at = excluded.observed_at,
			injected_turn_id = excluded.injected_turn_id,
			injected_at = excluded.injected_at,
			payload = excluded.payload,
			updated_at = excluded.updated_at
	`,
		completion.ID,
		completion.SessionID,
		completion.TaskID,
		formatPendingOptionalTime(completion.ObservedAt),
		completion.InjectedTurnID,
		formatPendingOptionalTime(completion.InjectedAt),
		string(payload),
		completion.CreatedAt.Format(timeLayout),
		completion.UpdatedAt.Format(timeLayout),
	)
	return err
}

func normalizePendingTaskCompletion(completion types.PendingTaskCompletion) types.PendingTaskCompletion {
	now := time.Now().UTC()
	if strings.TrimSpace(completion.ID) == "" {
		completion.ID = strings.TrimSpace(completion.TaskID)
	}
	if strings.TrimSpace(completion.ID) == "" {
		completion.ID = types.NewID("task_completion")
	}
	completion.SessionID = strings.TrimSpace(completion.SessionID)
	completion.ParentTurnID = strings.TrimSpace(completion.ParentTurnID)
	completion.TaskID = strings.TrimSpace(completion.TaskID)
	completion.TaskType = strings.TrimSpace(completion.TaskType)
	completion.TaskKind = strings.TrimSpace(completion.TaskKind)
	completion.Source = normalizeChildReportSource(completion.Source)
	completion.Status = normalizeChildReportStatus(completion.Status)
	completion.Objective = strings.TrimSpace(firstNonEmptyPendingString(completion.Objective, completion.Description, completion.Command))
	completion.Command = strings.TrimSpace(completion.Command)
	completion.Description = strings.TrimSpace(completion.Description)
	completion.MailboxReportID = strings.TrimSpace(completion.MailboxReportID)
	completion.ResultKind = strings.TrimSpace(completion.ResultKind)
	completion.ResultText = strings.TrimSpace(completion.ResultText)
	completion.ResultPreview = strings.TrimSpace(completion.ResultPreview)
	completion.ClaimedTurnID = strings.TrimSpace(firstNonEmptyPendingString(completion.ClaimedTurnID, completion.InjectedTurnID))
	completion.InjectedTurnID = completion.ClaimedTurnID
	if completion.ClaimedAt.IsZero() && !completion.InjectedAt.IsZero() {
		completion.ClaimedAt = completion.InjectedAt
	}
	if !completion.ClaimedAt.IsZero() {
		completion.ClaimedAt = completion.ClaimedAt.UTC()
		completion.InjectedAt = completion.ClaimedAt
	}
	if completion.Objective != "" && completion.Description == "" {
		completion.Description = completion.Objective
	}
	if completion.ResultText != "" {
		completion.ResultReady = true
	}
	if completion.Status == "" && completion.ResultReady {
		completion.Status = types.ChildReportStatusSuccess
	}
	if completion.CreatedAt.IsZero() {
		completion.CreatedAt = now
	} else {
		completion.CreatedAt = completion.CreatedAt.UTC()
	}
	if completion.UpdatedAt.IsZero() {
		completion.UpdatedAt = completion.CreatedAt
	} else {
		completion.UpdatedAt = completion.UpdatedAt.UTC()
	}
	if completion.ObservedAt.IsZero() {
		completion.ObservedAt = completion.UpdatedAt
	} else {
		completion.ObservedAt = completion.ObservedAt.UTC()
	}
	if !completion.InjectedAt.IsZero() {
		completion.InjectedAt = completion.InjectedAt.UTC()
	}
	return completion
}

func applyPendingTaskCompletionTimes(completion *types.PendingTaskCompletion, observedAtRaw, injectedTurnID, injectedAtRaw, createdAtRaw, updatedAtRaw string) {
	if completion == nil {
		return
	}
	if parsed, err := parsePendingOptionalTime(observedAtRaw); err == nil {
		completion.ObservedAt = parsed
	}
	completion.InjectedTurnID = strings.TrimSpace(injectedTurnID)
	completion.ClaimedTurnID = completion.InjectedTurnID
	if parsed, err := parsePendingOptionalTime(injectedAtRaw); err == nil {
		completion.InjectedAt = parsed
		completion.ClaimedAt = parsed
	}
	if parsed, err := parsePendingOptionalTime(createdAtRaw); err == nil {
		completion.CreatedAt = parsed
	}
	if parsed, err := parsePendingOptionalTime(updatedAtRaw); err == nil {
		completion.UpdatedAt = parsed
	}
}

func parsePendingOptionalTime(raw string) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, nil
	}
	parsed, err := time.Parse(timeLayout, raw)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse time %q: %w", raw, err)
	}
	return parsed.UTC(), nil
}

func formatPendingOptionalTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(timeLayout)
}

func pendingTaskCompletionsToChildReports(items []types.PendingTaskCompletion) []types.ChildReport {
	if len(items) == 0 {
		return nil
	}
	out := make([]types.ChildReport, len(items))
	for i := range items {
		out[i] = types.ChildReport(items[i])
	}
	return out
}

func normalizeChildReportStatus(status types.ChildReportStatus) types.ChildReportStatus {
	value := strings.ToLower(strings.TrimSpace(string(status)))
	switch value {
	case string(types.ChildReportStatusSuccess):
		return types.ChildReportStatusSuccess
	case string(types.ChildReportStatusBlocked):
		return types.ChildReportStatusBlocked
	case string(types.ChildReportStatusFailure):
		return types.ChildReportStatusFailure
	default:
		return types.ChildReportStatus(strings.TrimSpace(string(status)))
	}
}

func normalizeChildReportSource(source types.ChildReportSource) types.ChildReportSource {
	value := strings.ToLower(strings.TrimSpace(string(source)))
	switch value {
	case string(types.ChildReportSourceChat):
		return types.ChildReportSourceChat
	case string(types.ChildReportSourceAutomation):
		return types.ChildReportSourceAutomation
	case string(types.ChildReportSourceCron):
		return types.ChildReportSourceCron
	default:
		return types.ChildReportSource(strings.TrimSpace(string(source)))
	}
}

func firstNonEmptyPendingString(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
