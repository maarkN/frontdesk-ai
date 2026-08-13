package degrade

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// ErrAllProvidersFailed reports that every hedged provider failed. It must
// never surface to the caller on the line: the turn loop converts it into a
// canned phrase plus escalation to Transfer (ADR-006 §2).
var ErrAllProvidersFailed = errors.New("degrade: all providers failed")

// Completer is the minimal LLM surface the hedge composes over. The real
// implementation streams; for hedging, first-token wins and the loser's
// context is cancelled, which for a stream means the first Completer to
// resolve its first token.
type Completer interface {
	Complete(ctx context.Context, prompt string) (string, error)
}

// CompleterFunc adapts a function to Completer.
type CompleterFunc func(ctx context.Context, prompt string) (string, error)

// Complete implements Completer.
func (f CompleterFunc) Complete(ctx context.Context, prompt string) (string, error) {
	return f(ctx, prompt)
}

// Hedge races a primary and a secondary LLM provider: the secondary starts
// only after hedgeAfter (~p90 of the primary, default 700ms), the FIRST
// response wins, and the loser is cancelled via context. This overlaps
// latencies instead of adding them — sequential retry (wait for the primary
// to fail, then try the secondary) is forbidden on the call path.
//
// Cost: the call is duplicated on ~10% of turns — exactly the turns that
// would have ruined the conversation.
type Hedge struct {
	primary       Completer
	secondary     Completer
	after         time.Duration
	breakers      *Registry
	primaryName   string
	secondaryName string
	onHedge       func()
}

// HedgeOption configures a Hedge.
type HedgeOption func(*Hedge)

// WithHedgeAfter sets the delay before the secondary starts (default 700ms).
func WithHedgeAfter(d time.Duration) HedgeOption {
	return func(h *Hedge) { h.after = d }
}

// WithBreakers consults per-provider breakers before each attempt: an open
// breaker skips that provider immediately (0ms) instead of attempting.
func WithBreakers(r *Registry, primaryName, secondaryName string) HedgeOption {
	return func(h *Hedge) {
		h.breakers = r
		h.primaryName = primaryName
		h.secondaryName = secondaryName
	}
}

// WithOnHedge observes every fired hedge (duplicated-cost metric).
func WithOnHedge(fn func()) HedgeOption {
	return func(h *Hedge) { h.onHedge = fn }
}

// NewHedge returns a hedge over the two providers.
func NewHedge(primary, secondary Completer, opts ...HedgeOption) *Hedge {
	h := &Hedge{
		primary:       primary,
		secondary:     secondary,
		after:         700 * time.Millisecond,
		primaryName:   "llm-primary",
		secondaryName: "llm-secondary",
	}
	for _, o := range opts {
		o(h)
	}
	return h
}

var _ Completer = (*Hedge)(nil)

type hedgeResult struct {
	text string
	err  error
}

// Complete implements Completer with the hedge race.
func (h *Hedge) Complete(ctx context.Context, prompt string) (string, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel() // cancels the loser; producers select on ctx.Done

	// Buffered so a losing producer never blocks after we return.
	results := make(chan hedgeResult, 2)

	launched := 0
	launch := func(c Completer, name string) {
		launched++
		go func() {
			var br *Breaker
			if h.breakers != nil {
				br = h.breakers.For(name)
				if !br.Allow() {
					results <- hedgeResult{err: fmt.Errorf("degrade: breaker open for %s", name)}
					return
				}
			}
			text, err := c.Complete(ctx, prompt)
			if br != nil {
				// A loser cancelled mid-flight is not a provider failure.
				if err == nil {
					br.Success()
				} else if !errors.Is(err, context.Canceled) {
					br.Failure()
				}
			}
			results <- hedgeResult{text: text, err: err}
		}()
	}

	launch(h.primary, h.primaryName)

	timer := time.NewTimer(h.after)
	defer timer.Stop()

	var errs []error
	secondaryStarted := false
	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-timer.C:
			if !secondaryStarted {
				secondaryStarted = true
				if h.onHedge != nil {
					h.onHedge()
				}
				launch(h.secondary, h.secondaryName)
			}
		case r := <-results:
			if r.err == nil {
				return r.text, nil // first responder wins
			}
			errs = append(errs, r.err)
			// Primary failed before the hedge timer: start the secondary NOW
			// — waiting for the timer would be sequential-retry latency.
			if !secondaryStarted {
				secondaryStarted = true
				launch(h.secondary, h.secondaryName)
				continue
			}
			if len(errs) == launched {
				return "", fmt.Errorf("%w: %w", ErrAllProvidersFailed, errors.Join(errs...))
			}
		}
	}
}
