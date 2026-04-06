package runtimegraph

import (
	"context"

	"go-agent/internal/store/sqlite"
)

type TurnContext struct {
	CurrentSessionID string
	CurrentTurnID    string
	CurrentRunID     string
	CurrentTaskID    string
}

type RuntimeTx = sqlite.RuntimeTx

type Store interface {
	WithTx(context.Context, func(tx RuntimeTx) error) error
}

type Service struct {
	store Store
}

func NewService(store Store) *Service {
	return &Service{store: store}
}
