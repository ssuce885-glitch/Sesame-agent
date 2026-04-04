package httpapi

import (
	"context"

	"go-agent/internal/session"
	"go-agent/internal/types"
)

type Store interface {
	InsertSession(context.Context, types.Session) error
	ListSessions(context.Context) ([]types.Session, error)
	GetSelectedSessionID(context.Context) (string, bool, error)
	SetSelectedSessionID(context.Context, string) error
	InsertTurn(context.Context, types.Turn) error
	DeleteTurn(context.Context, string) error
	ListSessionEvents(context.Context, string, int64) ([]types.Event, error)
}

type Manager interface {
	RegisterSession(types.Session)
	SubmitTurn(context.Context, string, session.SubmitTurnInput) (string, error)
}

type Bus interface {
	Subscribe(sessionID string) <-chan types.Event
}
