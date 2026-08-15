package obs_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/maarkn/frontdesk/internal/degrade"
	"github.com/maarkn/frontdesk/internal/event"
	"github.com/maarkn/frontdesk/internal/notify"
	"github.com/maarkn/frontdesk/internal/obs"
	"github.com/maarkn/frontdesk/internal/turn"
)

// testMeter returns a meter backed by an in-memory manual reader.
func testMeter(t *testing.T) (*sdkmetric.ManualReader, *sdkmetric.MeterProvider) {
	t.Helper()
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() { _ = mp.Shutdown(context.Background()) })
	return reader, mp
}

// collect snapshots all metrics and returns the named one.
func collect(t *testing.T, reader *sdkmetric.ManualReader, name string) (metricdata.Metrics, bool) {
	t.Helper()
	var rm metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(context.Background(), &rm))
	for _, scope := range rm.ScopeMetrics {
		for _, m := range scope.Metrics {
			if m.Name == name {
				return m, true
			}
		}
	}
	return metricdata.Metrics{}, false
}

func TestMetricsImplementsNotifyMetrics(t *testing.T) {
	reader, mp := testMeter(t)
	m, err := obs.NewMetrics(mp.Meter("test"))
	require.NoError(t, err)

	var nm notify.Metrics = m
	nm.ObserveOwnerSMSLatency(42 * time.Second)

	got, ok := collect(t, reader, "sms_owner_latency_seconds")
	require.True(t, ok)
	hist, ok := got.Data.(metricdata.Histogram[float64])
	require.True(t, ok)
	require.Len(t, hist.DataPoints, 1)
	assert.Equal(t, uint64(1), hist.DataPoints[0].Count)
	assert.InDelta(t, 42.0, hist.DataPoints[0].Sum, 0.001)
}

func TestLadderHookRecordsTransitions(t *testing.T) {
	reader, mp := testMeter(t)
	m, err := obs.NewMetrics(mp.Meter("test"))
	require.NoError(t, err)

	ladder := degrade.NewLadder(m.LadderHook())
	ladder.Escalate("stt timeout")
	ladder.Escalate("llm hedge lost")

	counter, ok := collect(t, reader, "degradation_transitions_total")
	require.True(t, ok)
	sum, ok := counter.Data.(metricdata.Sum[int64])
	require.True(t, ok)
	require.Len(t, sum.DataPoints, 2)
	var total int64
	for _, dp := range sum.DataPoints {
		total += dp.Value
	}
	assert.Equal(t, int64(2), total)

	gauge, ok := collect(t, reader, "degradation_level")
	require.True(t, ok)
	g, ok := gauge.Data.(metricdata.Gauge[int64])
	require.True(t, ok)
	require.Len(t, g.DataPoints, 1)
	assert.Equal(t, int64(degrade.Scripted), g.DataPoints[0].Value)
}

func TestRegisterBreakersExportsStatePerProvider(t *testing.T) {
	reader, mp := testMeter(t)
	meter := mp.Meter("test")
	m, err := obs.NewMetrics(meter)
	require.NoError(t, err)

	reg := degrade.NewRegistry(degrade.WithMaxFailures(1))
	reg.For("deepgram") // stays closed
	openBreaker := reg.For("cartesia")
	openBreaker.Failure() // opens with maxFailures=1

	require.NoError(t, m.RegisterBreakers(meter, reg))

	got, ok := collect(t, reader, "provider_breaker_state")
	require.True(t, ok)
	g, ok := got.Data.(metricdata.Gauge[int64])
	require.True(t, ok)
	require.Len(t, g.DataPoints, 2)

	states := map[string]int64{}
	for _, dp := range g.DataPoints {
		provider, _ := dp.Attributes.Value(attribute.Key("provider"))
		states[provider.AsString()] = dp.Value
	}
	assert.Equal(t, int64(degrade.BreakerClosed), states["deepgram"])
	assert.Equal(t, int64(degrade.BreakerOpen), states["cartesia"])
}

func TestTurnHooksSilenceGapAndFalsePositives(t *testing.T) {
	reader, mp := testMeter(t)
	clock := time.Unix(0, 0)
	m, err := obs.NewMetrics(mp.Meter("test"),
		obs.WithMetricsClock(func() time.Time { return clock }))
	require.NoError(t, err)

	var inner struct {
		userTurns int
		duckings  int
		bargeIns  int
	}
	hooks := m.TurnHooks(context.Background(), turn.Hooks{
		OnUserTurn: func(string, event.Locale) { inner.userTurns++ },
		OnDucking:  func(bool) { inner.duckings++ },
		OnBargeIn:  func(string) { inner.bargeIns++ },
	})

	// One full turn: user turn closes, 750ms later the machine speaks.
	hooks.OnUserTurn("i need a plumber", "")
	clock = clock.Add(750 * time.Millisecond)
	hooks.OnStateChange(turn.StThinking, turn.StSpeaking)
	// A later transition without a pending turn must not record again.
	hooks.OnStateChange(turn.StSpeaking, turn.StListening)

	// Backchannel: duck engaged then released without a confirmed barge-in.
	hooks.OnDucking(true)
	hooks.OnDucking(false)
	// True positive: duck confirmed by a real partial — no false positive.
	hooks.OnDucking(true)
	hooks.OnBargeIn("actually wait")
	hooks.OnDucking(false)

	gap, ok := collect(t, reader, "silence_gap_ms")
	require.True(t, ok)
	hist, ok := gap.Data.(metricdata.Histogram[int64])
	require.True(t, ok)
	require.Len(t, hist.DataPoints, 1)
	assert.Equal(t, uint64(1), hist.DataPoints[0].Count)
	assert.Equal(t, int64(750), hist.DataPoints[0].Sum)

	fp, ok := collect(t, reader, "barge_in_false_positive_total")
	require.True(t, ok)
	sum, ok := fp.Data.(metricdata.Sum[int64])
	require.True(t, ok)
	require.Len(t, sum.DataPoints, 1)
	assert.Equal(t, int64(1), sum.DataPoints[0].Value)

	// The wrapped hooks still reach the inner observer.
	assert.Equal(t, 1, inner.userTurns)
	assert.Equal(t, 4, inner.duckings)
	assert.Equal(t, 1, inner.bargeIns)
}

func TestRecordRecoveryOrphans(t *testing.T) {
	reader, mp := testMeter(t)
	m, err := obs.NewMetrics(mp.Meter("test"))
	require.NoError(t, err)

	m.RecordRecoveryOrphans(context.Background(), 3)

	got, ok := collect(t, reader, "recovery_orphan_total")
	require.True(t, ok)
	sum, ok := got.Data.(metricdata.Sum[int64])
	require.True(t, ok)
	require.Len(t, sum.DataPoints, 1)
	assert.Equal(t, int64(3), sum.DataPoints[0].Value)
}
