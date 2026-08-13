//go:build integration

package store

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/maarkn/frontdesk/internal/domain"
	"github.com/maarkn/frontdesk/internal/tenantctx"
)

// TestPGIsolationBetweenTenants is the real RLS isolation test of ADR-005 /
// nota 08: two transactions with different tenants over the SAME physical
// connection of the pool, asserting that neither sees the other's rows and
// that without app.tenant_id no row returns. It needs a Postgres reachable
// via FRONTDESK_TEST_DATABASE_URL and therefore only runs under
// `go test -tags integration`.
func TestPGIsolationBetweenTenants(t *testing.T) {
	url := os.Getenv("FRONTDESK_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("FRONTDESK_TEST_DATABASE_URL not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cfg, err := pgxpool.ParseConfig(url)
	require.NoError(t, err)
	cfg.MaxConns = 1 // force every transaction through the same physical connection
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	require.NoError(t, err)
	defer pool.Close()

	migration, err := os.ReadFile(filepath.Join("migrations", "0001_init.sql"))
	require.NoError(t, err)
	_, err = pool.Exec(ctx, string(migration))
	require.NoError(t, err)

	pg := NewPG(pool)
	base := context.Background()
	now := time.Now().UTC()
	idA, idB := domain.NewID(now), domain.NewID(now)

	tenantA := newTenant(idA, "hash-"+idA)
	tenantB := newTenant(idB, "hash-"+idB)
	require.NoError(t, pg.CreateTenant(base, tenantA))
	require.NoError(t, pg.CreateTenant(base, tenantB))

	ctxA := tenantctx.WithTenant(base, idA)
	ctxB := tenantctx.WithTenant(base, idB)

	msg := newMessage(domain.NewID(now))
	require.NoError(t, pg.CreateMessage(ctxA, msg))

	// Same pool (MaxConns=1 → same physical connection), other tenant:
	// nothing visible.
	msgsB, err := pg.ListMessages(ctxB)
	require.NoError(t, err)
	assert.Empty(t, msgsB, "tenant B must not see tenant A rows on the same pooled connection")

	msgsA, err := pg.ListMessages(ctxA)
	require.NoError(t, err)
	assert.Len(t, msgsA, 1)

	// Without app.tenant_id the policy evaluates to NULL: zero rows even
	// for the table owner (FORCE ROW LEVEL SECURITY).
	var visible int
	err = pool.QueryRow(ctx, `select count(*) from messages`).Scan(&visible)
	require.NoError(t, err)
	assert.Zero(t, visible, "without app.tenant_id no row may be visible")

	// No tenant in context → fail closed before touching the database.
	_, err = pg.ListMessages(base)
	assert.ErrorIs(t, err, tenantctx.ErrNoTenant)
}
