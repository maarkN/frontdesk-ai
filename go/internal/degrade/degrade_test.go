package degrade_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"github.com/maarkn/frontdesk/internal/degrade"
	"github.com/maarkn/frontdesk/internal/media"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

// --- ladder ---

func TestLadderWalksDownAndUp(t *testing.T) {
	var transitions []string
	l := degrade.NewLadder(func(from, to degrade.Level, reason string) {
		transitions = append(transitions, from.String()+"->"+to.String())
	})

	require.Equal(t, degrade.Normal, l.Level())
	require.Equal(t, degrade.Fast, l.Escalate("llm timeout"))
	require.Equal(t, degrade.Scripted, l.Escalate("llm timeout again"))
	require.Equal(t, degrade.Fast, l.Relax("provider recovered"))
	require.Equal(t, degrade.Transfer, l.EscalateTo(degrade.Transfer, "all providers failed"))
	require.Equal(t,
		[]string{
			"degNormal->degFast",
			"degFast->degScripted",
			"degScripted->degFast",
			"degFast->degTransfer",
		},
		transitions)
}

func TestLadderFloorsAtVoicemail(t *testing.T) {
	var l degrade.Ladder // zero value: Normal
	for range 10 {
		l.Escalate("boom")
	}
	require.Equal(t, degrade.Voicemail, l.Level(), "the floor is voicemail, never past it")
}

// --- breaker ---

func TestBreakerOpensAndProbes(t *testing.T) {
	now := time.Unix(0, 0)
	b := degrade.NewBreaker(
		degrade.WithMaxFailures(3),
		degrade.WithProbeInterval(5*time.Second),
		degrade.WithClock(func() time.Time { return now }),
	)

	for range 3 {
		require.True(t, b.Allow())
		b.Failure()
	}
	require.Equal(t, degrade.BreakerOpen, b.State())
	require.False(t, b.Allow(), "open breaker refuses immediately")

	now = now.Add(5 * time.Second)
	require.True(t, b.Allow(), "one probe after the interval")
	require.False(t, b.Allow(), "second caller in the same window is refused")

	b.Failure() // probe failed → reopen, clock restarts
	require.False(t, b.Allow())
	now = now.Add(5 * time.Second)
	require.True(t, b.Allow())
	b.Success()
	require.Equal(t, degrade.BreakerClosed, b.State())
	require.True(t, b.Allow())
}

// TestBreakerPreventsRetryStorm mirrors US-6.4: 200 concurrent calls with the
// provider down must produce attempts bounded by the probe cadence, not
// 200 × 3 reconnections.
func TestBreakerPreventsRetryStorm(t *testing.T) {
	var mu sync.Mutex
	now := time.Unix(0, 0)
	clock := func() time.Time { mu.Lock(); defer mu.Unlock(); return now }

	reg := degrade.NewRegistry(
		degrade.WithMaxFailures(3),
		degrade.WithProbeInterval(5*time.Second),
		degrade.WithClock(clock),
	)
	br := reg.For("deepgram")

	// Provider goes down: the first consecutive failures open the breaker.
	for range 3 {
		require.True(t, br.Allow())
		br.Failure()
	}

	var attempts atomic.Int64
	tryOnce := func() {
		if br.Allow() {
			attempts.Add(1)
			br.Failure() // provider still down
		}
	}

	// 200 active calls all hit the dead provider inside the same window.
	var wg sync.WaitGroup
	for range 200 {
		wg.Go(tryOnce)
	}
	wg.Wait()
	require.Zero(t, attempts.Load(), "no probe before the interval elapses")

	// Advance one probe window: exactly ONE probe goes through.
	mu.Lock()
	now = now.Add(5 * time.Second)
	mu.Unlock()
	for range 200 {
		wg.Go(tryOnce)
	}
	wg.Wait()
	require.Equal(t, int64(1), attempts.Load(), "one probe per 5s for the whole provider")

	require.Equal(t, map[string]degrade.BreakerState{"deepgram": degrade.BreakerOpen}, reg.States())
}

