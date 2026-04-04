package engine

import (
	"context"

	contextstate "go-agent/internal/context"
	"go-agent/internal/model"
	"go-agent/internal/permissions"
	"go-agent/internal/tools"
	"go-agent/internal/types"
)

type Input struct {
	Session types.Session
	Turn    types.Turn
	Sink    EventSink
}

type ConversationStore interface {
	ListConversationItems(context.Context, string) ([]model.ConversationItem, error)
	ListConversationSummaries(context.Context, string) ([]model.Summary, error)
	InsertConversationItem(context.Context, string, string, int, model.ConversationItem) error
	InsertConversationSummary(context.Context, string, int, model.Summary) error
	ListMemoryEntriesByWorkspace(context.Context, string) ([]types.MemoryEntry, error)
}

type Engine struct {
	model        model.StreamingClient
	registry     *tools.Registry
	permission   *permissions.Engine
	store        ConversationStore
	ctxManager   *contextstate.Manager
	compactor    contextstate.Compactor
	maxToolSteps int
}

func New(
	modelClient model.StreamingClient,
	registry *tools.Registry,
	permission *permissions.Engine,
	store ConversationStore,
	ctxManager *contextstate.Manager,
	compactor contextstate.Compactor,
	maxToolSteps int,
) *Engine {
	return &Engine{
		model:        modelClient,
		registry:     registry,
		permission:   permission,
		store:        store,
		ctxManager:   ctxManager,
		compactor:    compactor,
		maxToolSteps: maxToolSteps,
	}
}

func (e *Engine) RunTurn(ctx context.Context, in Input) error {
	return runLoop(ctx, e, in)
}
