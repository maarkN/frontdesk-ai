package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/maarkn/frontdesk/internal/billing"
	"github.com/maarkn/frontdesk/internal/domain"
	"github.com/maarkn/frontdesk/internal/event"
	"github.com/maarkn/frontdesk/internal/store"
	"github.com/maarkn/frontdesk/internal/tenantctx"
)

// Compile-time proof that both store implementations satisfy the
// consumer-defined interfaces of this package.
var (
	_ TenantStore      = (*store.Mem)(nil)
	_ DIDStore         = (*store.Mem)(nil)
	_ CallStore        = (*store.Mem)(nil)
	_ AppointmentStore = (*store.Mem)(nil)
	_ MessageStore     = (*store.Mem)(nil)

	_ TenantStore      = (*store.PG)(nil)
	_ DIDStore         = (*store.PG)(nil)
	_ CallStore        = (*store.PG)(nil)
	_ AppointmentStore = (*store.PG)(nil)
	_ MessageStore     = (*store.PG)(nil)
)

// testEnv wires a Server over the in-memory store with a fixed clock.
type testEnv struct {
	handler http.Handler
	mem     *store.Mem
	now     time.Time
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	mem := store.NewMem()
	now := time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC)
	srv := New(Config{
		Tenants:      mem,
		DIDs:         mem,
		Calls:        mem,
		Appointments: mem,
		Messages:     mem,
		Extractor:    &domain.StaticExtractor{Now: func() time.Time { return now }},
		Billing:      &billing.FakeStripe{Now: func() time.Time { return now }},
		Now:          func() time.Time { return now },
	})
	return &testEnv{handler: srv.Router(), mem: mem, now: now}
}

// do performs a JSON request; apiKey may be empty for anonymous calls.
func (e *testEnv) do(t *testing.T, method, path, apiKey string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		require.NoError(t, json.NewEncoder(&buf).Encode(body))
	}
	req := httptest.NewRequest(method, path, &buf)
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	rec := httptest.NewRecorder()
	e.handler.ServeHTTP(rec, req)
	return rec
}

// onboard creates a tenant through the public endpoint and returns the
// response body.
func (e *testEnv) onboard(t *testing.T, name string) createTenantResponse {
	t.Helper()
	rec := e.do(t, http.MethodPost, "/v1/tenants", "", map[string]any{
		"name":        name,
		"plan":        "starter",
		"ownerMobile": "+15145550100",
		"websiteUrl":  "https://" + name + ".example.ca",
	})
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	var resp createTenantResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	return resp
}

func TestOnboardingCreatesTenantKeyProfileAndTrial(t *testing.T) {
	env := newTestEnv(t)
	resp := env.onboard(t, "bobs-plumbing")

	assert.NotEmpty(t, resp.Tenant.ID)
	assert.Equal(t, domain.PlanStarter, resp.Tenant.Plan)
	assert.Equal(t, domain.ModeOverflow, resp.Tenant.Mode, "overflow is the default mode")
	assert.Equal(t, 15, resp.Tenant.OverflowSeconds)
	assert.Contains(t, resp.APIKey, "fdk_")
	require.NotNil(t, resp.Profile, "websiteUrl set => profile extracted")
	assert.NotEmpty(t, resp.Profile.FAQs)
	assert.Equal(t, billing.StatusTrialing, resp.Subscription.Status)
	assert.Equal(t, env.now.AddDate(0, 0, 14), resp.Subscription.TrialEndsAt)

	// The key authenticates.
	rec := env.do(t, http.MethodGet, "/v1/tenants/me", resp.APIKey, nil)
	require.Equal(t, http.StatusOK, rec.Code)
	var me domain.Tenant
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &me))
	assert.Equal(t, resp.Tenant.ID, me.ID)
}

func TestAuthRejectsMissingAndUnknownKeys(t *testing.T) {
	env := newTestEnv(t)
	env.onboard(t, "bobs-plumbing")

	rec := env.do(t, http.MethodGet, "/v1/calls", "", nil)
	assert.Equal(t, http.StatusUnauthorized, rec.Code, "missing key")

	rec = env.do(t, http.MethodGet, "/v1/calls", "fdk_deadbeef", nil)
	assert.Equal(t, http.StatusUnauthorized, rec.Code, "unknown key never falls back to a default tenant")
}

func TestModeToggle(t *testing.T) {
	env := newTestEnv(t)
	key := env.onboard(t, "bobs-plumbing").APIKey

	rec := env.do(t, http.MethodPut, "/v1/tenants/me/mode", key, map[string]any{"mode": "always_ai"})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var tenant domain.Tenant
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &tenant))
	assert.Equal(t, domain.ModeAlwaysAI, tenant.Mode)

	rec = env.do(t, http.MethodPut, "/v1/tenants/me/mode", key, map[string]any{"mode": "carrier-pigeon"})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestDIDsHappyPathAndConflict(t *testing.T) {
	env := newTestEnv(t)
	keyA := env.onboard(t, "bobs-plumbing").APIKey
	keyB := env.onboard(t, "acme-hvac").APIKey

	rec := env.do(t, http.MethodPost, "/v1/dids", keyA, map[string]any{"number": "+15145550001"})
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())

	// Same number for another tenant: conflict, not a silent takeover.
	rec = env.do(t, http.MethodPost, "/v1/dids", keyB, map[string]any{"number": "+15145550001"})
	assert.Equal(t, http.StatusConflict, rec.Code)

	rec = env.do(t, http.MethodGet, "/v1/dids", keyA, nil)
	require.Equal(t, http.StatusOK, rec.Code)
	var dids []domain.DID
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &dids))
	require.Len(t, dids, 1)
	assert.Equal(t, "telnyx", dids[0].Provider)
}

