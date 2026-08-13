package notify

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"text/template"
	"time"

	"github.com/maarkn/frontdesk/internal/event"
	"github.com/maarkn/frontdesk/internal/tenantctx"
)

// Dedup key suffixes: each notification of one call.ended event is
// idempotent on its own, so a partial failure retries only what is missing.
const (
	dedupOwnerSuffix  = ":owner"
	dedupClientSuffix = ":client"
	dedupAggSuffix    = ":agg"
)

// Deps are the required collaborators of a Notifier. All fields except
// Metrics are mandatory.
type Deps struct {
	SMS     SMSSender
	Email   EmailSender
	Dedup   Deduper
	Tenants TenantDirectory
	// Metrics is optional; nil means NopMetrics.
	Metrics Metrics
}

// Option configures a Notifier (functional options, ADR-008).
type Option func(*Notifier)

// WithNow injects the clock, for deterministic tests.
func WithNow(now func() time.Time) Option {
	return func(n *Notifier) { n.now = now }
}

// WithTranscriptBaseURL sets the base URL of the dashboard transcript pages
// linked from the owner SMS (e.g. "https://app.frontdesk.ai"). Empty omits
// the link.
func WithTranscriptBaseURL(base string) Option {
	return func(n *Notifier) { n.transcriptBase = base }
}

// Notifier turns call.ended events into owner/client SMS and accumulates the
// per-tenant weekly report. It buffers each call's events in memory until
// call.ended arrives, then folds them into a Summary (state = fold(events)).
// HandleEvent is an event.Handler; it is safe for concurrent use.
type Notifier struct {
	sms            SMSSender
	email          EmailSender
	dedup          Deduper
	tenants        TenantDirectory
	metrics        Metrics
	tmpl           *template.Template
	now            func() time.Time
	transcriptBase string

	mu sync.Mutex
	// pending buffers events per call until call.ended. The bus may
	// redeliver events; Fold dedups by (Seq, EventID), so appending
	// duplicates is harmless.
	pending map[string][]event.Envelope
	// weekly accumulates the report counters per tenant, reset after each
	// successful weekly send.
	weekly map[string]WeeklyStats
}

var _ event.Handler = (*Notifier)(nil).HandleEvent

// WeeklyStats are the weekly report counters of one tenant.
type WeeklyStats struct {
	CallsAnswered int
	JobsBooked    int
	Messages      int
}

// New builds a Notifier. It fails when a required dependency is missing or
// the embedded templates do not parse.
func New(deps Deps, opts ...Option) (*Notifier, error) {
	switch {
	case deps.SMS == nil:
		return nil, errors.New("notify: Deps.SMS is required")
	case deps.Email == nil:
		return nil, errors.New("notify: Deps.Email is required")
	case deps.Dedup == nil:
		return nil, errors.New("notify: Deps.Dedup is required")
	case deps.Tenants == nil:
		return nil, errors.New("notify: Deps.Tenants is required")
	}

	tmpl, err := loadTemplates()
	if err != nil {
		return nil, err
	}

	n := &Notifier{
		sms:     deps.SMS,
		email:   deps.Email,
		dedup:   deps.Dedup,
		tenants: deps.Tenants,
		metrics: deps.Metrics,
		tmpl:    tmpl,
		now:     time.Now,
		pending: make(map[string][]event.Envelope),
		weekly:  make(map[string]WeeklyStats),
	}
	if n.metrics == nil {
		n.metrics = NopMetrics{}
	}
	for _, opt := range opts {
		opt(n)
	}
	return n, nil
}

// HandleEvent implements event.Handler. Non-terminal events are buffered;
// call.ended triggers the notifications. Returning an error leaves the
// message unacked so the bus redelivers it; the Deduper keeps redeliveries
// from notifying twice.
func (n *Notifier) HandleEvent(ctx context.Context, ev event.Envelope) error {
	n.mu.Lock()
	n.pending[ev.CallID] = append(n.pending[ev.CallID], ev)
	if ev.Type != event.TypeCallEnded {
		n.mu.Unlock()
		return nil
	}
	events := make([]event.Envelope, len(n.pending[ev.CallID]))
	copy(events, n.pending[ev.CallID])
	n.mu.Unlock()

	sum, err := BuildSummary(events)
	if err != nil {
		return err
	}

	// Tenant resolution point for this service (ADR-005): the envelope
	// carries the tenant; below this line only tenantctx is consulted.
	ctx = tenantctx.WithTenant(ctx, sum.TenantID)
	if err := n.notifyCallEnded(ctx, ev.EventID, sum); err != nil {
		return err
	}

	// Success: release the buffer. A later redelivery of the same
	// call.ended folds a sparse stream, but the dedup keys skip resends.
	n.mu.Lock()
	delete(n.pending, ev.CallID)
	n.mu.Unlock()
	return nil
}

