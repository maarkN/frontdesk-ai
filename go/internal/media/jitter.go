package media

// JitterBuffer reorders inbound frames by sequence number and releases them
// in order. It is deliberately simple (MVP, Telnyx WS is already TCP-ordered
// most of the time): a small reorder window absorbs bursts, late frames —
// older than the last released — are dropped, and gaps are conceded once the
// window fills so the stream never stalls.
//
// It is not safe for concurrent use; the transport goroutine owns it.
type JitterBuffer struct {
	window  int
	next    uint64
	started bool
	pending map[uint64]Frame
}

// NewJitterBuffer returns a buffer holding at most window out-of-order
// frames. window <= 0 means 5 (100ms).
func NewJitterBuffer(window int) *JitterBuffer {
	if window <= 0 {
		window = 5
	}
	return &JitterBuffer{window: window, pending: make(map[uint64]Frame)}
}

// Push inserts a frame and returns every frame now releasable, in order.
func (b *JitterBuffer) Push(f Frame) []Frame {
	if !b.started {
		b.started = true
		b.next = f.Seq
	}
	if f.Seq < b.next {
		return nil // too late: already released past it
	}
	b.pending[f.Seq] = f

	var out []Frame
	for {
		// Release the contiguous run starting at next.
		if f, ok := b.pending[b.next]; ok {
			out = append(out, f)
			delete(b.pending, b.next)
			b.next++
			continue
		}
		// Concede the gap once the window is exceeded: skip ahead to the
		// oldest pending frame instead of stalling the stream.
		if len(b.pending) > b.window {
			oldest := b.oldestPending()
			b.next = oldest
			continue
		}
		return out
	}
}

func (b *JitterBuffer) oldestPending() uint64 {
	first := true
	var min uint64
	for seq := range b.pending {
		if first || seq < min {
			min = seq
			first = false
		}
	}
	return min
}

// Len returns the number of buffered (not yet releasable) frames.
func (b *JitterBuffer) Len() int { return len(b.pending) }
