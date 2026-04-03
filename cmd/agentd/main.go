package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"

	httpapi "go-agent/internal/api/http"
	"go-agent/internal/config"
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

func (s storeAndBusSink) Emit(ctx context.Context, event types.Event) error {
	seq, err := s.store.AppendEvent(ctx, event)
	if err != nil {
		return err
	}
	event.Seq = seq
	s.bus.Publish(event)
	return nil
}

func (a sessionRunnerAdapter) RunTurn(ctx context.Context, in session.RunInput) error {
	err := a.engine.RunTurn(ctx, engine.Input{
		Session: in.Session,
		Turn: types.Turn{
			ID:           in.TurnID,
			ClientTurnID: "",
			UserMessage:  in.Message,
		},
		Sink: a.sink,
	})
	return err
}

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("load config", "err", err)
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
	permissionEngine := permissions.NewEngine()
	fakeModel := model.NewFakeStreaming(nil)
	runner := engine.New(fakeModel, registry, permissionEngine)
	manager := session.NewManager(sessionRunnerAdapter{
		engine: runner,
		sink: storeAndBusSink{
			store: store,
			bus:   bus,
		},
	})

	handler := httpapi.NewRouter(httpapi.Dependencies{
		Bus:     bus,
		Store:   store,
		Manager: manager,
	})

	slog.Info("agentd listening", "addr", cfg.Addr)
	if err := http.ListenAndServe(cfg.Addr, handler); err != nil {
		slog.Error("listen", "err", err)
		os.Exit(1)
	}
}
