package event

import (
	"context"
	"sync"
)

// Publisher publishes business events to the stream. Implementations must be
// safe for concurrent use.
type Publisher interface {
	Publish(ctx context.Context, ev Envelope) error
}

// Handler processes one delivered event. Returning an error signals the
// transport that delivery failed (redelivery follows on at-least-once buses).
type Handler func(ctx context.Context, ev Envelope) error

// Subscriber delivers events to a handler. Subscribe blocks until ctx is
// done; delivery is at-least-once and unordered unless the implementation
// says otherwise, so handlers must stay idempotent by (CallID, Seq).
type Subscriber interface {
	Subscribe(ctx context.Context, h Handler) error
}

// seqDedup drops already-seen (callID, seq) pairs, implementing the
// consumer-side dedup contract for at-least-once transports.
type seqDedup struct {
	mu   sync.Mutex
	seen map[string]map[uint64]struct{}
}

// isDuplicate records (callID, seq) and reports whether it was already seen.
func (d *seqDedup) isDuplicate(callID string, seq uint64) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.seen == nil {
		d.seen = make(map[string]map[uint64]struct{})
	}
	calls, ok := d.seen[callID]
	if !ok {
		calls = make(map[uint64]struct{})
		d.seen[callID] = calls
	}
	if _, dup := calls[seq]; dup {
		return true
	}
	calls[seq] = struct{}{}
	return false
}

// forget releases the dedup state of a finished call.
func (d *seqDedup) forget(callID string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.seen, callID)
}
