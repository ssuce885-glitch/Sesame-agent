package daemon

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	httpapi "go-agent/internal/api/http"
	"go-agent/internal/automation"
	"go-agent/internal/config"
	contextstate "go-agent/internal/context"
	"go-agent/internal/engine"
	"go-agent/internal/model"
	"go-agent/internal/permissions"
	rolectx "go-agent/internal/roles"
	"go-agent/internal/scheduler"
	"go-agent/internal/session"
	"go-agent/internal/store/sqlite"
	"go-agent/internal/stream"
	"go-agent/internal/tools"
	"go-agent/internal/types"
)

type runtimeWiring struct {
	contextManagerConfig contextstate.Config
	runtime              *contextstate.Runtime
	compactor            contextstate.Compactor
	archiver             *contextstate.ArchiveCompactor
}

func (s combinedEventSink) Emit(ctx context.Context, event types.Event) error {
	if s.primary != nil {
		if err := s.primary.Emit(ctx, event); err != nil {
			return err
		}
	}
	if s.observer != nil {
		if err := s.observer.Emit(ctx, event); err != nil {
			return err
		}
	}
	return nil
}

func (s combinedEventSink) FinalizeTurn(ctx context.Context, usage *types.TurnUsage, events []types.Event) error {
	if s.finalizer == nil {
		return nil
	}
	return s.finalizer.FinalizeTurn(ctx, usage, events)
}

func ensureDataDir(path string) error {
	return os.MkdirAll(path, 0o755)
}

func writePIDFile(path string, daemonID string, fingerprint string) error {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	raw, err := json.Marshal(map[string]any{
		"pid":                os.Getpid(),
		"daemon_id":          strings.TrimSpace(daemonID),
		"config_fingerprint": strings.TrimSpace(fingerprint),
	})
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0o644)
}

func configureRuntimeGuardrails(cfg config.Config) {
	tools.SetShellCommandGuardrails(cfg.MaxShellOutputBytes, cfg.ShellTimeoutSeconds)
	tools.SetFileWriteMaxBytes(cfg.MaxFileWriteBytes)
}

func buildHTTPDependencies(cfg config.Config, store *sqlite.Store, bus *stream.Bus, manager *session.Manager, schedulerService *scheduler.Service, automationService *automation.Service, fileCheckpoints *engine.FileCheckpointService) httpapi.Dependencies {
	if automationService == nil && store != nil {
		automationService = automation.NewService(store)
	}
	if fileCheckpoints == nil && store != nil {
		fileCheckpoints = engine.NewFileCheckpointService(store, cfg.Paths.WorkspaceRoot)
	}
	roleService := rolectx.NewServiceWithGlobalRoot(cfg.Paths.GlobalRoot)
	roleService.SetAutomationCleanupService(automationService)
	return httpapi.Dependencies{
		Bus:             bus,
		Store:           store,
		Manager:         manager,
		Scheduler:       schedulerService,
		Automation:      automationService,
		RoleService:     roleService,
		FileCheckpoints: fileCheckpoints,
		Status:          buildStatusPayload(cfg),
		ConsoleRoot:     filepath.Join("web", "console", "dist"),
		WorkspaceRoot:   cfg.Paths.WorkspaceRoot,
	}
}

func buildPermissionEngine(cfg config.Config) *permissions.Engine {
	return permissions.NewEngine(cfg.PermissionProfile)
}

func buildContextManagerConfig(cfg config.Config) contextstate.Config {
	return contextstate.Config{
		MaxRecentItems:             cfg.MaxRecentItems,
		MaxEstimatedTokens:         cfg.MaxEstimatedTokens,
		ModelContextWindow:         cfg.ModelContextWindow,
		CompactionThreshold:        cfg.CompactionThreshold,
		MicrocompactBytesThreshold: cfg.MicrocompactBytesThreshold,
		MaxCompactionBatchItems:    cfg.MaxCompactionBatchItems,
	}
}

func buildMaxToolSteps(cfg config.Config) int {
	return cfg.MaxToolSteps
}

func buildStatusPayload(cfg config.Config) httpapi.StatusPayload {
	return httpapi.StatusPayload{
		Provider:             cfg.ModelProvider,
		Model:                cfg.Model,
		PermissionProfile:    cfg.PermissionProfile,
		ProviderCacheProfile: cfg.ProviderCacheProfile,
		PID:                  os.Getpid(),
	}
}

func buildRuntimeWiring(cfg config.Config, modelClient model.StreamingClient) runtimeWiring {
	return runtimeWiring{
		contextManagerConfig: buildContextManagerConfig(cfg),
		runtime:              contextstate.NewRuntime(cfg.CacheExpirySeconds, cfg.MaxCompactionPasses),
		compactor:            contextstate.NewPromptedCompactor(modelClient, cfg.Model),
		archiver:             contextstate.NewArchiveCompactor(modelClient, cfg.Model),
	}
}
