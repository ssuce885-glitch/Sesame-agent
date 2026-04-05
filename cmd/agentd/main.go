package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"

	httpapi "go-agent/internal/api/http"
	"go-agent/internal/config"
	contextstate "go-agent/internal/context"
	"go-agent/internal/engine"
	"go-agent/internal/model"
	"go-agent/internal/permissions"
	"go-agent/internal/session"
	"go-agent/internal/store/artifacts"
	"go-agent/internal/store/sqlite"
	"go-agent/internal/stream"
	"go-agent/internal/tools"
	"go-agent/internal/types"
)

type sessionRunnerAdapter struct {
	engine *engine.Engine
	sink   engine.EventSink
}

type storeAndBusSink struct {
	store *sqlite.Store
	bus   *stream.Bus
}

type runtimeWiring struct {
	contextManagerConfig contextstate.Config
	runtime              *contextstate.Runtime
	compactor            contextstate.Compactor
}

func (s storeAndBusSink) Emit(ctx context.Context, event types.Event) error {
	seq, err := s.store.AppendEvent(ctx, event)
	if err != nil {
		return err
	}
	event.Seq = seq
	s.bus.Publish(event)
	return nil
}

func (s storeAndBusSink) FinalizeTurn(ctx context.Context, usage *types.TurnUsage, events []types.Event) error {
	persisted, err := s.store.FinalizeTurn(ctx, usage, events)
	if err != nil {
		return err
	}
	for _, event := range persisted {
		s.bus.Publish(event)
	}
	return nil
}

func (a sessionRunnerAdapter) RunTurn(ctx context.Context, in session.RunInput) error {
	err := a.engine.RunTurn(ctx, engine.Input{
		Session: in.Session,
		Turn: types.Turn{
			ID:           in.TurnID,
			SessionID:    in.Session.ID,
			ClientTurnID: "",
			UserMessage:  in.Message,
		},
		Sink: a.sink,
	})
	return err
}

func ensureDataDir(path string) error {
	return os.MkdirAll(path, 0o755)
}

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("load config", "err", err)
		os.Exit(1)
	}

	if err := ensureDataDir(cfg.DataDir); err != nil {
		slog.Error("prepare data dir", "err", err, "path", cfg.DataDir)
		os.Exit(1)
	}

	store, err := sqlite.Open(filepath.Join(cfg.DataDir, "agentd.db"))
	if err != nil {
		slog.Error("open sqlite store", "err", err)
		os.Exit(1)
	}
	defer store.Close()

	_, err = artifacts.New(filepath.Join(cfg.DataDir, "artifacts"))
	if err != nil {
		slog.Error("open artifact store", "err", err)
		os.Exit(1)
	}

	bus := stream.NewBus()
	registry := tools.NewRegistry()
	configureRuntimeGuardrails(cfg)
	permissionEngine := buildPermissionEngine(cfg)
	modelClient, err := model.NewFromConfig(cfg)
	if err != nil {
		slog.Error("build model client", "err", err)
		os.Exit(1)
	}
	wiring := buildRuntimeWiring(cfg, modelClient)
	runner := engine.NewWithRuntime(
		modelClient,
		registry,
		permissionEngine,
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
	runner.SetBaseSystemPrompt(cfg.SystemPrompt)
	manager := session.NewManager(sessionRunnerAdapter{
		engine: runner,
		sink: storeAndBusSink{
			store: store,
			bus:   bus,
		},
	})
	if err := recoverRuntimeState(context.Background(), store, manager); err != nil {
		slog.Error("recover runtime state", "err", err)
		os.Exit(1)
	}

	handler := httpapi.NewRouter(httpapi.Dependencies{
		Bus:         bus,
		Store:       store,
		Manager:     manager,
		Status:      buildStatusPayload(cfg),
		ConsoleRoot: filepath.Join("web", "console", "dist"),
	})

	slog.Info("agentd listening", "addr", cfg.Addr)
	if err := http.ListenAndServe(cfg.Addr, handler); err != nil {
		slog.Error("listen", "err", err)
		os.Exit(1)
	}
}

func configureRuntimeGuardrails(cfg config.Config) {
	tools.SetShellCommandGuardrails(cfg.MaxShellOutputBytes, cfg.ShellTimeoutSeconds)
	tools.SetFileWriteMaxBytes(cfg.MaxFileWriteBytes)
}

func buildPermissionEngine(cfg config.Config) *permissions.Engine {
	return permissions.NewEngine(cfg.PermissionProfile)
}

func buildContextManagerConfig(cfg config.Config) contextstate.Config {
	return contextstate.Config{
		MaxRecentItems:             cfg.MaxRecentItems,
		MaxEstimatedTokens:         cfg.MaxEstimatedTokens,
		CompactionThreshold:        cfg.CompactionThreshold,
		MicrocompactBytesThreshold: cfg.MicrocompactBytesThreshold,
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
	}
}

func buildRuntimeWiring(cfg config.Config, modelClient model.StreamingClient) runtimeWiring {
	return runtimeWiring{
		contextManagerConfig: buildContextManagerConfig(cfg),
		runtime:              contextstate.NewRuntime(cfg.CacheExpirySeconds, cfg.MaxCompactionPasses),
		compactor:            contextstate.NewPromptedCompactor(modelClient, cfg.Model),
	}
}

func recoverRuntimeState(ctx context.Context, store *sqlite.Store, manager *session.Manager) error {
	sessions, err := store.ListSessions(ctx)
	if err != nil {
		return err
	}
	if manager != nil {
		for _, sessionRow := range sessions {
			manager.RegisterSession(sessionRow)
		}
	}
	if err := ensureSelectedSession(ctx, store, sessions); err != nil {
		return err
	}

	running, err := store.ListRunningTurns(ctx)
	if err != nil {
		return err
	}

	for _, turn := range running {
		if err := store.MarkTurnInterrupted(ctx, turn.ID); err != nil {
			return err
		}

		event, err := types.NewEvent(turn.SessionID, turn.ID, types.EventTurnInterrupted, map[string]string{
			"reason": "daemon_restart",
		})
		if err != nil {
			return err
		}
		if _, err := store.AppendEvent(ctx, event); err != nil {
			return err
		}
	}

	return nil
}

func ensureSelectedSession(ctx context.Context, store *sqlite.Store, sessions []types.Session) error {
	if len(sessions) == 0 {
		return nil
	}

	selected, ok, err := store.GetSelectedSessionID(ctx)
	if err != nil {
		return err
	}
	if ok {
		for _, sessionRow := range sessions {
			if sessionRow.ID == selected {
				return nil
			}
		}
	}

	return store.SetSelectedSessionID(ctx, sessions[0].ID)
}
