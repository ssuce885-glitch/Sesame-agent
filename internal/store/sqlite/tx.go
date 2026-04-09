package sqlite

import (
	"context"
	"database/sql"

	"go-agent/internal/types"
)

type RuntimeTx interface {
	InsertRun(context.Context, types.Run) error
	UpsertPlan(context.Context, types.Plan) error
	UpsertTaskRecord(context.Context, types.TaskRecord) error
	UpsertWorktree(context.Context, types.Worktree) error
	ListActivePlansForSession(context.Context, string) ([]types.Plan, error)
}

type runtimeTx struct {
	tx *sql.Tx
}

func (s *Store) WithTx(ctx context.Context, fn func(tx RuntimeTx) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	adapter := runtimeTx{tx: tx}
	if err := fn(adapter); err != nil {
		return err
	}
	return tx.Commit()
}
