package engine

import (
	"context"

	"go-agent/internal/types"
)

type EventSink interface {
	Emit(context.Context, types.Event) error
}

// TurnFinalizingSink allows sinks to atomically persist final usage and final
// turn events (for example, in a single database transaction).
type TurnFinalizingSink interface {
	FinalizeTurn(context.Context, *types.TurnUsage, []types.Event) error
}
