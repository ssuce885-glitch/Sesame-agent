package sqlite

import (
	"context"

	"go-agent/internal/types"
)

func (s *Store) FinalizeTurn(ctx context.Context, usage *types.TurnUsage, events []types.Event) ([]types.Event, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	if usage != nil {
		if err := upsertTurnUsageWithExec(ctx, tx, *usage); err != nil {
			return nil, err
		}
	}

	persisted := make([]types.Event, 0, len(events))
	for _, event := range events {
		seq, err := appendEventWithExec(ctx, tx, event)
		if err != nil {
			return nil, err
		}
		event.Seq = seq
		persisted = append(persisted, event)
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return persisted, nil
}
