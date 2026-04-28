package engine

import (
	"context"
	"sync"

	contextstate "go-agent/internal/context"
	"go-agent/internal/model"
	"go-agent/internal/permissions"
	"go-agent/internal/runtimegraph"
	"go-agent/internal/scheduler"
	"go-agent/internal/session"
	"go-agent/internal/task"
	"go-agent/internal/tools"
	"go-agent/internal/types"
)

type Input struct {
	Session             types.Session
	SessionRole         types.SessionRole
	Turn                types.Turn
	TaskID              string
	Sink                EventSink
	ActivatedSkillNames []string
}

type RuntimeMetadata struct {
	Provider string
	Model    string
}

type ConversationStore interface {
	GetCurrentContextHeadID(context.Context) (string, bool, error)
	ListConversationItems(context.Context, string) ([]model.ConversationItem, error)
	MaxConversationPosition(context.Context, string) (int, bool, error)
	ListConversationTimelineItemsByContextHead(context.Context, string, string) ([]types.ConversationTimelineItem, error)
	ListConversationItemsByContextHead(context.Context, string, string) ([]model.ConversationItem, error)
	ListConversationCompactionsByStoredContextHead(context.Context, string, string) ([]types.ConversationCompaction, error)
	GetContextHeadSummary(context.Context, string, string) (types.ContextHeadSummary, bool, error)
	InsertConversationItem(context.Context, string, string, int, model.ConversationItem) error
	InsertConversationItemWithContextHead(context.Context, string, string, string, int, model.ConversationItem) error
	GetConversationItemIDByContextHeadAndPosition(context.Context, string, string, int) (int64, bool, error)
	UpsertTurnUsage(context.Context, types.TurnUsage) error
	UpsertContextHeadSummary(context.Context, types.ContextHeadSummary) error
	UpsertMemoryEntry(context.Context, types.MemoryEntry) error
	DeleteMemoryEntries(context.Context, []string) error
	ListVisibleMemoryEntries(context.Context, string, string) ([]types.MemoryEntry, error)
	GetProviderCacheHead(context.Context, string, string, string) (types.ProviderCacheHead, bool, error)
	UpsertProviderCacheHead(context.Context, types.ProviderCacheHead) error
	InsertProviderCacheEntry(context.Context, types.ProviderCacheEntry) error
	InsertConversationCompaction(context.Context, types.ConversationCompaction) error
	InsertConversationCompactionWithContextHead(context.Context, types.ConversationCompaction) error
	InsertConversationArchiveEntry(context.Context, types.ConversationArchiveEntry) error
	ListConversationArchiveEntries(context.Context, string) ([]types.ConversationArchiveEntry, error)
	SearchConversationArchiveEntries(context.Context, string, string) ([]types.ConversationArchiveEntry, error)
	InsertColdIndexEntry(context.Context, types.ColdIndexEntry) error
	SearchColdIndex(context.Context, types.ColdSearchQuery) ([]types.ColdIndexEntry, int, error)
	GetColdIndexEntry(context.Context, string) (types.ColdIndexEntry, bool, error)
}

type ContextHeadSummaryWorker interface {
	Enqueue(context.Context, *Engine, Input)
	Wait()
}

type Engine struct {
	model                         model.StreamingClient
	registry                      *tools.Registry
	permission                    *permissions.Engine
	store                         ConversationStore
	ctxManager                    *contextstate.Manager
	compactor                     contextstate.Compactor
	archiver                      *contextstate.ArchiveCompactor
	runtime                       *contextstate.Runtime
	meta                          RuntimeMetadata
	basePrompt                    string
	globalConfigRoot              string
	maxWorkspacePromptBytes       int
	activeSkillTokenBudget        int
	maxToolSteps                  int
	maxToolResultStoreBytes       int
	automationService             tools.AutomationService
	roleService                   tools.RoleService
	sessionDelegationService      session.RoleDelegationService
	taskManager                   *task.Manager
	runtimeService                *runtimegraph.Service
	schedulerService              *scheduler.Service
	contextHeadSummaryAsync       bool
	contextHeadSummaryWorker      ContextHeadSummaryWorker
	contextHeadSummaryWG          sync.WaitGroup
	compactionFailureMu           sync.Mutex
	consecutiveCompactionFailures int
	contextHeadSummaryMu          sync.Mutex
	contextHeadSummaryRunning     map[string]bool
	contextHeadSummaryPending     map[string]Input
}

