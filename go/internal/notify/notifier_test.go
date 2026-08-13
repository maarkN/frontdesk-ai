package notify

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"github.com/maarkn/frontdesk/internal/event"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

// fixture bundles a Notifier with its fakes.
type fixture struct {
	n       *Notifier
	sms     *FakeSMS
	email   *FakeEmail
	dedup   *MemoryDedup
	tenants *MemoryDirectory
	metrics *LatencyRecorder
	now     time.Time
}

func newFixture(t *testing.T, opts ...Option) *fixture {
	t.Helper()
	f := &fixture{
		sms:     &FakeSMS{},
		email:   &FakeEmail{},
		dedup:   &MemoryDedup{},
		tenants: NewMemoryDirectory(),
		metrics: &LatencyRecorder{},
		now:     time.Date(2026, 8, 13, 15, 0, 30, 0, time.UTC),
	}
	all := append([]Option{WithNow(func() time.Time { return f.now })}, opts...)
	n, err := New(Deps{
		SMS:     f.sms,
		Email:   f.email,
		Dedup:   f.dedup,
		Tenants: f.tenants,
		Metrics: f.metrics,
	}, all...)
	require.NoError(t, err)
	f.n = n
	return f
}

// mkEvent builds an envelope for tenant tenant-1 / flow flow-1.
func mkEvent(t *testing.T, callID string, seq uint64, ts time.Time, typ event.Type, payload any) event.Envelope {
	t.Helper()
	ev, err := event.New(callID, seq, ts, "tenant-1", "flow-1", 1, typ, "", payload, nil)
	require.NoError(t, err)
	return ev
}

// varEvent builds a var.assigned envelope with a JSON string value.
func varEvent(t *testing.T, callID string, seq uint64, ts time.Time, name, value string) event.Envelope {
	t.Helper()
	raw, err := json.Marshal(value)
	require.NoError(t, err)
	return mkEvent(t, callID, seq, ts, event.TypeVarAssigned,
		event.VarAssigned{Name: name, Value: raw})
}

// bookedCallEvents is a call in FR-CA that ends with a booked appointment.
func bookedCallEvents(t *testing.T, callID string, endedAt time.Time) []event.Envelope {
	t.Helper()
	start := endedAt.Add(-3 * time.Minute)
	return []event.Envelope{
		mkEvent(t, callID, 1, start, event.TypeCallStarted,
			event.CallStarted{From: "+15145551234", To: "+15140009999", Mode: event.ModeOverflow}),
		mkEvent(t, callID, 2, start.Add(time.Second), event.TypeLanguageDetected,
			event.LanguageDetected{Locale: event.LocaleFRCA, Confidence: 0.98}),
		varEvent(t, callID, 3, start.Add(30*time.Second), VarCallerName, "Marie Tremblay"),
		varEvent(t, callID, 4, start.Add(40*time.Second), VarService, "water heater leak"),
		varEvent(t, callID, 5, start.Add(50*time.Second), VarUrgency, "high"),
		varEvent(t, callID, 6, start.Add(60*time.Second), VarAppointmentAt, "2026-08-14T13:00:00Z"),
		mkEvent(t, callID, 7, endedAt, event.TypeCallEnded,
			event.CallEnded{Reason: event.EndCompleted, DurationMs: 180_000}),
	}
}

// messageCallEvents is a call in EN-CA that ends with a message taken.
func messageCallEvents(t *testing.T, callID string, endedAt time.Time) []event.Envelope {
	t.Helper()
	start := endedAt.Add(-2 * time.Minute)
	return []event.Envelope{
		mkEvent(t, callID, 1, start, event.TypeCallStarted,
			event.CallStarted{From: "+16135550000", To: "+15140009999", Mode: event.ModeAlwaysAI}),
		mkEvent(t, callID, 2, start.Add(time.Second), event.TypeLanguageDetected,
			event.LanguageDetected{Locale: event.LocaleENCA, Confidence: 0.99}),
		varEvent(t, callID, 3, start.Add(20*time.Second), VarCallerName, "John Doe"),
		varEvent(t, callID, 4, start.Add(30*time.Second), VarService, "furnace maintenance"),
		varEvent(t, callID, 5, start.Add(40*time.Second), VarMessage, "call me tomorrow morning"),
		mkEvent(t, callID, 6, endedAt, event.TypeCallEnded,
			event.CallEnded{Reason: event.EndCallerHangup, DurationMs: 120_000}),
	}
}

func deliver(t *testing.T, n *Notifier, events []event.Envelope) {
	t.Helper()
	ctx := context.Background()
	for _, ev := range events {
		require.NoError(t, n.HandleEvent(ctx, ev))
	}
}

