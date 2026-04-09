package engine

import (
	"context"
	"sync"
)

type InProcessSessionMemoryWorker struct {
	mu      sync.Mutex
	running map[string]bool
	pending map[string]queuedSessionMemoryJob
	wg      sync.WaitGroup
}

type queuedSessionMemoryJob struct {
	ctx context.Context
	in  Input
}

func NewInProcessSessionMemoryWorker() *InProcessSessionMemoryWorker {
	return &InProcessSessionMemoryWorker{
		running: make(map[string]bool),
		pending: make(map[string]queuedSessionMemoryJob),
	}
}

func (w *InProcessSessionMemoryWorker) Enqueue(ctx context.Context, e *Engine, in Input) {
	if w == nil || e == nil {
		return
	}
	sessionID := sessionIDForInput(in)
	if sessionID == "" {
		return
	}

	w.mu.Lock()
	if w.running[sessionID] {
		w.pending[sessionID] = queuedSessionMemoryJob{ctx: ctx, in: in}
		w.mu.Unlock()
		return
	}
	w.running[sessionID] = true
	w.mu.Unlock()

	w.wg.Add(1)
	go func(current queuedSessionMemoryJob) {
		defer w.wg.Done()
		for {
			_ = runObservedSessionMemoryRefresh(current.ctx, e, current.in, true)

			w.mu.Lock()
			next, ok := w.pending[sessionID]
			if ok {
				delete(w.pending, sessionID)
				w.mu.Unlock()
				current = next
				continue
			}
			delete(w.running, sessionID)
			w.mu.Unlock()
			return
		}
	}(queuedSessionMemoryJob{ctx: ctx, in: in})
}

func (w *InProcessSessionMemoryWorker) Wait() {
	if w == nil {
		return
	}
	w.wg.Wait()
}
