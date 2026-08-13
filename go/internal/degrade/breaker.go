package degrade

import (
	"sync"
	"time"
)

// BreakerState is the classic three-state circuit-breaker state.
type BreakerState int

// Breaker states.
const (
	// BreakerClosed: provider healthy, calls flow.
	BreakerClosed BreakerState = iota
	// BreakerOpen: provider considered down; calls skip straight to their
	// degradation fallback without attempting.
	BreakerOpen
	// BreakerHalfOpen: one probe in flight; everyone else keeps falling back.
	BreakerHalfOpen
)

// String returns the state name exported as the provider_breaker_state metric.
func (s BreakerState) String() string {
	switch s {
	case BreakerClosed:
		return "closed"
	case BreakerOpen:
		return "open"
	case BreakerHalfOpen:
		return "half_open"
	default:
		return "unknown"
	}
}

// Breaker is a circuit breaker for ONE provider, shared by every call in the
// process (ADR-006 §3: per provider, never per call — 200 calls each retrying
// on their own is a retry storm that turns partial degradation into total
// outage). Half-open admits exactly one probe per probe interval.
type Breaker struct {
	maxFailures int
	probeEvery  time.Duration
	now         func() time.Time

	mu       sync.Mutex
	state    BreakerState
	failures int
	openedAt time.Time
	probing  bool
}

// BreakerOption configures a Breaker.
type BreakerOption func(*Breaker)

// WithMaxFailures sets consecutive failures that open the breaker (default 3).
func WithMaxFailures(n int) BreakerOption {
	return func(b *Breaker) { b.maxFailures = n }
}

// WithProbeInterval sets the half-open probe cadence (default 5s: 1 probe/5s
// for the whole provider, whatever the number of active calls).
func WithProbeInterval(d time.Duration) BreakerOption {
	return func(b *Breaker) { b.probeEvery = d }
}

// WithClock injects the time source (tests).
func WithClock(now func() time.Time) BreakerOption {
	return func(b *Breaker) { b.now = now }
}

// NewBreaker returns a closed breaker.
func NewBreaker(opts ...BreakerOption) *Breaker {
	b := &Breaker{maxFailures: 3, probeEvery: 5 * time.Second, now: time.Now}
	for _, o := range opts {
		o(b)
	}
	return b
}

// Allow reports whether the caller may attempt the provider now. When the
// breaker is open it returns true at most once per probe interval (the probe);
// every other caller must jump directly to its degradation fallback.
func (b *Breaker) Allow() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	switch b.state {
	case BreakerClosed:
		return true
	case BreakerHalfOpen:
		return false // a probe is already in flight
	case BreakerOpen:
		if b.now().Sub(b.openedAt) < b.probeEvery {
			return false
		}
		b.state = BreakerHalfOpen
		b.probing = true
		return true
	default:
		return false
	}
}

// Success records a successful attempt (or probe): the breaker closes.
func (b *Breaker) Success() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.state = BreakerClosed
	b.failures = 0
	b.probing = false
}

// Failure records a failed attempt. In half-open it reopens the breaker and
// restarts the probe clock; in closed it opens after maxFailures consecutive
// failures.
func (b *Breaker) Failure() {
	b.mu.Lock()
	defer b.mu.Unlock()
	switch b.state {
	case BreakerHalfOpen:
		b.state = BreakerOpen
		b.openedAt = b.now()
		b.probing = false
	case BreakerClosed:
		b.failures++
		if b.failures >= b.maxFailures {
			b.state = BreakerOpen
			b.openedAt = b.now()
		}
	case BreakerOpen:
		// already open; nothing to count
	}
}

// State returns the current state (metrics).
func (b *Breaker) State() BreakerState {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.state
}

// Registry hands out the process-wide breaker of each provider by name.
// The zero value is ready to use.
type Registry struct {
	mu       sync.Mutex
	breakers map[string]*Breaker
	opts     []BreakerOption
}

// NewRegistry returns a registry applying opts to every breaker it creates.
func NewRegistry(opts ...BreakerOption) *Registry {
	return &Registry{opts: opts}
}

// For returns the breaker of the named provider, creating it on first use.
func (r *Registry) For(provider string) *Breaker {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.breakers == nil {
		r.breakers = make(map[string]*Breaker)
	}
	b, ok := r.breakers[provider]
	if !ok {
		b = NewBreaker(r.opts...)
		r.breakers[provider] = b
	}
	return b
}

// States snapshots every provider's state (metric export).
func (r *Registry) States() map[string]BreakerState {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make(map[string]BreakerState, len(r.breakers))
	for name, b := range r.breakers {
		out[name] = b.State()
	}
	return out
}
