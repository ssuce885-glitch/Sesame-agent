package discord

import (
	"context"
	"errors"
	"sync"
)

// Gateway abstracts the Discord gateway transport for the connector service.
type Gateway interface {
	Start(context.Context) error
	Close() error
}

// GatewayConfig captures static config passed into a gateway implementation.
type GatewayConfig struct {
	Global  GlobalConfig
	Binding WorkspaceBinding
}

type stubGateway struct {
	mu      sync.Mutex
	started bool
	closed  bool
}

// NewGateway returns a minimal placeholder gateway for V1 wiring.
func NewGateway(GatewayConfig) (Gateway, error) {
	return &stubGateway{}, nil
}

func (g *stubGateway) Start(_ context.Context) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.closed {
		return errors.New("discord gateway is closed")
	}
	g.started = true
	return nil
}

func (g *stubGateway) Close() error {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.started = false
	g.closed = true
	return nil
}
