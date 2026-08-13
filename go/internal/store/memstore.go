// Package store persists the core-api aggregates. It ships two
// implementations with the same method set — Mem (in-memory, for tests and
// local runs) and PG (pgx + Postgres row-level security).
//
// The per-aggregate interfaces are small and defined in the consumer
// (internal/api), per uber-go/guide; this package only returns concrete
// structs. Every method takes the tenant EXCLUSIVELY from the context
// (tenantctx, ADR-005) — no method accepts a tenant id parameter. The only
// exceptions sit above the resolution point: CreateTenant (the tenant does
// not exist yet) and TenantByAPIKeyHash (it IS the resolution edge).
package store

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"github.com/maarkn/frontdesk/internal/domain"
	"github.com/maarkn/frontdesk/internal/tenantctx"
)

// Mem is the in-memory store. It mirrors the Postgres RLS policy: without a
// tenant in the context no row is visible, and a tenant only ever sees its
// own rows. The zero value is not usable; call NewMem.
type Mem struct {
	mu sync.RWMutex

	// tenants and apiKeys sit above the RLS boundary (resolution edge).
	tenants map[string]domain.Tenant
	apiKeys map[string]string // key hash -> tenant id

	// didOwners enforces global DID uniqueness (unique index in SQL).
	didOwners map[string]string // number -> tenant id

	// Per-tenant rows, keyed by tenant id then aggregate id.
	dids         map[string]map[string]domain.DID
	calls        map[string]map[string]domain.Call
	appointments map[string]map[string]domain.Appointment
	messages     map[string]map[string]domain.Message
}

// NewMem returns an empty in-memory store.
func NewMem() *Mem {
	return &Mem{
		tenants:      make(map[string]domain.Tenant),
		apiKeys:      make(map[string]string),
		didOwners:    make(map[string]string),
		dids:         make(map[string]map[string]domain.DID),
		calls:        make(map[string]map[string]domain.Call),
		appointments: make(map[string]map[string]domain.Appointment),
		messages:     make(map[string]map[string]domain.Message),
	}
}

// CreateTenant registers a tenant and its API key hash. It runs above the
// resolution point: the id comes from the aggregate being created.
func (m *Mem) CreateTenant(_ context.Context, t domain.Tenant) error {
	if err := t.Validate(); err != nil {
		return fmt.Errorf("create tenant: %w", err)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.tenants[t.ID]; exists {
		return fmt.Errorf("create tenant %s: %w", t.ID, domain.ErrConflict)
	}
	m.tenants[t.ID] = t
	if t.APIKeyHash != "" {
		m.apiKeys[t.APIKeyHash] = t.ID
	}
	return nil
}

// TenantByAPIKeyHash resolves an API key hash to its tenant. It is the
// resolution edge for the HTTP API: unknown key yields domain.ErrNotFound,
// never a default tenant.
func (m *Mem) TenantByAPIKeyHash(_ context.Context, keyHash string) (domain.Tenant, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	id, ok := m.apiKeys[keyHash]
	if !ok {
		return domain.Tenant{}, fmt.Errorf("tenant by api key: %w", domain.ErrNotFound)
	}
	return m.tenants[id], nil
}

// CurrentTenant returns the tenant carried by the context.
func (m *Mem) CurrentTenant(ctx context.Context) (domain.Tenant, error) {
	id, err := tenantctx.FromContext(ctx)
	if err != nil {
		return domain.Tenant{}, fmt.Errorf("current tenant: %w", err)
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	t, ok := m.tenants[id]
	if !ok {
		return domain.Tenant{}, fmt.Errorf("current tenant %s: %w", id, domain.ErrNotFound)
	}
	return t, nil
}

// UpdateTenantMode toggles overflow / always-AI for the context tenant.
func (m *Mem) UpdateTenantMode(ctx context.Context, mode domain.Mode, overflowSeconds int) error {
	id, err := tenantctx.FromContext(ctx)
	if err != nil {
		return fmt.Errorf("update mode: %w", err)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.tenants[id]
	if !ok {
		return fmt.Errorf("update mode: tenant %s: %w", id, domain.ErrNotFound)
	}
	t.Mode = mode
	t.OverflowSeconds = overflowSeconds
	if err := t.Validate(); err != nil {
		return fmt.Errorf("update mode: %w", err)
	}
	m.tenants[id] = t
	return nil
}

// AddDID provisions a DID for the context tenant. Numbers are globally
// unique across tenants.
func (m *Mem) AddDID(ctx context.Context, d domain.DID) error {
	id, err := tenantctx.FromContext(ctx)
	if err != nil {
		return fmt.Errorf("add did: %w", err)
	}
	if err := d.Validate(); err != nil {
		return fmt.Errorf("add did: %w", err)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, taken := m.didOwners[d.Number]; taken {
		return fmt.Errorf("add did %s: %w", d.Number, domain.ErrConflict)
	}
	m.didOwners[d.Number] = id
	rows := m.dids[id]
	if rows == nil {
		rows = make(map[string]domain.DID)
		m.dids[id] = rows
	}
	rows[d.Number] = d
	return nil
}

// ListDIDs returns the context tenant's DIDs, ordered by number.
func (m *Mem) ListDIDs(ctx context.Context) ([]domain.DID, error) {
	id, err := tenantctx.FromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("list dids: %w", err)
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]domain.DID, 0, len(m.dids[id]))
	for _, d := range m.dids[id] {
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Number < out[j].Number })
	return out, nil
}

// PutCall upserts a call projection for the context tenant. Projections are
// folds of the event stream, so replays overwrite idempotently.
func (m *Mem) PutCall(ctx context.Context, c domain.Call) error {
	id, err := tenantctx.FromContext(ctx)
	if err != nil {
		return fmt.Errorf("put call: %w", err)
	}
	if c.ID() == "" {
		return fmt.Errorf("put call: missing call id")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	rows := m.calls[id]
	if rows == nil {
		rows = make(map[string]domain.Call)
		m.calls[id] = rows
	}
	rows[c.ID()] = c
	return nil
}

// ListCalls returns the context tenant's calls, most recent first.
func (m *Mem) ListCalls(ctx context.Context) ([]domain.Call, error) {
	id, err := tenantctx.FromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("list calls: %w", err)
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]domain.Call, 0, len(m.calls[id]))
	for _, c := range m.calls[id] {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Snapshot.StartedAt.After(out[j].Snapshot.StartedAt)
	})
	return out, nil
}

