package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"go-agent/internal/types"
)

func (s *Store) UpsertIntentConfirmation(ctx context.Context, confirmation types.IntentConfirmation) error {
	confirmation = normalizeIntentConfirmation(confirmation)
	payload, err := json.Marshal(confirmation)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `
		insert into session_pending_confirmations (
			session_id, source_turn_id, payload, created_at, updated_at
		) values (?, ?, ?, ?, ?)
		on conflict(session_id) do update set
			source_turn_id = excluded.source_turn_id,
			payload = excluded.payload,
			updated_at = excluded.updated_at
	`, confirmation.SessionID, confirmation.SourceTurnID, string(payload), confirmation.CreatedAt.Format(timeLayout), confirmation.UpdatedAt.Format(timeLayout))
	return err
}

func (s *Store) GetIntentConfirmation(ctx context.Context, sessionID string) (types.IntentConfirmation, bool, error) {
	row := s.db.QueryRowContext(ctx, `
		select payload, created_at, updated_at
		from session_pending_confirmations
		where session_id = ?
	`, strings.TrimSpace(sessionID))

	var (
		payload   string
		createdAt string
		updatedAt string
	)
	if err := row.Scan(&payload, &createdAt, &updatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return types.IntentConfirmation{}, false, nil
		}
		return types.IntentConfirmation{}, false, err
	}

	var confirmation types.IntentConfirmation
	if err := json.Unmarshal([]byte(payload), &confirmation); err != nil {
		return types.IntentConfirmation{}, false, err
	}
	if parsed, err := time.Parse(timeLayout, createdAt); err == nil {
		confirmation.CreatedAt = parsed
	}
	if parsed, err := time.Parse(timeLayout, updatedAt); err == nil {
		confirmation.UpdatedAt = parsed
	}
	confirmation.SessionID = strings.TrimSpace(sessionID)
	return normalizeIntentConfirmation(confirmation), true, nil
}

func (s *Store) DeleteIntentConfirmation(ctx context.Context, sessionID string) error {
	_, err := s.db.ExecContext(ctx, `
		delete from session_pending_confirmations
		where session_id = ?
	`, strings.TrimSpace(sessionID))
	return err
}

func normalizeIntentConfirmation(confirmation types.IntentConfirmation) types.IntentConfirmation {
	confirmation.SessionID = strings.TrimSpace(confirmation.SessionID)
	confirmation.SourceTurnID = strings.TrimSpace(confirmation.SourceTurnID)
	confirmation.RawMessage = strings.TrimSpace(confirmation.RawMessage)
	confirmation.ConfirmText = strings.TrimSpace(confirmation.ConfirmText)
	confirmation.RecommendedProfile = strings.TrimSpace(confirmation.RecommendedProfile)
	confirmation.FallbackProfile = strings.TrimSpace(confirmation.FallbackProfile)
	now := time.Now().UTC()
	if confirmation.CreatedAt.IsZero() {
		confirmation.CreatedAt = now
	}
	if confirmation.UpdatedAt.IsZero() {
		confirmation.UpdatedAt = confirmation.CreatedAt
	}
	return confirmation
}
