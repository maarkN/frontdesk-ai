// Package notify implements the FrontDesk notifier (EPIC-005): it consumes
// call.ended events from the bus, folds the call's event stream into a
// structured summary (who called, what they want, urgency, callback phone,
// booking or message) and sends the owner SMS (<60s SLA, measured and
// exported as a metric), the caller confirmation SMS in the locale detected
// during the call, and the weekly per-tenant report e-mail — the "anti-churn"
// report of RF3/G4.
//
// Every external effect sits behind a small consumer-defined interface
// (SMSSender, EmailSender, Deduper, TenantDirectory, Metrics), so providers
// are swappable per ADR-008 and tests run entirely on fakes with no network.
// Message content is assembled from events that were already PII-redacted at
// emission time (ADR-003); this package never sees raw PII.
package notify

import (
	"context"
	"time"

	"github.com/maarkn/frontdesk/internal/event"
)

// SMS is one outbound text message.
type SMS struct {
	// To is the destination number in E.164.
	To string
	// Body is the rendered message text.
	Body string
}

// SMSSender delivers SMS messages. Implementations must be safe for
// concurrent use and should return an error only when the carrier did not
// accept the message (the notifier retries via bus redelivery).
type SMSSender interface {
	Send(ctx context.Context, msg SMS) error
}

// Email is one outbound e-mail message.
type Email struct {
	To      string
	Subject string
	Body    string
}

// EmailSender delivers e-mail messages. Same contract as SMSSender.
type EmailSender interface {
	Send(ctx context.Context, msg Email) error
}

// Deduper is the persistable idempotency store: a notification key is marked
// after a successful send and checked before every attempt, so bus
// redeliveries never notify the same recipient twice. Implementations may be
// in-memory (tests, single instance) or database-backed (insert ... on
// conflict do nothing).
type Deduper interface {
	// Seen reports whether key was already marked.
	Seen(ctx context.Context, key string) (bool, error)
	// Mark records key as done. Marking an existing key is a no-op.
	Mark(ctx context.Context, key string) error
}

// TenantInfo is what the notifier needs to know about a tenant. It comes
// from the core-api projections (EPIC-004); MemoryDirectory serves it in
// tests and local runs.
type TenantInfo struct {
	// Name is the business name shown in messages.
	Name string `json:"name"`
	// OwnerPhone receives the post-call SMS (E.164). Empty disables it.
	OwnerPhone string `json:"ownerPhone"`
	// OwnerEmail receives the weekly report. Empty disables it.
	OwnerEmail string `json:"ownerEmail"`
	// Locale is the owner's language for owner-facing messages.
	Locale event.Locale `json:"locale"`
	// Timezone is the IANA zone used to format times and pick the report
	// week. Empty means UTC.
	Timezone string `json:"timezone"`
	// AvgJobValueCAD estimates the value of one booked job for the weekly
	// report ("valor estimado").
	AvgJobValueCAD float64 `json:"avgJobValueCad"`
}

// locale returns the owner locale, defaulting to en-CA.
func (i TenantInfo) locale() event.Locale {
	if i.Locale == event.LocaleFRCA {
		return event.LocaleFRCA
	}
	return event.LocaleENCA
}

// TenantDirectory resolves tenant data. Lookup reads the tenant from the
// context (tenantctx, ADR-005 — no tenantID parameters below the resolution
// point); TenantIDs lists every known tenant for the weekly report sweep.
type TenantDirectory interface {
	Lookup(ctx context.Context) (TenantInfo, error)
	TenantIDs(ctx context.Context) ([]string, error)
}

// Metrics receives the notifier's measurements. EPIC-010 plugs the real
// exporter; NopMetrics and LatencyRecorder cover wiring and tests.
type Metrics interface {
	// ObserveOwnerSMSLatency records sms_owner_latency_seconds: call end
	// to carrier acceptance of the owner SMS (G4: p95 < 60s).
	ObserveOwnerSMSLatency(d time.Duration)
}

// NopMetrics discards every observation. The zero value is ready to use.
type NopMetrics struct{}

var _ Metrics = NopMetrics{}

// ObserveOwnerSMSLatency implements Metrics.
func (NopMetrics) ObserveOwnerSMSLatency(time.Duration) {}
