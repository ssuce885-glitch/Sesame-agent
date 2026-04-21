package discord

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"sync"

	"go-agent/internal/types"
)

// Connector defines daemon lifecycle operations for the Discord connector.
type Connector interface {
	Start(context.Context) error
	Close() error
}

type sessionEventSubscriber interface {
	Subscribe(sessionID string) (<-chan types.Event, func())
}

type sessionEventStore interface {
	ListSessionEvents(ctx context.Context, sessionID string, afterSeq int64) ([]types.Event, error)
}

// ServiceConfig describes daemon wiring inputs for the connector.
type ServiceConfig struct {
	Global        GlobalConfig
	Binding       WorkspaceBinding
	WorkspaceRoot string
	DB            *sql.DB
	RuntimeStore  bridgeRuntimeStore
	Manager       bridgeRuntimeManager
	EventStore    sessionEventStore
	Bus           sessionEventSubscriber
	HTTPClient    *http.Client
	Gateway       Gateway
	Logger        *slog.Logger
}

// Service is the daemon-facing connector lifecycle shell.
type Service struct {
	mu            sync.Mutex
	gateway       Gateway
	bridge        Bridge
	logger        *slog.Logger
	workspaceRoot string
	started       bool
	closed        bool
	runCtx        context.Context
	cancel        context.CancelFunc
}

type eventReplyWaiter struct {
	store sessionEventStore
	bus   sessionEventSubscriber
}

func newEventReplyWaiter(store sessionEventStore, bus sessionEventSubscriber) parentReplyWaiter {
	return eventReplyWaiter{
		store: store,
		bus:   bus,
	}
}

func (w eventReplyWaiter) WaitParentReplyCommitted(ctx context.Context, sessionID, turnID string) (types.ParentReplyCommittedPayload, error) {
	if w.store == nil {
		return types.ParentReplyCommittedPayload{}, errors.New("discord parent reply event store is not configured")
	}
	if w.bus == nil {
		return types.ParentReplyCommittedPayload{}, errors.New("discord parent reply bus is not configured")
	}
	sessionID = strings.TrimSpace(sessionID)
	turnID = strings.TrimSpace(turnID)
	if sessionID == "" || turnID == "" {
		return types.ParentReplyCommittedPayload{}, errors.New("discord parent reply lookup requires session and turn IDs")
	}

	events, unsubscribe := w.bus.Subscribe(sessionID)
	defer unsubscribe()

	if payload, ok, err := w.findInStore(ctx, sessionID, turnID); err != nil {
		return types.ParentReplyCommittedPayload{}, err
	} else if ok {
		return payload, nil
	}

	for {
		select {
		case <-ctx.Done():
			return types.ParentReplyCommittedPayload{}, ctx.Err()
		case event, ok := <-events:
			if !ok {
				return types.ParentReplyCommittedPayload{}, errors.New("discord parent reply subscription closed")
			}
			payload, matched, err := decodeParentReplyEvent(event, turnID)
			if err != nil {
				return types.ParentReplyCommittedPayload{}, err
			}
			if matched {
				return payload, nil
			}
		}
	}
}

func (w eventReplyWaiter) findInStore(ctx context.Context, sessionID, turnID string) (types.ParentReplyCommittedPayload, bool, error) {
	events, err := w.store.ListSessionEvents(ctx, sessionID, 0)
	if err != nil {
		return types.ParentReplyCommittedPayload{}, false, err
	}
	for _, event := range events {
		payload, matched, err := decodeParentReplyEvent(event, turnID)
		if err != nil {
			return types.ParentReplyCommittedPayload{}, false, err
		}
		if matched {
			return payload, true, nil
		}
	}
	return types.ParentReplyCommittedPayload{}, false, nil
}

func decodeParentReplyEvent(event types.Event, turnID string) (types.ParentReplyCommittedPayload, bool, error) {
	if event.Type != types.EventParentReplyCommitted {
		return types.ParentReplyCommittedPayload{}, false, nil
	}
	if strings.TrimSpace(event.TurnID) != strings.TrimSpace(turnID) {
		return types.ParentReplyCommittedPayload{}, false, nil
	}

	var payload types.ParentReplyCommittedPayload
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return types.ParentReplyCommittedPayload{}, false, err
	}
	return payload, true, nil
}

func NewService(cfg ServiceConfig) (*Service, error) {
	if cfg.DB == nil {
		return nil, errors.New("discord connector DB is not configured")
	}
	if cfg.RuntimeStore == nil {
		return nil, errors.New("discord connector runtime store is not configured")
	}
	if cfg.Manager == nil {
		return nil, errors.New("discord connector runtime manager is not configured")
	}
	if cfg.EventStore == nil {
		return nil, errors.New("discord connector event store is not configured")
	}
	if cfg.Bus == nil {
		return nil, errors.New("discord connector event bus is not configured")
	}
	workspaceRoot := strings.TrimSpace(cfg.WorkspaceRoot)
	if workspaceRoot == "" {
		return nil, errors.New("discord connector workspace root is required")
	}

	token, err := resolveBotToken(cfg.Global)
	if err != nil {
		return nil, err
	}

	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}

	state := NewStateStore(cfg.DB)
	replies := newDiscordRESTPoster(token, httpClient, discordAPIBaseURL)

	svc := &Service{
		logger:        logger,
		workspaceRoot: workspaceRoot,
	}
	svc.bridge = Bridge{
		state:   state,
		store:   cfg.RuntimeStore,
		manager: cfg.Manager,
		waiter:  newEventReplyWaiter(cfg.EventStore, cfg.Bus),
		replies: replies,
		cfg:     cfg.Binding,
	}

	gateway := cfg.Gateway
	if gateway == nil {
		gateway, err = NewGateway(GatewayConfig{
			Global:        cfg.Global,
			Binding:       cfg.Binding,
			WorkspaceRoot: workspaceRoot,
			State:         state,
			Bridge:        &svc.bridge,
			ReplyPoster:   replies,
			HTTPClient:    httpClient,
			Logger:        logger,
		})
		if err != nil {
			return nil, err
		}
	}
	svc.gateway = gateway
	return svc, nil
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
	runCtx, cancel := context.WithCancel(ctx)
	s.runCtx = runCtx
	s.cancel = cancel
	s.bridge.backgroundCtx = runCtx
	s.mu.Unlock()

	if err := s.gateway.Start(runCtx); err != nil {
		cancel()
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
	cancel := s.cancel
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	return s.gateway.Close()
}

var _ Connector = (*Service)(nil)
