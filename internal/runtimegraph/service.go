package runtimegraph

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"go-agent/internal/store/sqlite"
	"go-agent/internal/types"
)

type TurnContext struct {
	CurrentSessionID    string
	CurrentTurnID       string
	CurrentRunID        string
	CurrentTaskID       string
	CurrentRunObjective string
	CurrentRunStartedAt time.Time

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

func (s *Service) FinishCurrentRun(ctx context.Context, turnCtx *TurnContext, sessionID, turnID string, runErr error) error {
	if s == nil || s.store == nil || turnCtx == nil || turnCtx.CurrentRunID == "" {
		return nil
	}

	now := time.Now().UTC()
	state := types.RunStateCompleted
	errorText := ""
	switch {
	case errors.Is(runErr, context.Canceled):
		state = types.RunStateInterrupted
		errorText = runErr.Error()
	case runErr != nil:
		state = types.RunStateFailed
		errorText = runErr.Error()
	}
	run := types.Run{
		ID:        turnCtx.CurrentRunID,
		SessionID: sessionID,
		TurnID:    turnID,
		State:     state,
		Objective: turnCtx.CurrentRunObjective,
		Error:     errorText,
		CreatedAt: firstNonZeroTime(turnCtx.CurrentRunStartedAt, now),
		UpdatedAt: now,
	}
	if err := s.store.WithTx(ctx, func(tx RuntimeTx) error {
		return tx.UpsertRun(ctx, run)
	}); err != nil {
		return err
	}
	turnCtx.CurrentRunID = ""
	turnCtx.CurrentRunObjective = ""
	turnCtx.CurrentRunStartedAt = time.Time{}
	return nil
}

func firstNonZeroTime(values ...time.Time) time.Time {
	for _, value := range values {
		if !value.IsZero() {
			return value
		}
	}
	return time.Time{}
}
