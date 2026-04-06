package engine

import (
	"context"

	contextstate "go-agent/internal/context"
	"go-agent/internal/model"
	"go-agent/internal/permissions"
	"go-agent/internal/task"
	"go-agent/internal/tools"
	"go-agent/internal/types"
)

type Input struct {
	Session types.Session
	Turn    types.Turn
	Sink    EventSink
}

type RuntimeMetadata struct {
	Provider string
	Model    string
}

type ConversationStore interface {
	ListConversationItems(context.Context, string) ([]model.ConversationItem, error)
	ListConversationSummaries(context.Context, string) ([]model.Summary, error)
	InsertConversationItem(context.Context, string, string, int, model.ConversationItem) error
	InsertConversationSummary(context.Context, string, int, model.Summary) error
	UpsertTurnUsage(context.Context, types.TurnUsage) error
	ListMemoryEntriesByWorkspace(context.Context, string) ([]types.MemoryEntry, error)
	GetProviderCacheHead(context.Context, string, string, string) (types.ProviderCacheHead, bool, error)
	UpsertProviderCacheHead(context.Context, types.ProviderCacheHead) error
	InsertProviderCacheEntry(context.Context, types.ProviderCacheEntry) error
	InsertConversationCompaction(context.Context, types.ConversationCompaction) error
}

type Engine struct {
	model                   model.StreamingClient
	registry                *tools.Registry
	permission              *permissions.Engine
	store                   ConversationStore
	ctxManager              *contextstate.Manager
	compactor               contextstate.Compactor
	runtime                 *contextstate.Runtime
	meta                    RuntimeMetadata
	basePrompt              string
	maxWorkspacePromptBytes int
	maxToolSteps            int
	taskManager             *task.Manager
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
	return NewWithRuntime(
		modelClient,
		registry,
		permission,
		store,
		ctxManager,
		contextstate.NewRuntime(86400, 3),
		compactor,
		RuntimeMetadata{},
		maxToolSteps,
	)
}

func NewWithRuntime(
	modelClient model.StreamingClient,
	registry *tools.Registry,
	permission *permissions.Engine,
	store ConversationStore,
	ctxManager *contextstate.Manager,
	runtime *contextstate.Runtime,
	compactor contextstate.Compactor,
	meta RuntimeMetadata,
	maxToolSteps int,
) *Engine {
	if runtime == nil {
		runtime = contextstate.NewRuntime(86400, 3)
	}
	return &Engine{
		model:        modelClient,
		registry:     registry,
		permission:   permission,
		store:        store,
		ctxManager:   ctxManager,
		runtime:      runtime,
		compactor:    compactor,
		meta:         meta,
		maxToolSteps: maxToolSteps,
	}
}

func (e *Engine) RunTurn(ctx context.Context, in Input) error {
	return runLoop(ctx, e, in)
}

func (e *Engine) SetBaseSystemPrompt(prompt string) {
	if e == nil {
		return
	}
	e.basePrompt = prompt
}

func (e *Engine) SetMaxWorkspacePromptBytes(n int) {
	if e == nil {
		return
	}
	e.maxWorkspacePromptBytes = n
}

func (e *Engine) SetTaskManager(manager *task.Manager) {
	if e == nil {
		return
	}
	e.taskManager = manager
}
