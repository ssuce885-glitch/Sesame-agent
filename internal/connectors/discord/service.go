package discord

import (
	"context"
	"errors"
	"sync"
)

// Connector defines daemon lifecycle operations for the Discord connector.
type Connector interface {
	Start(context.Context) error
	Close() error
}

// ServiceConfig describes daemon wiring inputs for the connector.
type ServiceConfig struct {
	Global  GlobalConfig
	Binding WorkspaceBinding
	Gateway Gateway
}

// Service is the V1 daemon-facing connector lifecycle shell.
type Service struct {
	mu      sync.Mutex
	gateway Gateway
	started bool
	closed  bool
}

func NewService(cfg ServiceConfig) (*Service, error) {
	gateway := cfg.Gateway
	if gateway == nil {
		var err error
		gateway, err = NewGateway(GatewayConfig{
			Global:  cfg.Global,
			Binding: cfg.Binding,
		})
		if err != nil {
			return nil, err
		}
	}
	return &Service{gateway: gateway}, nil
}

func (s *Service) Start(ctx context.Context) error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return errors.New("discord connector is closed")
	}
	if s.started {
		s.mu.Unlock()
		return nil
	}
	s.mu.Unlock()

	if err := s.gateway.Start(ctx); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.started = true
	return nil
}

func (s *Service) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	s.started = false
	s.mu.Unlock()
	return s.gateway.Close()
}

var _ Connector = (*Service)(nil)