const defaultActiveSkillTokenBudget = 2048
const defaultMaxToolResultStoreBytes = 16384

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
		model:                   modelClient,
		registry:                registry,
		permission:              permission,
		store:                   store,
		ctxManager:              ctxManager,
		runtime:                 runtime,
		compactor:               compactor,
		meta:                    meta,
		activeSkillTokenBudget:  defaultActiveSkillTokenBudget,
		maxToolSteps:            maxToolSteps,
		maxToolResultStoreBytes: defaultMaxToolResultStoreBytes,
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

func (e *Engine) SetGlobalConfigRoot(root string) {
	if e == nil {
		return
	}
	e.globalConfigRoot = root
}

func (e *Engine) SetMaxWorkspacePromptBytes(n int) {
	if e == nil {
		return
	}
	e.maxWorkspacePromptBytes = n
}

func (e *Engine) SetActiveSkillTokenBudget(n int) {
	if e == nil {
		return
	}
	e.activeSkillTokenBudget = n
}

func (e *Engine) SetArchiver(archiver *contextstate.ArchiveCompactor) {
	if e == nil {
		return
	}
	e.archiver = archiver
}

func (e *Engine) SetMaxToolResultStoreBytes(n int) {
	if e == nil || n <= 0 {
		return
	}
	e.maxToolResultStoreBytes = n
}

func (e *Engine) SetTaskManager(manager *task.Manager) {
	if e == nil {
		return
	}
	e.taskManager = manager
}

func (e *Engine) SetAutomationService(service tools.AutomationService) {
	if e == nil {
		return
	}
	e.automationService = service
}

func (e *Engine) SetRoleService(service tools.RoleService) {
	if e == nil {
		return
	}
	e.roleService = service
}

func (e *Engine) SetSessionDelegationService(service session.RoleDelegationService) {
	if e == nil {
		return
	}
	e.sessionDelegationService = service
}

func (e *Engine) SetRuntimeService(service *runtimegraph.Service) {
	if e == nil {
		return
	}
	e.runtimeService = service
}

func (e *Engine) SetSchedulerService(service *scheduler.Service) {
	if e == nil {
		return
	}
	e.schedulerService = service
}

func (e *Engine) compactionCircuitOpen() bool {
	if e == nil {
		return false
	}
	e.compactionFailureMu.Lock()
	defer e.compactionFailureMu.Unlock()
	return e.consecutiveCompactionFailures >= 3
}

func (e *Engine) recordSummaryCompactionResult(err error) {
	if e == nil {
		return
	}
	e.compactionFailureMu.Lock()
	defer e.compactionFailureMu.Unlock()
	if err != nil {
		e.consecutiveCompactionFailures++
		return
	}
	e.consecutiveCompactionFailures = 0
}

func (e *Engine) resetCompactionCircuit() {
	if e == nil {
		return
	}
	e.compactionFailureMu.Lock()
	defer e.compactionFailureMu.Unlock()
	e.consecutiveCompactionFailures = 0
}

func (e *Engine) SetContextHeadSummaryAsync(enabled bool) {
	if e == nil {
		return
	}
	e.contextHeadSummaryAsync = enabled
	if enabled && e.contextHeadSummaryWorker == nil {
		e.contextHeadSummaryWorker = NewInProcessContextHeadSummaryWorker()
	}
	if !enabled {
		e.contextHeadSummaryWorker = nil
	}
}

func (e *Engine) SetContextHeadSummaryWorker(worker ContextHeadSummaryWorker) {
	if e == nil {
		return
	}
	e.contextHeadSummaryWorker = worker
	if worker != nil {
		e.contextHeadSummaryAsync = true
	}
}

func (e *Engine) waitBackgroundTasks() {
	if e == nil {
		return
	}
	if e.contextHeadSummaryWorker != nil {
		e.contextHeadSummaryWorker.Wait()
	}
	e.contextHeadSummaryWG.Wait()
}
