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
	Resume              *types.TurnResume
	ActivatedSkillNames []string
}

type RuntimeMetadata struct {
	Provider string
	Model    string
}

type ConversationStore interface {
	GetCurrentContextHeadID(context.Context) (string, bool, error)
	ListConversationItems(context.Context, string) ([]model.ConversationItem, error)
	ListConversationTimelineItemsByContextHead(context.Context, string, string) ([]types.ConversationTimelineItem, error)
	ListConversationItemsByContextHead(context.Context, string, string) ([]model.ConversationItem, error)
	ListConversationCompactionsByStoredContextHead(context.Context, string, string) ([]types.ConversationCompaction, error)
	GetHeadMemory(context.Context, string, string) (types.HeadMemory, bool, error)
	InsertConversationItem(context.Context, string, string, int, model.ConversationItem) error
	InsertConversationItemWithContextHead(context.Context, string, string, string, int, model.ConversationItem) error
	GetConversationItemIDByContextHeadAndPosition(context.Context, string, string, int) (int64, bool, error)
	UpsertTurnUsage(context.Context, types.TurnUsage) error
	UpsertHeadMemory(context.Context, types.HeadMemory) error
	UpsertMemoryEntry(context.Context, types.MemoryEntry) error
	DeleteMemoryEntries(context.Context, []string) error
	ListMemoryEntriesByWorkspace(context.Context, string) ([]types.MemoryEntry, error)
	GetProviderCacheHead(context.Context, string, string, string) (types.ProviderCacheHead, bool, error)
	UpsertProviderCacheHead(context.Context, types.ProviderCacheHead) error
	InsertProviderCacheEntry(context.Context, types.ProviderCacheEntry) error
	InsertConversationCompaction(context.Context, types.ConversationCompaction) error
	InsertConversationCompactionWithContextHead(context.Context, types.ConversationCompaction) error
}

type HeadMemoryWorker interface {
	Enqueue(context.Context, *Engine, Input)
	Wait()
}

type Engine struct {
	model                    model.StreamingClient
	registry                 *tools.Registry
	permission               *permissions.Engine
	store                    ConversationStore
	ctxManager               *contextstate.Manager
	compactor                contextstate.Compactor
	runtime                  *contextstate.Runtime
	meta                     RuntimeMetadata
	basePrompt               string
	globalConfigRoot         string
	maxWorkspacePromptBytes  int
	activeSkillTokenBudget   int
	maxToolSteps             int
	automationService        tools.AutomationService
	roleService              tools.RoleService
	sessionDelegationService session.RoleDelegationService
	taskManager              *task.Manager
	runtimeService           *runtimegraph.Service
	schedulerService         *scheduler.Service
	headMemoryAsync          bool
	headMemoryWorker         HeadMemoryWorker
	headMemoryWG             sync.WaitGroup
	headMemoryMu             sync.Mutex
	headMemoryRunning        map[string]bool
	headMemoryPending        map[string]Input
}

const defaultActiveSkillTokenBudget = 2048

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
		model:                  modelClient,
		registry:               registry,
		permission:             permission,
		store:                  store,
		ctxManager:             ctxManager,
		runtime:                runtime,
		compactor:              compactor,
		meta:                   meta,
		activeSkillTokenBudget: defaultActiveSkillTokenBudget,
		maxToolSteps:           maxToolSteps,
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

func (e *Engine) SetHeadMemoryAsync(enabled bool) {
	if e == nil {
		return
	}
	e.headMemoryAsync = enabled
	if enabled && e.headMemoryWorker == nil {
		e.headMemoryWorker = NewInProcessHeadMemoryWorker()
	}
	if !enabled {
		e.headMemoryWorker = nil
	}
}

func (e *Engine) SetHeadMemoryWorker(worker HeadMemoryWorker) {
	if e == nil {
		return
	}
	e.headMemoryWorker = worker
	if worker != nil {
		e.headMemoryAsync = true
	}
}

func (e *Engine) waitBackgroundTasks() {
	if e == nil {
		return
	}
	if e.headMemoryWorker != nil {
		e.headMemoryWorker.Wait()
	}
	e.headMemoryWG.Wait()
}
