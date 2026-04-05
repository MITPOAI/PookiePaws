package engine

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

const defaultEventBuffer = 16

type eventSubscriber struct {
	ch chan Event
}

type StandardEventBus struct {
	mu        sync.RWMutex
	statsMu   sync.Mutex
	subs      map[uint64]eventSubscriber
	nextID    uint64
	published uint64
	closed    bool
	dropped   map[EventType]int
}

func NewEventBus() *StandardEventBus {
	return &StandardEventBus{
		subs:    make(map[uint64]eventSubscriber),
		dropped: make(map[EventType]int),
	}
}

func (b *StandardEventBus) Subscribe(buffer int) EventSubscription {
	if buffer <= 0 {
		buffer = defaultEventBuffer
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		ch := make(chan Event)
		close(ch)
		return EventSubscription{C: ch}
	}

	id := atomic.AddUint64(&b.nextID, 1)
	ch := make(chan Event, buffer)
	b.subs[id] = eventSubscriber{ch: ch}
	return EventSubscription{ID: id, C: ch}
}

func (b *StandardEventBus) Unsubscribe(id uint64) {
	b.mu.Lock()
	defer b.mu.Unlock()

	sub, ok := b.subs[id]
	if !ok {
		return
	}

	delete(b.subs, id)
	close(sub.ch)
}

func (b *StandardEventBus) Publish(ctx context.Context, event Event) error {
	b.mu.RLock()
	if b.closed {
		b.mu.RUnlock()
		return ErrAlreadyClosed
	}

	if event.ID == "" {
		event.ID = fmt.Sprintf("evt_%d", time.Now().UnixNano())
	}
	if event.Time.IsZero() {
		event.Time = time.Now().UTC()
	}

	subs := make([]chan Event, 0, len(b.subs))
	for _, sub := range b.subs {
		subs = append(subs, sub.ch)
	}
	b.mu.RUnlock()

	for _, ch := range subs {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case ch <- event:
		default:
			b.statsMu.Lock()
			b.dropped[event.Type]++
			b.statsMu.Unlock()
		}
	}

	atomic.AddUint64(&b.published, 1)
	return nil
}

func (b *StandardEventBus) Snapshot() EventBusSnapshot {
	b.mu.RLock()
	subscribers := len(b.subs)
	closed := b.closed
	b.mu.RUnlock()

	b.statsMu.Lock()
	dropped := make(map[EventType]int, len(b.dropped))
	for eventType, count := range b.dropped {
		dropped[eventType] = count
	}
	b.statsMu.Unlock()

	return EventBusSnapshot{
		Subscribers: subscribers,
		Published:   atomic.LoadUint64(&b.published),
		Closed:      closed,
		Dropped:     dropped,
	}
}

func (b *StandardEventBus) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		return
	}
	b.closed = true

	for id, sub := range b.subs {
		close(sub.ch)
		delete(b.subs, id)
	}
}