func TestCallEndedBookedFRCallENOwner(t *testing.T) {
	f := newFixture(t, WithTranscriptBaseURL("https://app.frontdesk.test"))
	f.tenants.Put("tenant-1", TenantInfo{
		Name:           "Plombex",
		OwnerPhone:     "+15140000001",
		OwnerEmail:     "owner@plombex.ca",
		Locale:         event.LocaleENCA,
		Timezone:       "America/Toronto",
		AvgJobValueCAD: 300,
	})

	endedAt := f.now.Add(-10 * time.Second)
	deliver(t, f.n, bookedCallEvents(t, "call-1", endedAt))

	sent := f.sms.Sent()
	require.Len(t, sent, 2, "owner + client SMS")

	owner := sent[0]
	assert.Equal(t, "+15140000001", owner.To)
	assert.Contains(t, owner.Body, "Plombex — new call from Marie Tremblay")
	assert.Contains(t, owner.Body, "about water heater leak")
	assert.Contains(t, owner.Body, "Urgency: high")
	assert.Contains(t, owner.Body, "Call back: +15145551234")
	// 2026-08-14T13:00Z is 09:00 in America/Toronto (EDT).
	assert.Contains(t, owner.Body, "Booked: Aug 14, 2026 at 9:00 AM")
	assert.Contains(t, owner.Body, "Transcript: https://app.frontdesk.test/calls/call-1")

	client := sent[1]
	assert.Equal(t, "+15145551234", client.To)
	assert.Contains(t, client.Body, "Plombex : votre rendez-vous est confirmé pour 14/08/2026 à 09h00")

	// SLA metric: end-of-call to carrier acceptance.
	samples := f.metrics.Samples()
	require.Len(t, samples, 1)
	assert.Equal(t, 10*time.Second, samples[0])
}

func TestCallEndedMessageENCallFROwner(t *testing.T) {
	f := newFixture(t)
	f.tenants.Put("tenant-1", TenantInfo{
		Name:       "Plomberie Roy",
		OwnerPhone: "+15140000002",
		Locale:     event.LocaleFRCA,
		Timezone:   "America/Montreal",
	})

	deliver(t, f.n, messageCallEvents(t, "call-2", f.now.Add(-5*time.Second)))

	sent := f.sms.Sent()
	require.Len(t, sent, 2)

	owner := sent[0]
	assert.Equal(t, "+15140000002", owner.To)
	assert.Contains(t, owner.Body, "Plomberie Roy — nouvel appel de John Doe")
	assert.Contains(t, owner.Body, "au sujet de furnace maintenance")
	assert.Contains(t, owner.Body, "Urgence : normale")
	assert.Contains(t, owner.Body, "Rappeler : +16135550000")
	assert.Contains(t, owner.Body, "Message : « call me tomorrow morning »")
	assert.NotContains(t, owner.Body, "Rendez-vous")

	client := sent[1]
	assert.Equal(t, "+16135550000", client.To)
	assert.Contains(t, client.Body, "Plomberie Roy: we received your message")
}

func TestDedupReplayDoesNotResend(t *testing.T) {
	f := newFixture(t)
	f.tenants.Put("tenant-1", TenantInfo{
		Name:       "Plombex",
		OwnerPhone: "+15140000001",
	})

	events := bookedCallEvents(t, "call-3", f.now.Add(-30*time.Second))
	deliver(t, f.n, events)
	require.Len(t, f.sms.Sent(), 2)

	// Redelivery of the terminal event alone (at-least-once bus).
	require.NoError(t, f.n.HandleEvent(context.Background(), events[len(events)-1]))
	// Full replay of the stream (consumer restart from seq 1).
	deliver(t, f.n, events)

	assert.Len(t, f.sms.Sent(), 2, "duplicates must not notify twice")
	assert.Len(t, f.metrics.Samples(), 1, "latency observed once")

	// The weekly aggregate counted the call exactly once too.
	f.n.mu.Lock()
	stats := f.n.weekly["tenant-1"]
	f.n.mu.Unlock()
	assert.Equal(t, WeeklyStats{CallsAnswered: 1, JobsBooked: 1}, stats)
}

func TestOwnerSMSFailureRetriesWithoutDuplicatingClient(t *testing.T) {
	f := newFixture(t)
	f.tenants.Put("tenant-1", TenantInfo{Name: "Plombex", OwnerPhone: "+15140000001"})

	events := bookedCallEvents(t, "call-4", f.now.Add(-5*time.Second))
	for _, ev := range events[:len(events)-1] {
		require.NoError(t, f.n.HandleEvent(context.Background(), ev))
	}

	f.sms.Err = assert.AnError
	require.Error(t, f.n.HandleEvent(context.Background(), events[len(events)-1]),
		"carrier failure must surface so the bus redelivers")
	require.Empty(t, f.sms.Sent())

	// Redelivery after the carrier recovers completes both sends once.
	f.sms.Err = nil
	require.NoError(t, f.n.HandleEvent(context.Background(), events[len(events)-1]))
	assert.Len(t, f.sms.Sent(), 2)
}

func TestMemoryBusIntegration(t *testing.T) {
	f := newFixture(t)
	f.tenants.Put("tenant-1", TenantInfo{Name: "Plombex", OwnerPhone: "+15140000001"})

	var bus event.MemoryBus
	bus.Handle(f.n.HandleEvent)

	ctx := context.Background()
	for _, ev := range bookedCallEvents(t, "call-5", f.now.Add(-time.Second)) {
		require.NoError(t, bus.Publish(ctx, ev))
	}
	assert.Len(t, f.sms.Sent(), 2)
}

func TestBuildSummarySparseStream(t *testing.T) {
	// A lone call.ended (buffer lost on restart) still folds.
	ended := mkEvent(t, "call-6", 9, time.Now(), event.TypeCallEnded,
		event.CallEnded{Reason: event.EndCompleted, DurationMs: 60_000})
	sum, err := BuildSummary([]event.Envelope{ended})
	require.NoError(t, err)
	assert.Equal(t, "call-6", sum.CallID)
	assert.Equal(t, "tenant-1", sum.TenantID)
	assert.Equal(t, OutcomeNone, sum.Outcome)
	assert.Empty(t, sum.CallerPhone)
}
