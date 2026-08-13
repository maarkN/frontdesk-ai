package store

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/maarkn/frontdesk/internal/domain"
	"github.com/maarkn/frontdesk/internal/event"
	"github.com/maarkn/frontdesk/internal/tenantctx"
)

// newTenant returns a valid tenant fixture.
func newTenant(id, keyHash string) domain.Tenant {
	return domain.Tenant{
		ID:              id,
		Name:            "Tenant " + id,
		Plan:            domain.PlanStarter,
		Mode:            domain.ModeOverflow,
		OverflowSeconds: 15,
		OwnerMobile:     "+15145550100",
		DefaultLocale:   event.LocaleENCA,
		APIKeyHash:      keyHash,
		CreatedAt:       time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC),
	}
}

// newMessage returns a valid structured-message fixture.
func newMessage(id string) domain.Message {
	return domain.Message{
		ID:            id,
		CallID:        "call-" + id,
		Who:           "Alice",
		What:          "Leaking water heater",
		Urgency:       domain.UrgencyHigh,
		CallbackPhone: "+15145550111",
		CreatedAt:     time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC),
	}
}

// TestMemIsolationBetweenTenants is the CI-style isolation test from nota 08
// / ADR-005, against the in-memory simulation of the RLS policy: two
// "transactions" of different tenants over the SAME store instance (the
// stand-in for the same pooled physical connection), asserting that neither
// sees the other's rows and that without app.tenant_id (no tenant in the
// context) no row comes back.
func TestMemIsolationBetweenTenants(t *testing.T) {
	mem := NewMem()
	base := context.Background()

	require.NoError(t, mem.CreateTenant(base, newTenant("tenant-a", "hash-a")))
	require.NoError(t, mem.CreateTenant(base, newTenant("tenant-b", "hash-b")))

	ctxA := tenantctx.WithTenant(base, "tenant-a")
	ctxB := tenantctx.WithTenant(base, "tenant-b")

	// Tenant A writes across every aggregate.
	require.NoError(t, mem.AddDID(ctxA, domain.DID{
		Number: "+15145550001", Provider: "telnyx", Active: true,
	}))
	require.NoError(t, mem.CreateMessage(ctxA, newMessage("msg-a")))
	require.NoError(t, mem.PutCall(ctxA, domain.Call{
		Snapshot: event.CallSnapshot{CallID: "call-a", TenantID: "tenant-a"},
	}))
	require.NoError(t, mem.CreateAppointment(ctxA, domain.Appointment{
		ID: "apt-a", CustomerName: "Alice", Service: "repair",
		StartsAt: time.Date(2026, 8, 14, 9, 0, 0, 0, time.UTC),
		EndsAt:   time.Date(2026, 8, 14, 10, 0, 0, 0, time.UTC),
	}))

	// Tenant B, on the same store ("same pooled connection"), sees nothing.
	for name, list := range map[string]func(context.Context) (int, error){
		"dids": func(ctx context.Context) (int, error) {
			rows, err := mem.ListDIDs(ctx)
			return len(rows), err
		},
		"messages": func(ctx context.Context) (int, error) {
			rows, err := mem.ListMessages(ctx)
			return len(rows), err
		},
		"calls": func(ctx context.Context) (int, error) {
			rows, err := mem.ListCalls(ctx)
			return len(rows), err
		},
		"appointments": func(ctx context.Context) (int, error) {
			rows, err := mem.ListAppointments(ctx)
			return len(rows), err
		},
	} {
		n, err := list(ctxB)
		require.NoError(t, err, name)
		assert.Zero(t, n, "tenant B must see zero %s of tenant A", name)
	}

	// Cross-tenant point read is indistinguishable from a miss.
	_, err := mem.GetCall(ctxB, "call-a")
	assert.ErrorIs(t, err, domain.ErrNotFound)

	// Tenant A still sees its own rows (the policy filters, not erases).
	msgs, err := mem.ListMessages(ctxA)
	require.NoError(t, err)
	assert.Len(t, msgs, 1)

	// Without app.tenant_id, every read fails closed: no default tenant.
	_, err = mem.ListMessages(base)
	assert.ErrorIs(t, err, tenantctx.ErrNoTenant)
	_, err = mem.CurrentTenant(base)
	assert.ErrorIs(t, err, tenantctx.ErrNoTenant)
	err = mem.CreateMessage(base, newMessage("msg-x"))
	assert.ErrorIs(t, err, tenantctx.ErrNoTenant)
}

func TestMemDIDGlobalUniqueness(t *testing.T) {
	mem := NewMem()
	base := context.Background()
	require.NoError(t, mem.CreateTenant(base, newTenant("tenant-a", "hash-a")))
	require.NoError(t, mem.CreateTenant(base, newTenant("tenant-b", "hash-b")))

	ctxA := tenantctx.WithTenant(base, "tenant-a")
	ctxB := tenantctx.WithTenant(base, "tenant-b")

	did := domain.DID{Number: "+15145550002", Provider: "telnyx", Active: true}
	require.NoError(t, mem.AddDID(ctxA, did))
	err := mem.AddDID(ctxB, did)
	assert.ErrorIs(t, err, domain.ErrConflict,
		"a DID provisioned for one tenant must not be reusable by another")
}

func TestMemTenantByAPIKeyHash(t *testing.T) {
	mem := NewMem()
	base := context.Background()
	require.NoError(t, mem.CreateTenant(base, newTenant("tenant-a", "hash-a")))

	tn, err := mem.TenantByAPIKeyHash(base, "hash-a")
	require.NoError(t, err)
	assert.Equal(t, "tenant-a", tn.ID)

	_, err = mem.TenantByAPIKeyHash(base, "hash-unknown")
	assert.ErrorIs(t, err, domain.ErrNotFound, "unknown key resolves to nothing, never a default tenant")
}

func TestMemUpdateTenantMode(t *testing.T) {
	mem := NewMem()
	base := context.Background()
	require.NoError(t, mem.CreateTenant(base, newTenant("tenant-a", "hash-a")))
	ctxA := tenantctx.WithTenant(base, "tenant-a")

	require.NoError(t, mem.UpdateTenantMode(ctxA, domain.ModeAlwaysAI, 0))
	tn, err := mem.CurrentTenant(ctxA)
	require.NoError(t, err)
	assert.Equal(t, domain.ModeAlwaysAI, tn.Mode)

	err = mem.UpdateTenantMode(ctxA, domain.Mode("bogus"), 0)
	assert.Error(t, err)
}
