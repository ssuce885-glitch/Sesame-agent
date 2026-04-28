package daemon

import (
	"context"
	"os"
	"path/filepath"

	"go-agent/internal/automation"
	"go-agent/internal/config"
	"go-agent/internal/connectors/discord"
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

type daemonDiscordConnector interface {
	Start(context.Context) error
	Close() error
}

type discordBindingLoader func(workspaceRoot string) (discord.WorkspaceBinding, error)
type discordConnectorFactory func(cfg discord.ServiceConfig) (daemonDiscordConnector, error)

func startDiscordConnectorIfConfigured(ctx context.Context, cfg config.Config, userCfg config.UserConfig, runtime *Runtime, loadBinding discordBindingLoader, factory discordConnectorFactory) (daemonDiscordConnector, error) {
	connector, err := buildDiscordConnector(cfg, userCfg, runtime, loadBinding, factory)
	if err != nil || connector == nil {
		return connector, err
	}

	if err := connector.Start(ctx); err != nil {
		_ = connector.Close()
		return nil, err
	}
	return connector, nil
}

func buildDiscordConnector(cfg config.Config, userCfg config.UserConfig, runtime *Runtime, loadBinding discordBindingLoader, factory discordConnectorFactory) (daemonDiscordConnector, error) {
	if !userCfg.Discord.Enabled {
		return nil, nil
	}
	if runtime == nil || runtime.Store == nil || runtime.SessionManager == nil || runtime.Bus == nil {
		return nil, nil
	}

	if loadBinding == nil {
		loadBinding = discord.LoadWorkspaceBinding
	}
	if factory == nil {
		factory = func(cfg discord.ServiceConfig) (daemonDiscordConnector, error) {
			return discord.NewService(cfg)
		}
	}

	binding, err := loadBinding(cfg.Paths.WorkspaceRoot)
	if err != nil {
		return nil, err
	}
	if !binding.Enabled {
		return nil, nil
	}

	return factory(discord.ServiceConfig{
		Global: discord.GlobalConfig{
			Enabled:              userCfg.Discord.Enabled,
			BotToken:             userCfg.Discord.BotToken,
			BotTokenEnv:          userCfg.Discord.BotTokenEnv,
			GatewayIntents:       append([]string(nil), userCfg.Discord.GatewayIntents...),
			MessageContentIntent: userCfg.Discord.MessageContentIntent,
			LogIgnoredMessages:   userCfg.Discord.LogIgnoredMessages,
		},
		Binding:       binding,
		WorkspaceRoot: cfg.Paths.WorkspaceRoot,
		DB:            runtime.Store.DB(),
		RuntimeStore:  runtime.Store,
		Manager:       runtime.SessionManager,
		EventStore:    runtime.Store,
		Bus:           runtime.Bus,
	})
}

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
	runner.SetArchiver(wiring.archiver)
	runner.SetContextHeadSummaryAsync(true)
	runner.SetMaxWorkspacePromptBytes(cfg.MaxWorkspacePromptBytes)
	runner.SetMaxToolResultStoreBytes(cfg.MaxToolResultStoreBytes)
	runner.SetRuntimeService(runtimeService)
	runner.SetAutomationService(automationService)
	roleService := rolectx.NewServiceWithGlobalRoot(cfg.Paths.GlobalRoot)
	roleService.SetAutomationCleanupService(automationService)
	runner.SetRoleService(roleService)

	taskNotifier := buildTaskTerminalNotifier(store, bus, cfg.Paths.WorkspaceRoot)
	agentExecutor := buildAgentTaskExecutor(runner, store)
	taskManager := task.NewManager(task.Config{
		MaxConcurrentTasks: cfg.MaxConcurrentTasks,
		TaskOutputMaxBytes: cfg.TaskOutputMaxBytes,
		TerminalNotifier:   taskNotifier,
		WorkspaceStore:     store,
	}, nil, agentExecutor)
	automationService.SetSimpleRuntime(automation.NewSimpleRuntime(store, taskManager, automation.SimpleRuntimeConfig{}))
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
		taskNotifier.watcher = watcherService
	}
	runner.SetTaskManager(taskManager)
	runner.SetSchedulerService(schedulerService)

	sessionManager := session.NewManager(sessionRunnerAdapter{
		engine:   runner,
		store:    store,
		tasker:   taskManager,
		notifier: taskNotifier,
		sink: storeAndBusSink{
			store: store,
			bus:   bus,
		},
	}, newTurnResultFallbackSink(store, bus, taskNotifier))
	if agentExecutor != nil {
		agentExecutor.manager = sessionManager
	}
	if taskNotifier != nil {
		taskNotifier.manager = sessionManager
		sessionManager.SetIdleNotifier(func(sessionID string) {
			_ = taskNotifier.EnqueueSyntheticReportTurn(context.Background(), sessionID)
		})
	}
	runner.SetSessionDelegationService(session.NewDelegationService(store, taskManager))

	reportingService := reporting.NewService(store)
	if taskNotifier != nil && taskNotifier.reporting != nil {
		reportingService = taskNotifier.reporting
	}
	reportingService.SetColdStore(store)
	reportingService.SetCleanupStore(store)
	reportingService.SetWorkspaceRoot(cfg.Paths.WorkspaceRoot)

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
