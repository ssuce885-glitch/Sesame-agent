package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"go-agent/internal/types"
)

type queryExecContexter interface {
	execContexter
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func (s *Store) AppendEventWithState(ctx context.Context, event types.Event) (types.Event, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return types.Event{}, err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	seq, err := appendEventWithExec(ctx, tx, event)
	if err != nil {
		return types.Event{}, err
	}
	event.Seq = seq
	if err := applyEventStateTransition(ctx, tx, event); err != nil {
		return types.Event{}, err
	}
	if err := tx.Commit(); err != nil {
		return types.Event{}, err
	}
	return event, nil
}

func (s *Store) CommitPermissionResume(ctx context.Context, sessionID, turnID string, continuation types.TurnContinuation, toolRun *types.ToolRun) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	if err := updateTurnStateWithExec(ctx, tx, turnID, types.TurnStateLoopContinue, true); err != nil {
		return err
	}
	if err := updateSessionStateWithExec(ctx, tx, sessionID, types.SessionStateRunning, turnID, true); err != nil {
		return err
	}
	if err := upsertTurnContinuationWithExec(ctx, tx, continuation); err != nil {
		return err
	}
	if toolRun != nil {
		if err := upsertToolRunWithExec(ctx, tx, *toolRun); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func applyEventStateTransition(ctx context.Context, execer execContexter, event types.Event) error {
	switch event.Type {
	case types.EventTurnStarted:
		if err := updateTurnStateWithExec(ctx, execer, event.TurnID, types.TurnStateBuildingContext, true); err != nil {
			return err
		}
		return updateSessionStateWithExec(ctx, execer, event.SessionID, types.SessionStateRunning, event.TurnID, true)
	case types.EventTurnFailed:
		if err := updateTurnStateWithExec(ctx, execer, event.TurnID, types.TurnStateFailed, true); err != nil {
			return err
		}
		queryer, ok := execer.(queryExecContexter)
		if !ok {
			return errors.New("query execer required for turn failure state transition")
		}
		return clearSessionActiveTurnIfMatchesWithExec(ctx, queryer, event.SessionID, event.TurnID, types.SessionStateIdle)
	case types.EventTurnCompleted:
		if err := updateTurnStateWithExec(ctx, execer, event.TurnID, types.TurnStateCompleted, true); err != nil {
			return err
		}
		queryer, ok := execer.(queryExecContexter)
		if !ok {
			return errors.New("query execer required for turn completion state transition")
		}
		return clearSessionActiveTurnIfMatchesWithExec(ctx, queryer, event.SessionID, event.TurnID, types.SessionStateIdle)
	case types.EventTurnInterrupted:
		var payload map[string]string
		if len(event.Payload) > 0 {
			if err := json.Unmarshal(event.Payload, &payload); err != nil {
				return err
			}
		}
		if payload["reason"] == "permission_requested" {
			return nil
		}
		if err := updateTurnStateWithExec(ctx, execer, event.TurnID, types.TurnStateInterrupted, true); err != nil {
			return err
		}
		queryer, ok := execer.(queryExecContexter)
		if !ok {
			return errors.New("query execer required for turn interruption state transition")
		}
		return clearSessionActiveTurnIfMatchesWithExec(ctx, queryer, event.SessionID, event.TurnID, types.SessionStateIdle)
	default:
		return nil
	}
}

func clearSessionActiveTurnIfMatchesWithExec(ctx context.Context, queryer queryExecContexter, sessionID, turnID string, state types.SessionState) error {
	var exists int
	if err := queryer.QueryRowContext(ctx, `
		select 1
		from sessions
		where id = ?`,
		sessionID,
	).Scan(&exists); err != nil {
		return err
	}

	_, err := queryer.ExecContext(ctx, `
		update sessions
		set state = ?, active_turn_id = '', updated_at = ?
		where id = ? and active_turn_id = ?`,
		state,
		time.Now().UTC().Format(timeLayout),
		sessionID,
		turnID,
	)
	return err
}

var _ execContexter = (*sql.Tx)(nil)
