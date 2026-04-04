package engine

import (
	"context"

	"go-agent/internal/types"
)

type EventSink interface {
	Emit(context.Context, types.Event) error
}
