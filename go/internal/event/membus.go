package event

import (
	"context"
	"sync"
)

// MemoryBus is an in-process Publisher/Subscriber for tests and local tools.
// Delivery is synchronous: Publish invokes every subscribed handler before
// returning. Duplicate (CallID, Seq) publications are dropped, matching the
// consumer-side dedup contract of the real bus. The zero value is ready to
// use.
type MemoryBus struct {
	mu       sync.Mutex
	dedup    seqDedup
	events   []Envelope
	handlers []Handler
}

var (
	_ Publisher  = (*MemoryBus)(nil)
	_ Subscriber = (*MemoryBus)(nil)
)

// Publish implements Publisher. Handler errors are returned to the caller —
// there is no redelivery in memory.
func (b *MemoryBus) Publish(ctx context.Context, ev Envelope) error {
	if b.dedup.isDuplicate(ev.CallID, ev.Seq) {
		return nil
	}

	b.mu.Lock()
	b.events = append(b.events, ev)
	handlers := make([]Handler, len(b.handlers))
	copy(handlers, b.handlers)
	b.mu.Unlock()

	for _, h := range handlers {
		if err := h(ctx, ev); err != nil {
			return err
		}
	}
	return nil
}

// Subscribe implements Subscriber: the handler receives every event published
// after the call, synchronously. It blocks until ctx is done, mirroring the
// JetStream implementation's lifecycle.
func (b *MemoryBus) Subscribe(ctx context.Context, h Handler) error {
	b.mu.Lock()
	b.handlers = append(b.handlers, h)
	b.mu.Unlock()

	<-ctx.Done()
	return ctx.Err()
}

// Handle registers a handler without blocking — the test-friendly form of
// Subscribe.
func (b *MemoryBus) Handle(h Handler) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.handlers = append(b.handlers, h)
}

// Events returns a copy of everything published so far, in publish order.
func (b *MemoryBus) Events() []Envelope {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]Envelope, len(b.events))
	copy(out, b.events)
	return out
}
