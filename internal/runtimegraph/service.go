package runtimegraph

import (
	"context"
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
