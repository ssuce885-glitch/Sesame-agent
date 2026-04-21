package daemon

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	httpapi "go-agent/internal/api/http"
	"go-agent/internal/automation"
	"go-agent/internal/config"
	"go-agent/internal/model"
	"go-agent/internal/store/artifacts"
	"go-agent/internal/store/sqlite"
)

func Run(ctx context.Context) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	userCfg, err := config.LoadUserConfigFromGlobalRoot(cfg.Paths.GlobalRoot)
	if err != nil {
		return err
	}
	if missing := config.MissingSetupFields(cfg); len(missing) > 0 {
		configPath, _ := config.GlobalConfigPath()
		return fmt.Errorf("sesame daemon is not configured: missing %s in %s", strings.Join(missing, ", "), configPath)
	}

	basePrompt, err := cfg.ResolveSystemPrompt()
	if err != nil {
		return err
	}

	if err := ensureDataDir(cfg.DataDir); err != nil {
		return err
	}
	if err := writePIDFile(cfg.Paths.PIDFile, cfg.DaemonID, cfg.ConfigFingerprint); err != nil {
		return err
	}
	defer os.Remove(cfg.Paths.PIDFile)

	store, err := sqlite.Open(cfg.Paths.DatabaseFile)
	if err != nil {
		return err
	}
	defer store.Close()

	_, err = artifacts.New(filepath.Join(cfg.DataDir, "artifacts"))
	if err != nil {
		return err
	}

	configureRuntimeGuardrails(cfg)
	modelClient, err := model.NewFromConfig(cfg)
	if err != nil {
		return err
	}
	runtime, err := buildRuntime(ctx, cfg, store, modelClient)
	if err != nil {
		return err
	}
	if err := validateRuntime(runtime); err != nil {
		return err
	}
	if runtime.Engine != nil {
		runtime.Engine.SetBaseSystemPrompt(basePrompt)
	}

	if err := recoverRuntimeState(ctx, runtime.Store, runtime.SessionManager); err != nil {
		return err
	}

	discordConnector, err := startDiscordConnectorIfConfigured(ctx, cfg, userCfg, nil, nil)
	if err != nil {
		return err
	}
	if discordConnector != nil {
		defer func() {
			if closeErr := discordConnector.Close(); closeErr != nil {
				slog.Error("discord connector close failed", "error", closeErr)
			}
		}()
	}

	dispatcher := automation.NewDispatcher(runtime.Store, automationTaskLauncher{
		store:   runtime.Store,
		manager: runtime.TaskManager,
	}, automation.DispatcherConfig{Watcher: runtime.WatcherService})
	go func() {
		if err := runtime.SchedulerService.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
			slog.Error("scheduler loop exited", "error", err)
		}
	}()
	if runtime.WatcherService != nil {
		go func() {
			runSupervisedLoop(ctx, "watcher", runtime.WatcherService.ReconcileInterval(), runtime.WatcherService.Reconcile, func(_ context.Context, err error) {
				slog.Error("watcher tick failed", "error", err)
			})
		}()
	}
	if runtime.ReportingService != nil {
		go func() {
			runSupervisedLoop(ctx, "reporting", runtime.ReportingService.PollInterval(), runtime.ReportingService.Tick, func(_ context.Context, err error) {
				slog.Error("reporting tick failed", "error", err)
			})
		}()
	}
	go func() {
		runSupervisedLoop(ctx, "automation_dispatcher", time.Second, dispatcher.Tick, func(_ context.Context, err error) {
			slog.Error("automation dispatcher tick failed", "error", err)
		})
	}()

	handler := httpapi.NewRouter(buildHTTPDependencies(cfg, runtime.Store, runtime.Bus, runtime.SessionManager, runtime.SchedulerService, runtime.AutomationService))

	slog.Info("sesame daemon listening", "addr", cfg.Addr)
	return http.ListenAndServe(cfg.Addr, handler)
}
