package engine

import (
	"context"
	"sync"
)

type queuedHeadMemoryJob struct {
	ctx context.Context
	in  Input
}

type InProcessHeadMemoryWorker struct {
	mu      sync.Mutex
	running map[string]bool
	pending map[string]queuedHeadMemoryJob
	wg      sync.WaitGroup
}

func NewInProcessHeadMemoryWorker() *InProcessHeadMemoryWorker {
	return &InProcessHeadMemoryWorker{
		running: make(map[string]bool),
		pending: make(map[string]queuedHeadMemoryJob),
	}
}

func (w *InProcessHeadMemoryWorker) Enqueue(ctx context.Context, e *Engine, in Input) {
	if w == nil || e == nil {
		return
	}
	headKey := headMemoryKeyForInput(in)
	if headKey == "" {
		return
	}

	w.mu.Lock()
	if w.running[headKey] {
		w.pending[headKey] = queuedHeadMemoryJob{ctx: ctx, in: in}
		w.mu.Unlock()
		return
	}
	w.running[headKey] = true
	w.mu.Unlock()

	w.wg.Add(1)
	go func(current queuedHeadMemoryJob) {
		defer w.wg.Done()
		for {
			_ = runObservedHeadMemoryRefresh(current.ctx, e, current.in, true)

			w.mu.Lock()
			next, ok := w.pending[headKey]
			if ok {
				delete(w.pending, headKey)
				w.mu.Unlock()
				current = next
				continue
			}
			delete(w.running, headKey)
			w.mu.Unlock()
			return
		}
	}(queuedHeadMemoryJob{ctx: ctx, in: in})
}

func (w *InProcessHeadMemoryWorker) Wait() {
	if w == nil {
		return
	}
	w.wg.Wait()
}
