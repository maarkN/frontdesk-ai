package notify

import (
	"context"
	"sync"
)

// MemoryDedup is the in-memory Deduper for tests and single-instance runs.
// The persistent implementation (Postgres: insert ... on conflict do
// nothing) plugs in behind the same interface. The zero value is ready to
// use.
type MemoryDedup struct {
	mu   sync.Mutex
	seen map[string]struct{}
}

var _ Deduper = (*MemoryDedup)(nil)

// Seen implements Deduper.
func (d *MemoryDedup) Seen(_ context.Context, key string) (bool, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	_, ok := d.seen[key]
	return ok, nil
}

// Mark implements Deduper.
func (d *MemoryDedup) Mark(_ context.Context, key string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.seen == nil {
		d.seen = make(map[string]struct{})
	}
	d.seen[key] = struct{}{}
	return nil
}
