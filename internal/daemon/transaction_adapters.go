package daemon

import (
	"context"

	"go-agent/internal/runtimegraph"
	"go-agent/internal/scheduler"
	"go-agent/internal/store/sqlite"
)

// Keep sqlite's concrete transaction callback type at the wiring boundary.
type runtimeGraphStoreAdapter struct {
	*sqlite.Store
}

func (a runtimeGraphStoreAdapter) WithTx(ctx context.Context, fn func(runtimegraph.RuntimeTx) error) error {
	return a.Store.WithTx(ctx, func(tx sqlite.RuntimeTx) error {
		return fn(tx)
	})
}

type schedulerStoreAdapter struct {
	*sqlite.Store
}

func (a schedulerStoreAdapter) WithTx(ctx context.Context, fn func(scheduler.RuntimeTx) error) error {
	return a.Store.WithTx(ctx, func(tx sqlite.RuntimeTx) error {
		return fn(tx)
	})
}
