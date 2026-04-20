package daemon

import (
	"context"
	"os"
	"path/filepath"

	"go-agent/internal/automation"
	"go-agent/internal/config"
	contextstate "go-agent/internal/context"
	"go-agent/internal/engine"
	"go-agent/internal/model"
	"go-agent/internal/reporting"
	rolectx "go-agent/internal/roles"
	"go-agent/internal/runtimegraph"
	"go-agent/internal/scheduler"
	"go-agent/internal/session"
	"go-agent/internal/store/sqlite"
	"go-agent/internal/stream"
	"go-agent/internal/task"
	"go-agent/internal/tools"
)

func buildRuntime(_ context.Context, cfg config.Config, store *sqlite.Store, modelClient model.StreamingClient) (*Runtime, error) {
	bus := stream.NewBus()
	runtimeService := runtimegraph.NewService(store)
	automationService := automation.NewService(store)

	wiring := buildRuntimeWiring(cfg, modelClient)
	runner := engine.NewWithRuntime(
		modelClient,
		toolsRegistry(),
		buildPermissionEngine(cfg),
		store,
		contextstate.NewManager(wiring.contextManagerConfig),
		wiring.runtime,
		wiring.compactor,
		engine.RuntimeMetadata{
			Provider: cfg.ModelProvider,
			Model:    cfg.Model,
		},
		buildMaxToolSteps(cfg),
	)
	runner.SetGlobalConfigRoot(cfg.Paths.GlobalRoot)
	runner.SetHeadMemoryAsync(true)
	runner.SetMaxWorkspacePromptBytes(cfg.MaxWorkspacePromptBytes)
	runner.SetRuntimeService(runtimeService)
	runner.SetAutomationService(automationService)
	runner.SetRoleService(rolectx.NewServiceWithGlobalRoot(cfg.Paths.GlobalRoot))

	taskNotifier := buildTaskTerminalNotifier(store, bus, cfg.Paths.WorkspaceRoot)
	var deliveryService *automation.DeliveryService
	if taskNotifier != nil {
		deliveryService = automation.NewDeliveryService(store, taskNotifier.reporting, nil)
	} else {
		deliveryService = automation.NewDeliveryService(store, nil, nil)
	}
	agentExecutor := buildAgentTaskExecutor(runner, store)
	taskManager := task.NewManager(task.Config{
		MaxConcurrentTasks: cfg.MaxConcurrentTasks,
		TaskOutputMaxBytes: cfg.TaskOutputMaxBytes,
		TerminalNotifier:   taskNotifier,
		WorkspaceStore:     store,
	}, nil, agentExecutor)
	executablePath, _ := os.Executable()
	watcherService := automation.NewWatcherService(store, taskManager, automation.WatcherConfig{
		DataRoot:       filepath.Join(cfg.DataDir, "automation"),
		ExecutablePath: executablePath,
		DataDir:        cfg.DataDir,
		Addr:           cfg.Addr,
	})
	automationService.SetWatcherService(watcherService)

	schedulerService := scheduler.NewService(store, taskManager)
	taskManager.SetRemoteConfig(task.RemoteExecutorConfig{
		ShimCommand:    cfg.RemoteExecutorShimCommand,
		TimeoutSeconds: cfg.RemoteExecutorTimeoutSeconds,
	})
	if taskNotifier != nil {
		taskNotifier.scheduler = schedulerService
		taskNotifier.delivery = deliveryService
		taskNotifier.watcher = watcherService
	}
	runner.SetTaskManager(taskManager)
	runner.SetSchedulerService(schedulerService)

	sessionManager := session.NewManager(sessionRunnerAdapter{
		engine:   runner,
		store:    store,
		delivery: deliveryService,
		watcher:  watcherService,
		tasker:   taskManager,
		notifier: taskNotifier,
		sink: storeAndBusSink{
			store: store,
			bus:   bus,
		},
	}, newTurnResultFallbackSink(store, bus))
	if agentExecutor != nil {
		agentExecutor.manager = sessionManager
	}
	if taskNotifier != nil {
		taskNotifier.manager = sessionManager
	}
	runner.SetSessionDelegationService(session.NewDelegationService(store, taskManager))

	reportingService := reporting.NewService(store)
	if taskNotifier != nil && taskNotifier.reporting != nil {
		reportingService = taskNotifier.reporting
	}

	return &Runtime{
		Store:             store,
		Bus:               bus,
		Engine:            runner,
		SessionManager:    sessionManager,
		TaskManager:       taskManager,
		RuntimeService:    runtimeService,
		AutomationService: automationService,
		WatcherService:    watcherService,
		SchedulerService:  schedulerService,
		ReportingService:  reportingService,
		TaskNotifier:      taskNotifier,
	}, nil
}

func toolsRegistry() *tools.Registry {
	return tools.NewRegistry()
}
