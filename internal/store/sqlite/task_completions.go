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

func (s *Store) ListPendingTaskCompletions(ctx context.Context, sessionID string) ([]types.PendingTaskCompletion, error) {
	return listPendingTaskCompletionsWithQuery(ctx, s.db, sessionID, "")
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
	if len(claimed) > 0 {
		if err := tx.Commit(); err != nil {
			return nil, err
		}
		return claimed, nil
	}

	pending, err := listPendingTaskCompletionsWithQuery(ctx, tx, sessionID, "")
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
	for index := range pending {
		pending[index].InjectedTurnID = turnID
		pending[index].InjectedAt = now
		pending[index].UpdatedAt = now
		if err := upsertPendingTaskCompletionWithExec(ctx, tx, pending[index]); err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return pending, nil
}

type pendingTaskCompletionQueryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func listPendingTaskCompletionsWithQuery(ctx context.Context, queryer pendingTaskCompletionQueryer, sessionID, injectedTurnID string) ([]types.PendingTaskCompletion, error) {
	rows, err := queryer.QueryContext(ctx, `
		select payload, observed_at, injected_turn_id, injected_at, created_at, updated_at
		from pending_task_completions
		where session_id = ? and injected_turn_id = ?
		order by observed_at asc, created_at asc, id asc
	`, strings.TrimSpace(sessionID), strings.TrimSpace(injectedTurnID))
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
	completion.Command = strings.TrimSpace(completion.Command)
	completion.Description = strings.TrimSpace(completion.Description)
	completion.ResultKind = strings.TrimSpace(completion.ResultKind)
	completion.ResultPreview = strings.TrimSpace(completion.ResultPreview)
	completion.InjectedTurnID = strings.TrimSpace(completion.InjectedTurnID)
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
	if parsed, err := parsePendingOptionalTime(injectedAtRaw); err == nil {
		completion.InjectedAt = parsed
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
