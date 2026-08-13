package notify

import (
	"sync"
	"time"
)

// LatencyRecorder is a Metrics implementation that keeps every observation
// in memory — the fake used by tests and the local SLA check until EPIC-010
// plugs the real exporter. The zero value is ready to use.
type LatencyRecorder struct {
	mu      sync.Mutex
	samples []time.Duration
}

var _ Metrics = (*LatencyRecorder)(nil)

// ObserveOwnerSMSLatency implements Metrics.
func (r *LatencyRecorder) ObserveOwnerSMSLatency(d time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.samples = append(r.samples, d)
}

// Samples returns a copy of the recorded latencies, in observation order.
func (r *LatencyRecorder) Samples() []time.Duration {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]time.Duration, len(r.samples))
	copy(out, r.samples)
	return out
}
