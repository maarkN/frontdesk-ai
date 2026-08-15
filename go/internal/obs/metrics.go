package obs

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/maarkn/frontdesk/internal/audiobank"
	"github.com/maarkn/frontdesk/internal/degrade"
	"github.com/maarkn/frontdesk/internal/event"
	"github.com/maarkn/frontdesk/internal/notify"
	"github.com/maarkn/frontdesk/internal/turn"
)

// Metrics holds the FrontDesk product instruments (EPIC-010). It is the
// single OTel implementation of the metric hook points the domain packages
// already expose: notify.Metrics, the degrade ladder onChange callback, the
// breaker registry snapshot and turn.Hooks — instrumentation stays here, the
// domain packages stay OTel-free (ADR-008).
type Metrics struct {
	now func() time.Time

	silenceGap    metric.Int64Histogram
	degLevel      metric.Int64Gauge
	degTransition metric.Int64Counter
	bargeInFP     metric.Int64Counter
	breakerState  metric.Int64ObservableGauge
	smsLatency    metric.Float64Histogram
	orphans       metric.Int64Counter
}

// MetricsOption configures NewMetrics.
type MetricsOption func(*Metrics)

// WithMetricsClock injects the time source (tests).
func WithMetricsClock(now func() time.Time) MetricsOption {
	return func(m *Metrics) { m.now = now }
}

// NewMetrics creates every product instrument on meter.
func NewMetrics(meter metric.Meter, opts ...MetricsOption) (*Metrics, error) {
	m := &Metrics{now: time.Now}
	for _, o := range opts {
		o(m)
	}
	var err error
	if m.silenceGap, err = meter.Int64Histogram("silence_gap_ms",
		metric.WithUnit("ms"),
		metric.WithDescription("Perceived silence between the end of caller speech and the first agent audio (G1)."),
	); err != nil {
		return nil, fmt.Errorf("obs: silence_gap_ms: %w", err)
	}
	if m.degLevel, err = meter.Int64Gauge("degradation_level",
		metric.WithDescription("Current degradation ladder level of a call (0=degNormal .. 4=degVoicemail)."),
	); err != nil {
		return nil, fmt.Errorf("obs: degradation_level: %w", err)
	}
	if m.degTransition, err = meter.Int64Counter("degradation_transitions_total",
		metric.WithDescription("Degradation ladder transitions by target level and reason."),
	); err != nil {
		return nil, fmt.Errorf("obs: degradation_transitions_total: %w", err)
	}
	if m.bargeInFP, err = meter.Int64Counter("barge_in_false_positive_total",
		metric.WithDescription("Duckings undone without a confirmed barge-in (backchannel or noise)."),
	); err != nil {
		return nil, fmt.Errorf("obs: barge_in_false_positive_total: %w", err)
	}
	if m.smsLatency, err = meter.Float64Histogram("sms_owner_latency_seconds",
		metric.WithUnit("s"),
		metric.WithDescription("Call end to carrier acceptance of the owner SMS (G4: p95 < 60s)."),
	); err != nil {
		return nil, fmt.Errorf("obs: sms_owner_latency_seconds: %w", err)
	}
	if m.orphans, err = meter.Int64Counter("recovery_orphan_total",
		metric.WithDescription("Calls found orphaned during post-restart recovery (fold/rehydration)."),
	); err != nil {
		return nil, fmt.Errorf("obs: recovery_orphan_total: %w", err)
	}
	return m, nil
}

// ObserveOwnerSMSLatency implements notify.Metrics.
func (m *Metrics) ObserveOwnerSMSLatency(d time.Duration) {
	m.smsLatency.Record(context.Background(), d.Seconds())
}

var _ notify.Metrics = (*Metrics)(nil)

// ObserveSilenceGap records one perceived-silence gap.
func (m *Metrics) ObserveSilenceGap(ctx context.Context, d time.Duration) {
	m.silenceGap.Record(ctx, d.Milliseconds())
}

// RecordRecoveryOrphans counts calls orphaned by a restart.
func (m *Metrics) RecordRecoveryOrphans(ctx context.Context, n int64) {
	m.orphans.Add(ctx, n)
}