// --- hedge ---

// slowCompleter answers text after d, or fails fast when err is set.
func slowCompleter(text string, d time.Duration, err error) degrade.CompleterFunc {
	return func(ctx context.Context, _ string) (string, error) {
		if err != nil {
			return "", err
		}
		select {
		case <-time.After(d):
			return text, nil
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
}

func TestHedgeFastPrimaryWinsWithoutHedging(t *testing.T) {
	var secondaryCalls atomic.Int64
	secondary := degrade.CompleterFunc(func(ctx context.Context, p string) (string, error) {
		secondaryCalls.Add(1)
		return "secondary", nil
	})
	h := degrade.NewHedge(
		slowCompleter("primary", time.Millisecond, nil),
		secondary,
		degrade.WithHedgeAfter(200*time.Millisecond),
	)

	got, err := h.Complete(context.Background(), "hi")
	require.NoError(t, err)
	require.Equal(t, "primary", got)
	require.Zero(t, secondaryCalls.Load(), "fast primary must not hedge")
}

func TestHedgeSlowPrimaryLosesToSecondary(t *testing.T) {
	var hedged atomic.Int64
	h := degrade.NewHedge(
		slowCompleter("primary", time.Second, nil), // way past the hedge point
		slowCompleter("secondary", time.Millisecond, nil),
		degrade.WithHedgeAfter(20*time.Millisecond),
		degrade.WithOnHedge(func() { hedged.Add(1) }),
	)

	start := time.Now()
	got, err := h.Complete(context.Background(), "hi")
	require.NoError(t, err)
	require.Equal(t, "secondary", got, "first responder wins")
	require.GreaterOrEqual(t, time.Since(start), 20*time.Millisecond, "secondary only after hedgeAfter")
	require.Less(t, time.Since(start), 500*time.Millisecond, "winner is not delayed by the loser")
	require.Equal(t, int64(1), hedged.Load())
}

func TestHedgeFailingPrimaryFiresSecondaryImmediately(t *testing.T) {
	errBoom := errors.New("429")
	h := degrade.NewHedge(
		slowCompleter("", 0, errBoom),
		slowCompleter("secondary", time.Millisecond, nil),
		degrade.WithHedgeAfter(10*time.Second), // timer must NOT be waited on
	)

	start := time.Now()
	got, err := h.Complete(context.Background(), "hi")
	require.NoError(t, err)
	require.Equal(t, "secondary", got)
	require.Less(t, time.Since(start), time.Second, "failure fires the hedge now, not after the timer")
}

func TestHedgeAllProvidersFailed(t *testing.T) {
	errA, errB := errors.New("timeout A"), errors.New("timeout B")
	h := degrade.NewHedge(
		slowCompleter("", 0, errA),
		slowCompleter("", 0, errB),
		degrade.WithHedgeAfter(time.Millisecond),
	)

	_, err := h.Complete(context.Background(), "hi")
	require.ErrorIs(t, err, degrade.ErrAllProvidersFailed)
	require.ErrorIs(t, err, errA)
	require.ErrorIs(t, err, errB)
}

func TestHedgeSkipsProviderWithOpenBreaker(t *testing.T) {
	now := time.Unix(0, 0)
	reg := degrade.NewRegistry(
		degrade.WithMaxFailures(1),
		degrade.WithClock(func() time.Time { return now }),
	)
	reg.For("primary").Failure() // primary breaker open

	h := degrade.NewHedge(
		slowCompleter("primary", time.Millisecond, nil),
		slowCompleter("secondary", time.Millisecond, nil),
		degrade.WithHedgeAfter(10*time.Second),
		degrade.WithBreakers(reg, "primary", "secondary"),
	)

	start := time.Now()
	got, err := h.Complete(context.Background(), "hi")
	require.NoError(t, err)
	require.Equal(t, "secondary", got, "open breaker skips straight to the fallback")
	require.Less(t, time.Since(start), time.Second)
}

// --- stt ring ---

// fakeSTTConn records frames and fails on demand.
type fakeSTTConn struct {
	mu     sync.Mutex
	frames []uint64
	fail   bool
	closed bool
}

func (c *fakeSTTConn) Send(f media.Frame) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.fail {
		return errors.New("ws closed")
	}
	c.frames = append(c.frames, f.Seq)
	return nil
}

func (c *fakeSTTConn) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed = true
	return nil
}