// seedCall stores a call projection for the tenant, bypassing HTTP (the
// projector that folds the bus into the store lives outside this EPIC).
func seedCall(t *testing.T, env *testEnv, tenantID, callID string, minutes int64) {
	t.Helper()
	ctx := tenantctx.WithTenant(t.Context(), tenantID)
	call := domain.Call{
		Snapshot: event.CallSnapshot{
			CallID:    callID,
			TenantID:  tenantID,
			Status:    event.StatusEnded,
			StartedAt: env.now,
			EndedAt:   env.now.Add(time.Duration(minutes) * time.Minute),
			EndReason: event.EndCompleted,
			Locale:    event.LocaleFRCA,
			Transcript: []event.TranscriptEntry{
				{Seq: 2, Speaker: event.SpeakerCaller, Text: "mon chauffe-eau fuit"},
				{Seq: 3, Speaker: event.SpeakerAgent, Text: "Je peux réserver une visite demain."},
			},
			Usage:   event.Usage{CallMinutes: minutes, STTSeconds: 42},
			LastSeq: 4,
		},
		RecordingURL: "https://recordings.example.ca/" + callID + ".ogg",
	}
	require.NoError(t, env.mem.PutCall(ctx, call))
}

func TestCallsListAndDetail(t *testing.T) {
	env := newTestEnv(t)
	respA := env.onboard(t, "bobs-plumbing")
	seedCall(t, env, respA.Tenant.ID, "call-1", 3)

	rec := env.do(t, http.MethodGet, "/v1/calls", respA.APIKey, nil)
	require.Equal(t, http.StatusOK, rec.Code)
	var list []map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &list))
	require.Len(t, list, 1)
	assert.Equal(t, "call-1", list[0]["callId"])
	assert.Equal(t, true, list[0]["recorded"])

	rec = env.do(t, http.MethodGet, "/v1/calls/call-1", respA.APIKey, nil)
	require.Equal(t, http.StatusOK, rec.Code)
	var detail domain.Call
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &detail))
	assert.Len(t, detail.Snapshot.Transcript, 2)
	assert.Contains(t, detail.RecordingURL, "call-1")

	rec = env.do(t, http.MethodGet, "/v1/calls/nope", respA.APIKey, nil)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestTenantIsolationAcrossTheAPI(t *testing.T) {
	env := newTestEnv(t)
	respA := env.onboard(t, "bobs-plumbing")
	respB := env.onboard(t, "acme-hvac")

	// Tenant A produces data of every kind.
	seedCall(t, env, respA.Tenant.ID, "call-a", 2)
	rec := env.do(t, http.MethodPost, "/v1/messages", respA.APIKey, map[string]any{
		"who": "Alice", "what": "leaking heater", "urgency": "high", "callbackPhone": "+15145550111",
	})
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	rec = env.do(t, http.MethodPost, "/v1/appointments", respA.APIKey, map[string]any{
		"customerName": "Alice", "service": "repair",
		"startsAt": "2026-08-14T09:00:00Z", "endsAt": "2026-08-14T10:00:00Z",
	})
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())

	// Tenant B sees none of it.
	for _, path := range []string{"/v1/calls", "/v1/messages", "/v1/appointments", "/v1/dids"} {
		rec := env.do(t, http.MethodGet, path, respB.APIKey, nil)
		require.Equal(t, http.StatusOK, rec.Code, path)
		var rows []json.RawMessage
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &rows))
		assert.Empty(t, rows, "tenant B must see zero rows at %s", path)
	}

	// Point read across tenants is a plain 404.
	rec = env.do(t, http.MethodGet, "/v1/calls/call-a", respB.APIKey, nil)
	assert.Equal(t, http.StatusNotFound, rec.Code)

	// Tenant A still sees its own data.
	rec = env.do(t, http.MethodGet, "/v1/messages", respA.APIKey, nil)
	require.Equal(t, http.StatusOK, rec.Code)
	var msgs []domain.Message
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &msgs))
	require.Len(t, msgs, 1)
	assert.Equal(t, domain.UrgencyHigh, msgs[0].Urgency)
}

func TestUsageFoldsCallsIntoInvoice(t *testing.T) {
	env := newTestEnv(t)
	resp := env.onboard(t, "bobs-plumbing")
	for i, minutes := range []int64{120, 90, 40} { // 250 min total
		seedCall(t, env, resp.Tenant.ID, fmt.Sprintf("call-%d", i), minutes)
	}

	rec := env.do(t, http.MethodGet, "/v1/usage", resp.APIKey, nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var usage usageResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &usage))

	assert.Equal(t, int64(250), usage.Usage.CallMinutes)
	assert.Equal(t, int64(50), usage.Invoice.OverageMinutes, "starter includes 200 min")
	assert.Equal(t, int64(50*35), usage.Invoice.OverageCents)
	assert.True(t, usage.Invoice.Trial, "onboarding starts inside the 14-day trial")
	assert.Zero(t, usage.Invoice.TotalCents, "trial charges nothing")
}