// LadderHook returns the onChange callback for degrade.NewLadder: every
// transition bumps degradation_transitions_total{level,reason} and sets the
// degradation_level gauge to the new level.
func (m *Metrics) LadderHook() func(from, to degrade.Level, reason string) {
	return func(from, to degrade.Level, reason string) {
		ctx := context.Background()
		m.degLevel.Record(ctx, int64(to))
		m.degTransition.Add(ctx, 1, metric.WithAttributes(
			attribute.String("from", from.String()),
			attribute.String("level", to.String()),
			attribute.String("reason", reason),
		))
	}
}

// RegisterBreakers exports provider_breaker_state as an observable gauge over
// the process-wide breaker registry: one series per provider, value 0=closed,
// 1=open, 2=half_open, plus a state attribute for humans.
func (m *Metrics) RegisterBreakers(meter metric.Meter, reg *degrade.Registry) error {
	if m.breakerState == nil {
		var err error
		m.breakerState, err = meter.Int64ObservableGauge("provider_breaker_state",
			metric.WithDescription("Circuit-breaker state per provider (0=closed, 1=open, 2=half_open)."),
		)
		if err != nil {
			return fmt.Errorf("obs: provider_breaker_state: %w", err)
		}
	}
	_, err := meter.RegisterCallback(func(_ context.Context, o metric.Observer) error {
		for provider, state := range reg.States() {
			o.ObserveInt64(m.breakerState, int64(state), metric.WithAttributes(
				attribute.String("provider", provider),
				attribute.String("state", state.String()),
			))
		}
		return nil
	}, m.breakerState)
	if err != nil {
		return fmt.Errorf("obs: register breaker callback: %w", err)
	}
	return nil
}

// TurnHooks wraps next with the per-call turn instrumentation:
//
//   - silence_gap_ms: user turn closed (endpointing) → machine enters
//     stSpeaking (first agent audio is queued).
//   - barge_in_false_positive_total: ducking engaged (energy onset) and then
//     released without a confirmed barge-in — a backchannel or noise burst.
//
// Call it once per call: the returned Hooks carries that call's private state.
func (m *Metrics) TurnHooks(ctx context.Context, next turn.Hooks) turn.Hooks {
	s := &turnProbe{}
	wrapped := next
	wrapped.OnUserTurn = func(text string, loc event.Locale) {
		s.mu.Lock()
		s.turnClosed = m.now()
		s.mu.Unlock()
		if next.OnUserTurn != nil {
			next.OnUserTurn(text, loc)
		}
	}
	wrapped.OnStateChange = func(from, to turn.State) {
		if to == turn.StSpeaking {
			s.mu.Lock()
			if !s.turnClosed.IsZero() {
				m.silenceGap.Record(ctx, m.now().Sub(s.turnClosed).Milliseconds())
				s.turnClosed = time.Time{}
			}
			s.mu.Unlock()
		}
		if next.OnStateChange != nil {
			next.OnStateChange(from, to)
		}
	}
	wrapped.OnDucking = func(on bool) {
		s.mu.Lock()
		if on {
			s.ducked = true
		} else {
			if s.ducked {
				m.bargeInFP.Add(ctx, 1)
			}
			s.ducked = false
		}
		s.mu.Unlock()
		if next.OnDucking != nil {
			next.OnDucking(on)
		}
	}
	wrapped.OnBargeIn = func(text string) {
		s.mu.Lock()
		s.ducked = false // confirmed: the duck was a true positive
		s.mu.Unlock()
		if next.OnBargeIn != nil {
			next.OnBargeIn(text)
		}
	}
	// OnFiller counts as agent audio for the caller's ears: close the gap.
	wrapped.OnFiller = func(key audiobank.Key) {
		s.mu.Lock()
		if !s.turnClosed.IsZero() {
			m.silenceGap.Record(ctx, m.now().Sub(s.turnClosed).Milliseconds())
			s.turnClosed = time.Time{}
		}
		s.mu.Unlock()
		if next.OnFiller != nil {
			next.OnFiller(key)
		}
	}
	return wrapped
}

// turnProbe is the per-call state behind TurnHooks.
type turnProbe struct {
	mu         sync.Mutex
	turnClosed time.Time
	ducked     bool
}
