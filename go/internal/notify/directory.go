package notify

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"github.com/maarkn/frontdesk/internal/tenantctx"
)

// MemoryDirectory is an in-memory TenantDirectory for tests and local runs;
// production resolves tenants from the core-api projections (EPIC-004).
type MemoryDirectory struct {
	mu      sync.RWMutex
	tenants map[string]TenantInfo
}

var _ TenantDirectory = (*MemoryDirectory)(nil)

// NewMemoryDirectory returns an empty directory.
func NewMemoryDirectory() *MemoryDirectory {
	return &MemoryDirectory{tenants: make(map[string]TenantInfo)}
}

// Put registers or replaces a tenant.
func (d *MemoryDirectory) Put(tenantID string, info TenantInfo) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.tenants[tenantID] = info
}

// Lookup implements TenantDirectory: the tenant comes from the context
// (ADR-005), never from a parameter.
func (d *MemoryDirectory) Lookup(ctx context.Context) (TenantInfo, error) {
	id, err := tenantctx.FromContext(ctx)
	if err != nil {
		return TenantInfo{}, err
	}
	d.mu.RLock()
	defer d.mu.RUnlock()
	info, ok := d.tenants[id]
	if !ok {
		return TenantInfo{}, fmt.Errorf("notify: unknown tenant %s", id)
	}
	return info, nil
}

// TenantIDs implements TenantDirectory, in sorted order for determinism.
func (d *MemoryDirectory) TenantIDs(context.Context) ([]string, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	ids := make([]string, 0, len(d.tenants))
	for id := range d.tenants {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids, nil
}
