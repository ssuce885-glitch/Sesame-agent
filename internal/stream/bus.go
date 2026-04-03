package stream

import (
	"sync"

	"go-agent/internal/types"
)

type Bus struct {
	mu          sync.RWMutex
	subscribers map[string][]chan types.Event
}

func NewBus() *Bus {
	return &Bus{subscribers: make(map[string][]chan types.Event)}
}

func (b *Bus) Publish(event types.Event) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	for _, ch := range b.subscribers[event.SessionID] {
		select {
		case ch <- event:
		default:
		}
	}
}

func (b *Bus) Subscribe(sessionID string) <-chan types.Event {
	b.mu.Lock()
	defer b.mu.Unlock()

	ch := make(chan types.Event, 32)
	b.subscribers[sessionID] = append(b.subscribers[sessionID], ch)

	return ch
}
