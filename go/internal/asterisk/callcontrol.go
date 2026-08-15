package asterisk

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/maarkn/frontdesk/internal/telnyx"
)

// CallControl implements the gateway's call-control contract
// (internal/telnyx.CallControl — the carrier-neutral surface of ADR-004)
// over ARI. The compile-time assertion below is the point of ADR-009: the
// second implementation proves the interface does not leak Telnyx.
//
// The callControlID of the contract is the ARI channel id.
type CallControl struct {
	ari             ARI
	transferContext string

	pbSeq atomic.Uint64

	mu sync.Mutex
	// playbacks tracks the ACTIVE playback ids per channel: ARI has no
	// "stop everything" verb, so FlushPlayback (the barge-in hard cut,
	// nota 06) is an explicit StopPlayback of each one.
	playbacks map[string][]string
}

var _ telnyx.CallControl = (*CallControl)(nil)

// NewCallControl returns a CallControl issuing commands through the given
// ARI client. transferContext is the dialplan context that executes warm
// transfers (extensions.conf: transfer-owner).
func NewCallControl(ari ARI, transferContext string) *CallControl {
	if transferContext == "" {
		transferContext = "transfer-owner"
	}
	return &CallControl{
		ari:             ari,
		transferContext: transferContext,
		playbacks:       make(map[string][]string),
	}
}

// Answer implements telnyx.CallControl.
func (c *CallControl) Answer(_ context.Context, channelID string) error {
	return c.ari.Answer(channelID)
}

// Hangup implements telnyx.CallControl.
func (c *CallControl) Hangup(_ context.Context, channelID string) error {
	c.forget(channelID)
	return c.ari.Hangup(channelID)
}

// Transfer implements telnyx.CallControl: the channel leaves Stasis back
// into the dialplan at (transferContext, to), where extensions.conf dials
// the destination over the trunk (warm transfer to the owner's cell is the
// product failover — ADR-006 §5).
func (c *CallControl) Transfer(_ context.Context, channelID, to string) error {
	c.forget(channelID)
	return c.ari.ContinueTo(channelID, c.transferContext, to, 1)
}

// StartRecording implements telnyx.CallControl (called only after
// consent.captured — the gate lives in the turn wiring).
func (c *CallControl) StartRecording(_ context.Context, channelID string) error {
	return c.ari.Record(channelID, "call-"+channelID)
}

// Playback implements telnyx.CallControl: plays a hosted asset with a
// generated playback id, tracked so FlushPlayback can stop it explicitly.
func (c *CallControl) Playback(_ context.Context, channelID, audioURL string) error {
	pbID := fmt.Sprintf("pb-%s-%d", channelID, c.pbSeq.Add(1))
	if err := c.ari.Play(channelID, pbID, audioURL); err != nil {
		return err
	}
	c.mu.Lock()
	c.playbacks[channelID] = append(c.playbacks[channelID], pbID)
	c.mu.Unlock()
	return nil
}

// FlushPlayback implements telnyx.CallControl. ARI has no queue-flush verb:
// the barge-in hard cut is an EXPLICIT StopPlayback for every playback still
// active on the channel (nota 06 — latency here is directly audible). All
// stops are attempted even if one fails; the first error is reported.
func (c *CallControl) FlushPlayback(_ context.Context, channelID string) error {
	c.mu.Lock()
	ids := c.playbacks[channelID]
	delete(c.playbacks, channelID)
	c.mu.Unlock()

	var firstErr error
	for _, id := range ids {
		if err := c.ari.StopPlayback(id); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	if firstErr != nil {
		return fmt.Errorf("asterisk: flush playback on %s: %w", channelID, firstErr)
	}
	return nil
}

// SendDTMF implements telnyx.CallControl.
func (c *CallControl) SendDTMF(_ context.Context, channelID, digits string) error {
	return c.ari.SendDTMF(channelID, digits)
}

// PlaybackFinished releases the bookkeeping of a playback that ended on its
// own (the wiring calls this on the ARI PlaybackFinished event, so the
// per-channel list only holds truly active playbacks).
func (c *CallControl) PlaybackFinished(channelID, playbackID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	ids := c.playbacks[channelID]
	for i, id := range ids {
		if id == playbackID {
			c.playbacks[channelID] = append(ids[:i], ids[i+1:]...)
			break
		}
	}
	if len(c.playbacks[channelID]) == 0 {
		delete(c.playbacks, channelID)
	}
}

// ActivePlaybacks returns the playback ids currently tracked for a channel
// (observability and tests).
func (c *CallControl) ActivePlaybacks(channelID string) []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.playbacks[channelID]...)
}

// forget drops all playback bookkeeping of a channel.
func (c *CallControl) forget(channelID string) {
	c.mu.Lock()
	delete(c.playbacks, channelID)
	c.mu.Unlock()
}
