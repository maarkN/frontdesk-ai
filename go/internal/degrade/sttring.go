package degrade

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/maarkn/frontdesk/internal/media"
)

// ErrSTTUnavailable reports that every reconnection attempt on every provider
// failed. The call does NOT die on it: the turn loop escalates to Scripted
// (canned + DTMF) and the call lives — DTMF arrives on the signaling path,
// independent of the audio pipeline.
var ErrSTTUnavailable = errors.New("degrade: stt unavailable after reconnects and fallback")

// STTConn is one live STT streaming connection.
type STTConn interface {
	Send(f media.Frame) error
	Close() error
}

// STTFactory dials one STT provider connection.
type STTFactory func(ctx context.Context) (STTConn, error)

// sttReconnectDelays is the fixed fast-retry schedule: 3 attempts at
// 0/80/200ms. NEVER exponential backoff — each wait is audible silence
// (ADR-006). After the schedule fails, the next provider gets the same
// schedule.
var sttReconnectDelays = [...]time.Duration{0, 80 * time.Millisecond, 200 * time.Millisecond}

// STTRing wraps STT connections with the resilience contract of ADR-006 §2:
//
//   - a ring buffer receives every frame ALWAYS, connected or not;
//   - on send failure it reconnects with the 0/80/200ms schedule, then falls
//     back to the next provider factory;
//   - after (re)connecting it replays the frames buffered since the last
//     frame the dead connection acknowledged, so no audio is lost to the
//     transcript;
//   - if everything fails, Send returns ErrSTTUnavailable and the caller
//     escalates the ladder — the call lives in Scripted.
//
// It is owned by the call goroutine; not safe for concurrent use.
type STTRing struct {
	factories []STTFactory
	capacity  int
	sleep     func(ctx context.Context, d time.Duration) error

	conn      STTConn
	buf       []media.Frame // frames not yet delivered to a live connection
	reconnect int           // total reconnection attempts (metrics/tests)
}

// STTRingOption configures an STTRing.
type STTRingOption func(*STTRing)

// WithRingCapacity sets how many frames the ring holds (default 500 = 10s).
func WithRingCapacity(frames int) STTRingOption {
	return func(r *STTRing) { r.capacity = frames }
}

// WithSleep injects the delay function (tests assert the 0/80/200 schedule
// without real waiting).
func WithSleep(fn func(ctx context.Context, d time.Duration) error) STTRingOption {
	return func(r *STTRing) { r.sleep = fn }
}

// NewSTTRing returns a ring over the provider factories in fallback order
// (primary first). At least one factory is required.
func NewSTTRing(factories []STTFactory, opts ...STTRingOption) *STTRing {
	r := &STTRing{
		factories: factories,
		capacity:  500,
		sleep:     ctxSleep,
	}
	for _, o := range opts {
		o(r)
	}
	return r
}

// Send buffers the frame unconditionally and delivers it (plus any backlog)
// to the live connection, reconnecting if needed.
func (r *STTRing) Send(ctx context.Context, f media.Frame) error {
	r.push(f)
	if r.conn == nil {
		if err := r.dial(ctx); err != nil {
			return err
		}
	}
	if err := r.flush(); err == nil {
		return nil
	}
	// Connection died mid-stream: drop it and run the reconnect ladder,
	// which replays the backlog on success.
	r.dropConn()
	if err := r.dial(ctx); err != nil {
		return err
	}
	if err := r.flush(); err != nil {
		r.dropConn()
		return fmt.Errorf("%w: replay failed: %w", ErrSTTUnavailable, err)
	}
	return nil
}

// Reconnects returns the number of reconnection attempts performed.
func (r *STTRing) Reconnects() int { return r.reconnect }

// Buffered returns how many frames are waiting for replay.
func (r *STTRing) Buffered() int { return len(r.buf) }

// Close closes the live connection, if any.
func (r *STTRing) Close() error {
	if r.conn == nil {
		return nil
	}
	err := r.conn.Close()
	r.conn = nil
	return err
}

// push appends to the ring, evicting the oldest frame when full. Eviction
// only happens during an outage longer than the ring (capacity frames): the
// oldest audio is sacrificed so the most recent speech survives the replay.
func (r *STTRing) push(f media.Frame) {
	if len(r.buf) >= r.capacity {
		r.buf = r.buf[1:]
	}
	r.buf = append(r.buf, f)
}

// flush delivers (replays) every buffered frame to the live connection, in
// order. Delivered frames leave the ring.
func (r *STTRing) flush() error {
	for len(r.buf) > 0 {
		if err := r.conn.Send(r.buf[0]); err != nil {
			return err
		}
		r.buf = r.buf[1:]
	}
	return nil
}

func (r *STTRing) dropConn() {
	if r.conn != nil {
		_ = r.conn.Close()
		r.conn = nil
	}
}

// dial walks providers in order, each with the 0/80/200ms schedule.
func (r *STTRing) dial(ctx context.Context) error {
	var errs []error
	for _, factory := range r.factories {
		for _, delay := range sttReconnectDelays {
			if delay > 0 {
				if err := r.sleep(ctx, delay); err != nil {
					return err
				}
			}
			r.reconnect++
			conn, err := factory(ctx)
			if err == nil {
				r.conn = conn
				return nil
			}
			errs = append(errs, err)
		}
	}
	return fmt.Errorf("%w: %w", ErrSTTUnavailable, errors.Join(errs...))
}

// ctxSleep sleeps for d or until ctx is done.
func ctxSleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
