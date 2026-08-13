package billing

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/maarkn/frontdesk/internal/domain"
	"github.com/maarkn/frontdesk/internal/tenantctx"
)

// TrialDays is the free trial length (RF4: Stripe with a 14-day trial).
const TrialDays = 14

// SubscriptionStatus mirrors the Stripe lifecycle states the MVP cares about.
type SubscriptionStatus string

// Subscription statuses.
const (
	StatusTrialing SubscriptionStatus = "trialing"
	StatusActive   SubscriptionStatus = "active"
	StatusCanceled SubscriptionStatus = "canceled"
)

// Subscription is the platform view of a payment-provider subscription.
type Subscription struct {
	ID          string             `json:"id"`
	Plan        domain.Plan        `json:"plan"`
	Status      SubscriptionStatus `json:"status"`
	TrialEndsAt time.Time          `json:"trialEndsAt,omitzero"`
}

// Gateway is the minimal payment-provider surface the core-api consumes.
// The production implementation wraps Stripe; the MVP ships FakeStripe. The
// tenant comes from the context (ADR-005), never as a parameter.
type Gateway interface {
	// CreateSubscription starts a subscription with a 14-day trial for
	// the context tenant.
	CreateSubscription(ctx context.Context, plan domain.Plan) (Subscription, error)
	// Subscription returns the context tenant's subscription.
	Subscription(ctx context.Context) (Subscription, error)
}

// FakeStripe is the deterministic, offline payment gateway of the MVP.
type FakeStripe struct {
	// Now stamps trial windows; time.Now when nil (zero value is usable).
	Now func() time.Time

	mu   sync.Mutex
	subs map[string]Subscription // tenant id -> subscription
}

var _ Gateway = (*FakeStripe)(nil)

// CreateSubscription starts a trialing subscription for the context tenant.
func (f *FakeStripe) CreateSubscription(ctx context.Context, plan domain.Plan) (Subscription, error) {
	tenantID, err := tenantctx.FromContext(ctx)
	if err != nil {
		return Subscription{}, fmt.Errorf("create subscription: %w", err)
	}
	if _, err := PricingFor(plan); err != nil {
		return Subscription{}, fmt.Errorf("create subscription: %w", err)
	}
	now := time.Now
	if f.Now != nil {
		now = f.Now
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	if _, exists := f.subs[tenantID]; exists {
		return Subscription{}, fmt.Errorf("create subscription: tenant already subscribed: %w", domain.ErrConflict)
	}
	if f.subs == nil {
		f.subs = make(map[string]Subscription)
	}
	sub := Subscription{
		ID:          "sub_" + domain.NewID(now()),
		Plan:        plan,
		Status:      StatusTrialing,
		TrialEndsAt: now().UTC().AddDate(0, 0, TrialDays),
	}
	f.subs[tenantID] = sub
	return sub, nil
}

// Subscription returns the context tenant's subscription.
func (f *FakeStripe) Subscription(ctx context.Context) (Subscription, error) {
	tenantID, err := tenantctx.FromContext(ctx)
	if err != nil {
		return Subscription{}, fmt.Errorf("get subscription: %w", err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	sub, ok := f.subs[tenantID]
	if !ok {
		return Subscription{}, fmt.Errorf("get subscription: %w", domain.ErrNotFound)
	}
	return sub, nil
}
