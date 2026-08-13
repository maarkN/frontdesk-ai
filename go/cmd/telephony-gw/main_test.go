package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/maarkn/frontdesk/internal/event"
	"github.com/maarkn/frontdesk/internal/telnyx"
)

func TestLoadConfig(t *testing.T) {
	env := map[string]string{
		"TELNYX_API_KEY": "key",
		"TENANT_DIDS":    "+15145550100=tenant-a, +15145550101=tenant-b",
		"DRAIN_TIMEOUT":  "45s",
		"OWNER_PHONE":    "+15145559999",
	}
	cfg, err := loadConfig(func(k string) string { return env[k] })
	require.NoError(t, err)
	require.Equal(t, ":8080", cfg.HTTPAddr)
	require.Equal(t, "tenant-a", cfg.TenantByDID["+15145550100"])
	require.Equal(t, "tenant-b", cfg.TenantByDID["+15145550101"])
	require.Equal(t, 45*time.Second, cfg.DrainTimeout)
	require.Equal(t, "+15145559999", cfg.OwnerPhone)
}

func TestLoadConfigRejectsMalformedDIDs(t *testing.T) {
	_, err := loadConfig(func(k string) string {
		if k == "TENANT_DIDS" {
			return "not-a-pair"
		}
		return ""
	})
	require.Error(t, err)
}

func newTestGateway(t *testing.T) (*gateway, *telnyx.FakeCallControl, *event.MemoryBus) {
	t.Helper()
	control := &telnyx.FakeCallControl{}
	bus := &event.MemoryBus{}
	gw := &gateway{
		cfg: config{
			TenantByDID: map[string]string{"+15145550100": "tenant-a"},
			OwnerPhone:  "+15145559999",
		},
		logger:   slog.New(slog.NewTextHandler(&strings.Builder{}, nil)),
		events:   bus,
		control:  control,
		sessions: newSessionRegistry(),
	}
	return gw, control, bus
}

func webhookBody(t *testing.T, eventType, callID, from, to string) *strings.Reader {
	t.Helper()
	body := map[string]any{"data": map[string]any{
		"event_type": eventType,
		"payload":    map[string]any{"call_control_id": callID, "from": from, "to": to},
	}}
	raw, err := json.Marshal(body)
	require.NoError(t, err)
	return strings.NewReader(string(raw))
}

// TestWebhookAnswersKnownDID: known DID → answer + call.started with the
// resolved tenant on the envelope; hangup webhook ends the session.
func TestWebhookAnswersKnownDID(t *testing.T) {
	gw, control, bus := newTestGateway(t)

	req := httptest.NewRequest("POST", "/telnyx/webhook",
		webhookBody(t, "call.initiated", "cc-1", "+14385550000", "+15145550100"))
	gw.handleWebhook(httptest.NewRecorder(), req)

	require.Eventually(t, func() bool { return len(control.Answered()) == 1 },
		2*time.Second, time.Millisecond)
	require.Eventually(t, func() bool { return len(bus.Events()) == 1 },
		2*time.Second, time.Millisecond)

	ev := bus.Events()[0]
	require.Equal(t, event.TypeCallStarted, ev.Type)
	require.Equal(t, "tenant-a", ev.TenantID, "tenant resolved from DID at the edge")
	require.Equal(t, "cc-1", ev.CallID)

	// Carrier hangup drains the session.
	req = httptest.NewRequest("POST", "/telnyx/webhook",
		webhookBody(t, "call.hangup", "cc-1", "", ""))
	gw.handleWebhook(httptest.NewRecorder(), req)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	require.True(t, gw.sessions.drain(ctx), "session ends on hangup")
	require.Zero(t, gw.sessions.active())
}

// TestWebhookUnknownDIDHangsUp: ADR-005 — no tenant, no call.
func TestWebhookUnknownDIDHangsUp(t *testing.T) {
	gw, control, bus := newTestGateway(t)

	req := httptest.NewRequest("POST", "/telnyx/webhook",
		webhookBody(t, "call.initiated", "cc-2", "+14385550000", "+19995550000"))
	gw.handleWebhook(httptest.NewRecorder(), req)

	require.Equal(t, []string{"cc-2"}, control.Hungup())
	require.Empty(t, control.Answered())
	require.Empty(t, bus.Events(), "no events for a tenantless call")
}

// TestDrainCancelsStragglers: shutdown never hangs forever on a stuck call.
func TestDrainCancelsStragglers(t *testing.T) {
	gw, _, _ := newTestGateway(t)

	req := httptest.NewRequest("POST", "/telnyx/webhook",
		webhookBody(t, "call.initiated", "cc-3", "+14385550000", "+15145550100"))
	gw.handleWebhook(httptest.NewRecorder(), req)
	require.Eventually(t, func() bool { return gw.sessions.active() == 1 },
		2*time.Second, time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	require.False(t, gw.sessions.drain(ctx), "timeout reported")
	require.Zero(t, gw.sessions.active(), "stragglers cancelled, not leaked")
}
