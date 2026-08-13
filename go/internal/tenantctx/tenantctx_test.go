package tenantctx_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/maarkn/frontdesk/internal/tenantctx"
)

func TestFromContextRoundTrip(t *testing.T) {
	ctx := tenantctx.WithTenant(context.Background(), "tenant-abc")
	id, err := tenantctx.FromContext(ctx)
	require.NoError(t, err)
	require.Equal(t, "tenant-abc", id)
}

func TestFromContextMissingTenantErrors(t *testing.T) {
	// No default tenant, ever: absence is an error the caller must handle.
	id, err := tenantctx.FromContext(context.Background())
	require.ErrorIs(t, err, tenantctx.ErrNoTenant)
	require.Empty(t, id)
}

func TestFromContextEmptyTenantErrors(t *testing.T) {
	ctx := tenantctx.WithTenant(context.Background(), "")
	_, err := tenantctx.FromContext(ctx)
	require.ErrorIs(t, err, tenantctx.ErrNoTenant)
}

func TestWithTenantDoesNotLeakAcrossContexts(t *testing.T) {
	parent := context.Background()
	_ = tenantctx.WithTenant(parent, "tenant-abc")

	// The parent context remains tenant-free.
	_, err := tenantctx.FromContext(parent)
	require.ErrorIs(t, err, tenantctx.ErrNoTenant)
}
