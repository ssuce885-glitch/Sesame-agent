package runtimegraph

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go-agent/internal/store/sqlite"
)

type TurnContext struct {
	CurrentSessionID string
	CurrentTurnID    string
	CurrentRunID     string
	CurrentTaskID    string

	mu            sync.RWMutex
	fileReadState map[string]time.Time
}

func (t *TurnContext) HasFreshFileRead(path string, modifiedAt time.Time) bool {
	if t == nil || path == "" || modifiedAt.IsZero() {
		return false
	}
	t.mu.RLock()
	defer t.mu.RUnlock()
	if t.fileReadState == nil {
		return false
	}
	lastReadAt, ok := t.fileReadState[path]
	return ok && lastReadAt.Equal(modifiedAt)
}

func (t *TurnContext) RememberFileRead(path string, modifiedAt time.Time) {
	if t == nil || path == "" || modifiedAt.IsZero() {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.fileReadState == nil {
		t.fileReadState = make(map[string]time.Time)
	}
	t.fileReadState[path] = modifiedAt
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

func (s *Service) EnsureRun(ctx context.Context, turnCtx *TurnContext, sessionID, turnID, objective string) (string, error) {
	if s == nil || s.store == nil {
		return "", fmt.Errorf("runtime service is not configured")
	}
	if turnCtx == nil {
		return "", fmt.Errorf("turn runtime context is not configured")
	}
	if current := turnCtx.CurrentRunID; current != "" {
		return current, nil
	}

	now := time.Now().UTC()
	var runID string
	if err := s.store.WithTx(ctx, func(tx RuntimeTx) error {
		var err error
		runID, err = ensureRunLocked(ctx, tx, turnCtx, sessionID, turnID, objective, now)
		return err
	}); err != nil {
		return "", err
	}
	return runID, nil
}
