// Package tenantctx carries the tenant identity in a context.Context.
//
// Rule (ADR-005, CONTEXT.md §5): the tenant is resolved exactly once, at call
// start, from the DID — an unknown DID hangs up. From that point on, "nenhuma
// função aceita tenantID como parâmetro": no function below the resolution
// point takes a tenant id argument. The ONLY way to obtain the tenant is
// FromContext, and it returns an error when the tenant is absent — there is
// never a default tenant, because a silently wrong default would cross tenant
// boundaries (RLS, limits, crypto keys).
package tenantctx

import (
	"context"
	"errors"
)

// ErrNoTenant is returned by FromContext when the context carries no tenant.
// Callers must treat it as a programming or routing error, never fall back to
// a default tenant.
var ErrNoTenant = errors.New("tenantctx: no tenant in context")

// ctxKey is the private context key type; being unexported, no other package
// can forge or shadow the tenant entry.
type ctxKey struct{}

// WithTenant returns a copy of ctx carrying the tenant id. It is called at
// the single resolution point (DID → tenant at call start, or authenticated
// principal → tenant at the API edge).
func WithTenant(ctx context.Context, tenantID string) context.Context {
	return context.WithValue(ctx, ctxKey{}, tenantID)
}

// FromContext extracts the tenant id. It returns ErrNoTenant when the context
// has no tenant or an empty one — never a default.
func FromContext(ctx context.Context) (string, error) {
	id, ok := ctx.Value(ctxKey{}).(string)
	if !ok || id == "" {
		return "", ErrNoTenant
	}
	return id, nil
}
