package domain

import (
	"fmt"

	"github.com/maarkn/frontdesk/internal/event"
)

// Call is the read model of one phone call: the deterministic fold of its
// event stream (state = fold(events), ADR-003) plus the recording location.
type Call struct {
	// Snapshot is the folded state: status, transcript (already redacted at
	// emission), locale history, per-turn latencies and billable usage.
	Snapshot event.CallSnapshot `json:"snapshot"`
	// RecordingURL points at the (DEK-encrypted) audio in object storage;
	// empty when the call was not recorded.
	RecordingURL string `json:"recordingUrl,omitempty"`
}

// ID returns the call id from the folded snapshot.
func (c Call) ID() string { return c.Snapshot.CallID }

// CallFromEvents folds a single call's events into a Call. It inherits the
// fold's guarantees: duplicated and out-of-order deliveries are harmless.
func CallFromEvents(events []event.Envelope, recordingURL string) (Call, error) {
	snap, err := event.Fold(events)
	if err != nil {
		return Call{}, fmt.Errorf("call: fold events: %w", err)
	}
	return Call{Snapshot: snap, RecordingURL: recordingURL}, nil
}
