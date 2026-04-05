package httpapi

import (
	"context"

	"go-agent/internal/model"
	"go-agent/internal/session"
	"go-agent/internal/types"
)

type Store interface {
	InsertSession(context.Context, types.Session) error
	ListSessions(context.Context) ([]types.Session, error)
	GetSession(context.Context, string) (types.Session, bool, error)
	UpdateSessionSystemPrompt(context.Context, string, string) (types.Session, bool, error)
	GetSelectedSessionID(context.Context) (string, bool, error)
	SetSelectedSessionID(context.Context, string) error
	DeleteSession(context.Context, string) (string, bool, error)
	InsertTurn(context.Context, types.Turn) error
	DeleteTurn(context.Context, string) error
	ListTurnsBySession(context.Context, string) ([]types.Turn, error)
	ListConversationItems(context.Context, string) ([]model.ConversationItem, error)
	ListSessionEvents(context.Context, string, int64) ([]types.Event, error)
	LatestSessionEventSeq(context.Context, string) (int64, error)
}

type Manager interface {
	RegisterSession(types.Session)
	UpdateSession(types.Session) bool
	SubmitTurn(context.Context, string, session.SubmitTurnInput) (string, error)
}

type Bus interface {
	Subscribe(sessionID string) (<-chan types.Event, func())
}
