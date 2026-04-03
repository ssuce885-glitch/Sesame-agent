package engine

import (
	"context"

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

type Engine struct {
	model      model.StreamingClient
	registry   *tools.Registry
	permission *permissions.Engine
}

func New(modelClient model.StreamingClient, registry *tools.Registry, permission *permissions.Engine) *Engine {
	return &Engine{model: modelClient, registry: registry, permission: permission}
}

func (e *Engine) RunTurn(ctx context.Context, in Input) error {
	return runLoop(ctx, e.model, e.registry, e.permission, in)
}
