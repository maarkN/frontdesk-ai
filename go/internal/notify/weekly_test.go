package notify

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/maarkn/frontdesk/internal/event"
)

func TestWeeklyReportAggregatesCalls(t *testing.T) {
	f := newFixture(t)
	f.tenants.Put("tenant-1", TenantInfo{
		Name:           "Plombex",
		OwnerPhone:     "+15140000001",
		OwnerEmail:     "owner@plombex.ca",
		Locale:         event.LocaleENCA,
		Timezone:       "America/Toronto",
		AvgJobValueCAD: 300,
	})
	f.tenants.Put("tenant-idle", TenantInfo{
		Name:       "Électricité Roy",
		OwnerEmail: "roy@example.ca",
		Locale:     event.LocaleFRCA,
	})

	// Three calls for tenant-1: two booked, one message.
	deliver(t, f.n, bookedCallEvents(t, "call-a", f.now.Add(-3*time.Hour)))
	deliver(t, f.n, bookedCallEvents(t, "call-b", f.now.Add(-2*time.Hour)))
	deliver(t, f.n, messageCallEvents(t, "call-c", f.now.Add(-time.Hour)))

	ctx := context.Background()
	require.NoError(t, f.n.SendWeeklyReports(ctx))

	sent := f.email.Sent()
	require.Len(t, sent, 2, "active + idle tenant")

	active := sent[0]
	assert.Equal(t, "owner@plombex.ca", active.To)
	assert.Equal(t, "Plombex — your AI receptionist this week", active.Subject)
	assert.Contains(t, active.Body, "answered 3 call(s)")
	assert.Contains(t, active.Body, "booked 2 job(s)")
	assert.Contains(t, active.Body, "took 1 message(s)")
	assert.Contains(t, active.Body, "CAD 600", "2 jobs x CAD 300 avg")

	idle := sent[1]
	assert.Equal(t, "roy@example.ca", idle.To)
	assert.Equal(t, "Électricité Roy — votre réceptionniste IA cette semaine", idle.Subject)
	assert.Contains(t, idle.Body, "Aucun appel cette semaine")

	// Same week: idempotent, nothing re-sent.
	require.NoError(t, f.n.SendWeeklyReports(ctx))
	assert.Len(t, f.email.Sent(), 2)

	// Next week with one new call: counters were reset after the send.
	f.now = f.now.AddDate(0, 0, 7)
	deliver(t, f.n, messageCallEvents(t, "call-d", f.now.Add(-time.Hour)))
	require.NoError(t, f.n.SendWeeklyReports(ctx))

	sent = f.email.Sent()
	require.Len(t, sent, 4)
	assert.Contains(t, sent[2].Body, "answered 1 call(s)")
	assert.Contains(t, sent[2].Body, "booked 0 job(s)")
	assert.Contains(t, sent[2].Body, "took 1 message(s)")
	assert.Contains(t, sent[2].Body, "CAD 0")
}

func TestWeeklyReportSkipsTenantWithoutEmail(t *testing.T) {
	f := newFixture(t)
	f.tenants.Put("tenant-1", TenantInfo{Name: "Plombex", OwnerPhone: "+15140000001"})

	deliver(t, f.n, bookedCallEvents(t, "call-x", f.now.Add(-time.Hour)))
	require.NoError(t, f.n.SendWeeklyReports(context.Background()))
	assert.Empty(t, f.email.Sent())
}
