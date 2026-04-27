package engine

import (
	"context"
	"sync"
)

type queuedContextHeadSummaryJob struct {
	ctx context.Context
	in  Input
}

type InProcessContextHeadSummaryWorker struct {
	mu      sync.Mutex
	running map[string]bool
	pending map[string]queuedContextHeadSummaryJob
	wg      sync.WaitGroup
}

func NewInProcessContextHeadSummaryWorker() *InProcessContextHeadSummaryWorker {
	return &InProcessContextHeadSummaryWorker{
		running: make(map[string]bool),
		pending: make(map[string]queuedContextHeadSummaryJob),
	}
}

func (w *InProcessContextHeadSummaryWorker) Enqueue(ctx context.Context, e *Engine, in Input) {
	if w == nil || e == nil {
		return
	}
	headKey := contextHeadSummaryKeyForInput(in)
	if headKey == "" {
		return
	}

	w.mu.Lock()
	if w.running[headKey] {
		w.pending[headKey] = queuedContextHeadSummaryJob{ctx: ctx, in: in}
		w.mu.Unlock()
		return
	}
	w.running[headKey] = true
	w.mu.Unlock()

	w.wg.Add(1)
	go func(current queuedContextHeadSummaryJob) {
		defer w.wg.Done()
		for {
			_ = runObservedContextHeadSummaryRefresh(current.ctx, e, current.in, true)

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
	}(queuedContextHeadSummaryJob{ctx: ctx, in: in})
}

func (w *InProcessContextHeadSummaryWorker) Wait() {
	if w == nil {
		return
	}
	w.wg.Wait()
}
