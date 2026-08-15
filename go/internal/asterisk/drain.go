package asterisk

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// ErrDrainTimeout reports that the maxDrainSec ceiling elapsed with calls
// still active (they will be cut when the process exits — the ceiling is the
// deliberate trade-off of nota 07).
var ErrDrainTimeout = errors.New("asterisk: drain ceiling reached with active calls")

// Drainer implements the blue/green drain of nota 07: after the dialplan
// global ACTIVE_APP points at the next Stasis app, the old process flips its
// readiness probe to not-ready and stays alive until its active calls end,
// capped by a ceiling. New calls stop arriving on their own — Asterisk
// routes them to the new app name.
//
// The zero value is NOT usable; call NewDrainer.
type Drainer struct {
	mu       sync.Mutex
	active   map[string]struct{}
	draining bool
	empty    chan struct{} // closed when the last active call ends while draining
}

// NewDrainer returns a ready Drainer.
func NewDrainer() *Drainer {
	return &Drainer{active: make(map[string]struct{})}
}

// CallStarted registers an active call (StasisStart of the app).
func (d *Drainer) CallStarted(channelID string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.active[channelID] = struct{}{}
}

// CallEnded releases a call (StasisEnd / hangup). Ending the last call while
// draining releases Drain.
func (d *Drainer) CallEnded(channelID string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.active, channelID)
	if d.draining && len(d.active) == 0 && d.empty != nil {
		close(d.empty)
		d.empty = nil
	}
}

// Active returns the number of active calls (the drain gauge).
func (d *Drainer) Active() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.active)
}

// Ready is the readiness probe: false from the moment draining starts, so
// the orchestrator stops routing to this instance while the process stays
// alive to finish its calls.
func (d *Drainer) Ready() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return !d.draining
}

// Drain flips readiness to not-ready and blocks until every active call
// ends, the maxDrain ceiling elapses (ErrDrainTimeout), or ctx is done. It
// is safe to call once per process lifetime.
func (d *Drainer) Drain(ctx context.Context, maxDrain time.Duration) error {
	d.mu.Lock()
	d.draining = true
	if len(d.active) == 0 {
		d.mu.Unlock()
		return nil
	}
	if d.empty == nil {
		d.empty = make(chan struct{})
	}
	empty := d.empty
	d.mu.Unlock()

	timer := time.NewTimer(maxDrain)
	defer timer.Stop()
	select {
	case <-empty:
		return nil
	case <-timer.C:
		return fmt.Errorf("%w: %d call(s) after %s", ErrDrainTimeout, d.Active(), maxDrain)
	case <-ctx.Done():
		return fmt.Errorf("asterisk: drain interrupted: %w", ctx.Err())
	}
}
