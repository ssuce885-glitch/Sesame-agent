package stream

import (
	"sync"

	"go-agent/internal/types"
)

type Bus struct {
	mu          sync.Mutex
	nextID      uint64
	subscribers map[string]map[uint64]chan types.Event
}

func NewBus() *Bus {
	return &Bus{subscribers: make(map[string]map[uint64]chan types.Event)}
}

func (b *Bus) Publish(event types.Event) {
	b.mu.Lock()
	defer b.mu.Unlock()

	subs := b.subscribers[event.SessionID]
	for id, ch := range subs {
		select {
		case ch <- event:
		default:
			delete(subs, id)
			close(ch)
		}
	}
	if len(subs) == 0 {
		delete(b.subscribers, event.SessionID)
	}
}

func (b *Bus) Subscribe(sessionID string) (<-chan types.Event, func()) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.nextID++
	id := b.nextID
	ch := make(chan types.Event, 32)
	if b.subscribers[sessionID] == nil {
		b.subscribers[sessionID] = make(map[uint64]chan types.Event)
	}
	b.subscribers[sessionID][id] = ch

	var once sync.Once
	unsubscribe := func() {
		once.Do(func() {
			b.mu.Lock()
			defer b.mu.Unlock()

			subs := b.subscribers[sessionID]
			existing, ok := subs[id]
			if !ok {
				return
			}
			delete(subs, id)
			if len(subs) == 0 {
				delete(b.subscribers, sessionID)
			}
			close(existing)
		})
	}

	return ch, unsubscribe
}
