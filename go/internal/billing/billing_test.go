package billing

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/maarkn/frontdesk/internal/domain"
	"github.com/maarkn/frontdesk/internal/event"
	"github.com/maarkn/frontdesk/internal/tenantctx"
)

// callEvents builds a minimal billable event stream for one call: started,
// one agent turn (tokens + TTS chars), stt.final (STT seconds), ended with
// the given duration.
func callEvents(t *testing.T, callID string, durationMs int64) []event.Envelope {
	t.Helper()
	now := time.Date(2026, 8, 13, 15, 0, 0, 0, time.UTC)

	mustJSON := func(v any) json.RawMessage {
		raw, err := json.Marshal(v)
		require.NoError(t, err)
		return raw
	}
	envelope := func(seq uint64, typ event.Type, payload json.RawMessage) event.Envelope {
		return event.Envelope{
			EventID:  event.NewEventID(now),
			CallID:   callID,
			Seq:      seq,
			TS:       now,
			TenantID: "tenant-a",
			FlowID:   "receptionist",
			Type:     typ,
			Payload:  payload,
		}
	}
	return []event.Envelope{
		envelope(1, event.TypeCallStarted, mustJSON(event.CallStarted{
			From: "+15145550111", To: "+15145550001", Mode: event.ModeOverflow,
		})),
		envelope(2, event.TypeSTTFinal, mustJSON(event.STTFinal{
			Text: "my water heater is leaking", AudioMs: 4_000,
		})),
		envelope(3, event.TypeAgentTurnCompleted, mustJSON(event.AgentTurnCompleted{
			Text: "I can book a visit tomorrow morning.", LLMTokensIn: 500, LLMTokensOut: 80, TTSChars: 38,
		})),
		envelope(4, event.TypeCallEnded, mustJSON(event.CallEnded{
			Reason: event.EndCompleted, DurationMs: durationMs,
		})),
	}
}

func TestUsageFromEventsFoldsPerCallAndSums(t *testing.T) {
	// Two calls: 90s → 2 billed minutes, 150s → 3 billed minutes (ceil).
	events := append(callEvents(t, "call-1", 90_000), callEvents(t, "call-2", 150_000)...)
	// Duplicate deliveries must not double-bill (at-least-once bus).
	events = append(events, callEvents(t, "call-1", 90_000)[3])

	usage, err := UsageFromEvents(events)
	require.NoError(t, err)

	assert.Equal(t, int64(5), usage.CallMinutes)
	assert.InDelta(t, 8.0, usage.STTSeconds, 0.001)
	assert.Equal(t, int64(1_000), usage.LLMTokensIn)
	assert.Equal(t, int64(160), usage.LLMTokensOut)
	assert.Equal(t, int64(76), usage.TTSChars)
}

func TestComputeInvoicePerPlan(t *testing.T) {
	usage := event.Usage{CallMinutes: 250}

	tests := []struct {
		name           string
		plan           domain.Plan
		wantBase       int64
		wantOverageMin int64
		wantTotal      int64
	}{
		{
			// 250 used - 200 included = 50 min * 35¢ = CAD 17.50 overage.
			name: "starter with overage", plan: domain.PlanStarter,
			wantBase: 14_900, wantOverageMin: 50, wantTotal: 16_650,
		},
		{
			name: "pro inside allowance", plan: domain.PlanPro,
			wantBase: 24_900, wantOverageMin: 0, wantTotal: 24_900,
		},
		{
			name: "growth inside allowance", plan: domain.PlanGrowth,
			wantBase: 39_900, wantOverageMin: 0, wantTotal: 39_900,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inv, err := Compute(tt.plan, usage, false)
			require.NoError(t, err)
			assert.Equal(t, tt.wantBase, inv.BaseCents)
			assert.Equal(t, tt.wantOverageMin, inv.OverageMinutes)
			assert.Equal(t, tt.wantOverageMin*OverageCentsPerMinute, inv.OverageCents)
			assert.Equal(t, tt.wantTotal, inv.TotalCents)
			assert.False(t, inv.Trial)
		})
	}
}

func TestComputeInvoiceTrialChargesNothing(t *testing.T) {
	inv, err := Compute(domain.PlanStarter, event.Usage{CallMinutes: 999}, true)
	require.NoError(t, err)
	assert.True(t, inv.Trial)
	assert.Zero(t, inv.TotalCents, "trial periods charge nothing")
	assert.NotZero(t, inv.OverageCents, "breakdown still reported during trial")
}

func TestComputeUnknownPlan(t *testing.T) {
	_, err := Compute(domain.Plan("enterprise"), event.Usage{}, false)
	assert.Error(t, err)
}

func TestFakeStripeTrialFourteenDays(t *testing.T) {
	now := time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC)
	fake := &FakeStripe{Now: func() time.Time { return now }}

	// No tenant in context → fail closed, never a default tenant.
	_, err := fake.CreateSubscription(context.Background(), domain.PlanPro)
	assert.ErrorIs(t, err, tenantctx.ErrNoTenant)

	ctx := tenantctx.WithTenant(context.Background(), "tenant-a")
	sub, err := fake.CreateSubscription(ctx, domain.PlanPro)
	require.NoError(t, err)
	assert.Equal(t, StatusTrialing, sub.Status)
	assert.Equal(t, now.AddDate(0, 0, 14), sub.TrialEndsAt)

	got, err := fake.Subscription(ctx)
	require.NoError(t, err)
	assert.Equal(t, sub, got)

	_, err = fake.CreateSubscription(ctx, domain.PlanPro)
	assert.ErrorIs(t, err, domain.ErrConflict)

	// Another tenant has no subscription: isolation via context.
	_, err = fake.Subscription(tenantctx.WithTenant(context.Background(), "tenant-b"))
	assert.ErrorIs(t, err, domain.ErrNotFound)
}