func (c *fakeSTTConn) seqs() []uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]uint64(nil), c.frames...)
}

func (c *fakeSTTConn) die() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.fail = true
}

func TestSTTRingReplaysAfterReconnect(t *testing.T) {
	var slept []time.Duration
	sleep := func(_ context.Context, d time.Duration) error {
		slept = append(slept, d)
		return nil
	}

	first := &fakeSTTConn{}
	second := &fakeSTTConn{}
	conns := []degrade.STTConn{first, second}
	dialErrs := []error{nil, errors.New("dial refused"), errors.New("dial refused"), nil}
	dial := 0
	factory := func(ctx context.Context) (degrade.STTConn, error) {
		err := dialErrs[dial]
		if err != nil {
			dial++
			return nil, err
		}
		conn := conns[0]
		conns = conns[1:]
		dial++
		return conn, nil
	}

	r := degrade.NewSTTRing([]degrade.STTFactory{factory}, degrade.WithSleep(sleep))
	ctx := context.Background()

	// Frames 0..9 flow on the first connection.
	for seq := uint64(0); seq < 10; seq++ {
		require.NoError(t, r.Send(ctx, media.Silence(seq)))
	}
	require.Equal(t, []uint64{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}, first.seqs())

	// Connection dies; the next Send reconnects (two dial failures, then
	// success) and replays the frame buffered during the outage.
	first.die()
	require.NoError(t, r.Send(ctx, media.Silence(10)))
	require.True(t, first.closed, "dead connection is closed")
	require.Equal(t, []uint64{10}, second.seqs(), "no audio lost: outage frame replayed")
	require.Equal(t, []time.Duration{80 * time.Millisecond, 200 * time.Millisecond}, slept,
		"retry schedule is 0/80/200ms, never exponential")

	require.NoError(t, r.Send(ctx, media.Silence(11)))
	require.Equal(t, []uint64{10, 11}, second.seqs())
	require.NoError(t, r.Close())
}

func TestSTTRingBuffersWhileDownAndFallsBack(t *testing.T) {
	sleep := func(context.Context, time.Duration) error { return nil }

	deadFactory := func(ctx context.Context) (degrade.STTConn, error) {
		return nil, errors.New("provider down")
	}
	fallbackConn := &fakeSTTConn{}
	fallbackDials := 0
	fallbackFactory := func(ctx context.Context) (degrade.STTConn, error) {
		fallbackDials++
		if fallbackDials < 3 {
			return nil, errors.New("fallback warming up")
		}
		return fallbackConn, nil
	}

	r := degrade.NewSTTRing(
		[]degrade.STTFactory{deadFactory, fallbackFactory},
		degrade.WithSleep(sleep),
	)
	require.NoError(t, r.Send(context.Background(), media.Silence(0)))
	require.Equal(t, []uint64{0}, fallbackConn.seqs(), "fallback provider got the audio")
	require.Equal(t, 6, r.Reconnects(), "3 attempts on primary + 3 on fallback")
}

func TestSTTRingTotalFailureKeepsCallAlive(t *testing.T) {
	sleep := func(context.Context, time.Duration) error { return nil }
	dead := func(ctx context.Context) (degrade.STTConn, error) {
		return nil, errors.New("provider down")
	}

	r := degrade.NewSTTRing([]degrade.STTFactory{dead, dead}, degrade.WithSleep(sleep))

	err := r.Send(context.Background(), media.Silence(0))
	require.ErrorIs(t, err, degrade.ErrSTTUnavailable,
		"caller escalates to degScripted on this error; the call lives")
	require.Equal(t, 1, r.Buffered(), "the frame stays buffered for a later recovery")
}