// GetCall returns one call of the context tenant; another tenant's call is
// indistinguishable from a miss.
func (m *Mem) GetCall(ctx context.Context, callID string) (domain.Call, error) {
	id, err := tenantctx.FromContext(ctx)
	if err != nil {
		return domain.Call{}, fmt.Errorf("get call: %w", err)
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	c, ok := m.calls[id][callID]
	if !ok {
		return domain.Call{}, fmt.Errorf("get call %s: %w", callID, domain.ErrNotFound)
	}
	return c, nil
}

// CreateAppointment stores an appointment for the context tenant.
func (m *Mem) CreateAppointment(ctx context.Context, a domain.Appointment) error {
	id, err := tenantctx.FromContext(ctx)
	if err != nil {
		return fmt.Errorf("create appointment: %w", err)
	}
	if err := a.Validate(); err != nil {
		return fmt.Errorf("create appointment: %w", err)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	rows := m.appointments[id]
	if rows == nil {
		rows = make(map[string]domain.Appointment)
		m.appointments[id] = rows
	}
	if _, exists := rows[a.ID]; exists {
		return fmt.Errorf("create appointment %s: %w", a.ID, domain.ErrConflict)
	}
	rows[a.ID] = a
	return nil
}

// ListAppointments returns the context tenant's appointments by start time.
func (m *Mem) ListAppointments(ctx context.Context) ([]domain.Appointment, error) {
	id, err := tenantctx.FromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("list appointments: %w", err)
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]domain.Appointment, 0, len(m.appointments[id]))
	for _, a := range m.appointments[id] {
		out = append(out, a)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].StartsAt.Before(out[j].StartsAt) })
	return out, nil
}

// CreateMessage stores a structured message for the context tenant.
func (m *Mem) CreateMessage(ctx context.Context, msg domain.Message) error {
	id, err := tenantctx.FromContext(ctx)
	if err != nil {
		return fmt.Errorf("create message: %w", err)
	}
	if err := msg.Validate(); err != nil {
		return fmt.Errorf("create message: %w", err)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	rows := m.messages[id]
	if rows == nil {
		rows = make(map[string]domain.Message)
		m.messages[id] = rows
	}
	if _, exists := rows[msg.ID]; exists {
		return fmt.Errorf("create message %s: %w", msg.ID, domain.ErrConflict)
	}
	rows[msg.ID] = msg
	return nil
}

// ListMessages returns the context tenant's messages, most recent first.
func (m *Mem) ListMessages(ctx context.Context) ([]domain.Message, error) {
	id, err := tenantctx.FromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("list messages: %w", err)
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]domain.Message, 0, len(m.messages[id]))
	for _, msg := range m.messages[id] {
		out = append(out, msg)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}