// notifyCallEnded runs the three effects of one finished call in order; each
// one is individually idempotent by dedup key.
func (n *Notifier) notifyCallEnded(ctx context.Context, eventID string, sum Summary) error {
	info, err := n.tenants.Lookup(ctx)
	if err != nil {
		return fmt.Errorf("lookup tenant: %w", err)
	}
	if err := n.sendOwnerSMS(ctx, eventID, sum, info); err != nil {
		return err
	}
	if err := n.sendClientSMS(ctx, eventID, sum, info); err != nil {
		return err
	}
	return n.recordWeekly(ctx, eventID, sum)
}

// sendOwnerSMS sends the post-call summary to the business owner in the
// tenant's locale and records the end-to-carrier latency (G4 SLA <60s).
func (n *Notifier) sendOwnerSMS(ctx context.Context, eventID string, sum Summary, info TenantInfo) error {
	if info.OwnerPhone == "" {
		return nil
	}
	key := eventID + dedupOwnerSuffix
	seen, err := n.dedup.Seen(ctx, key)
	if err != nil {
		return fmt.Errorf("dedup owner SMS: %w", err)
	}
	if seen {
		return nil
	}

	loc := info.locale()
	tz := loadLocation(info.Timezone)
	data := ownerSMSData{
		Business:   info.Name,
		CallerName: sum.CallerName,
		Service:    sum.Service,
		Urgency:    urgencyLabel(loc, sum.Urgency),
		Phone:      sum.CallbackPhone,
		Booked:     sum.Outcome == OutcomeBooked,
		Message:    sum.Message,
		URL:        n.transcriptURL(sum.CallID),
	}
	if data.Booked {
		data.When = formatWhen(sum.AppointmentAt, loc, tz)
	}
	body, err := render(n.tmpl, templateName("owner_sms", loc), data)
	if err != nil {
		return err
	}
	if err := n.sms.Send(ctx, SMS{To: info.OwnerPhone, Body: body}); err != nil {
		return fmt.Errorf("send owner SMS: %w", err)
	}
	if !sum.EndedAt.IsZero() {
		n.metrics.ObserveOwnerSMSLatency(n.now().Sub(sum.EndedAt))
	}
	if err := n.dedup.Mark(ctx, key); err != nil {
		return fmt.Errorf("mark owner SMS: %w", err)
	}
	return nil
}

// sendClientSMS confirms the booking or the recorded message to the caller,
// in the locale detected during the call.
func (n *Notifier) sendClientSMS(ctx context.Context, eventID string, sum Summary, info TenantInfo) error {
	if sum.CallerPhone == "" {
		return nil
	}
	if sum.Outcome != OutcomeBooked && sum.Outcome != OutcomeMessage {
		return nil
	}
	key := eventID + dedupClientSuffix
	seen, err := n.dedup.Seen(ctx, key)
	if err != nil {
		return fmt.Errorf("dedup client SMS: %w", err)
	}
	if seen {
		return nil
	}

	loc := sum.Locale
	if loc != event.LocaleFRCA {
		loc = event.LocaleENCA
	}
	data := clientSMSData{
		Business: info.Name,
		Booked:   sum.Outcome == OutcomeBooked,
	}
	if data.Booked {
		data.When = formatWhen(sum.AppointmentAt, loc, loadLocation(info.Timezone))
	}
	body, err := render(n.tmpl, templateName("client_sms", loc), data)
	if err != nil {
		return err
	}
	if err := n.sms.Send(ctx, SMS{To: sum.CallerPhone, Body: body}); err != nil {
		return fmt.Errorf("send client SMS: %w", err)
	}
	if err := n.dedup.Mark(ctx, key); err != nil {
		return fmt.Errorf("mark client SMS: %w", err)
	}
	return nil
}

// recordWeekly folds one finished call into the tenant's weekly counters,
// exactly once per event.
func (n *Notifier) recordWeekly(ctx context.Context, eventID string, sum Summary) error {
	key := eventID + dedupAggSuffix
	seen, err := n.dedup.Seen(ctx, key)
	if err != nil {
		return fmt.Errorf("dedup weekly agg: %w", err)
	}
	if seen {
		return nil
	}

	n.mu.Lock()
	st := n.weekly[sum.TenantID]
	st.CallsAnswered++
	switch sum.Outcome {
	case OutcomeBooked:
		st.JobsBooked++
	case OutcomeMessage:
		st.Messages++
	}
	n.weekly[sum.TenantID] = st
	n.mu.Unlock()

	if err := n.dedup.Mark(ctx, key); err != nil {
		return fmt.Errorf("mark weekly agg: %w", err)
	}
	return nil
}

// transcriptURL builds the dashboard link for a call, or "" when no base URL
// is configured.
func (n *Notifier) transcriptURL(callID string) string {
	if n.transcriptBase == "" || callID == "" {
		return ""
	}
	return n.transcriptBase + "/calls/" + callID
}
