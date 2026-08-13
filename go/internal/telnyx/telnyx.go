// Package telnyx isolates the carrier behind the service boundary of
// ADR-004: the rest of the gateway sees call-control verbs and 20ms PCM
// frames, never the Telnyx API. Swapping the media layer in v2 (own
// Asterisk/RTPengine) stays confined to this package.
//
// Two surfaces are defined here and consumed by the call wiring:
//
//   - CallControl: the command side (answer, hangup, transfer, playback).
//   - MediaStream: the bidirectional audio of one call.
//
// Client is the real HTTP implementation of CallControl (Telnyx Call Control
// v2 actions). The media WebSocket transport is dial-injected so tests (and
// this build, which must not depend on the network) run on the in-memory
// Fake* implementations.
package telnyx

import (
	"context"

	"github.com/maarkn/frontdesk/internal/media"
)

// CallControl is the command surface of one carrier connection. All methods
// take the carrier's call control id.
type CallControl interface {
	// Answer answers an inbound call.
	Answer(ctx context.Context, callControlID string) error
	// Hangup terminates the call.
	Hangup(ctx context.Context, callControlID string) error
	// Transfer bridges the call to another number (warm transfer to the
	// owner's cell is the product failover — ADR-006 §5).
	Transfer(ctx context.Context, callControlID, to string) error
	// StartRecording begins call recording (only after consent.captured).
	StartRecording(ctx context.Context, callControlID string) error
	// Playback plays a hosted audio asset (canned phrases fallback when the
	// media stream is not available).
	Playback(ctx context.Context, callControlID, audioURL string) error
	// FlushPlayback stops everything queued on the line NOW — the barge-in
	// hard cut. Latency here is directly audible.
	FlushPlayback(ctx context.Context, callControlID string) error
	// SendDTMF plays DTMF digits on the line.
	SendDTMF(ctx context.Context, callControlID, digits string) error
}

// MediaStream is the audio of one call: inbound caller frames and outbound
// synthesized audio.
type MediaStream interface {
	// Frames delivers inbound 20ms frames in order (post jitter buffer).
	// The channel closes when the stream ends.
	Frames() <-chan media.Frame
	// Write queues outbound PCM for playout. It must not block the caller:
	// implementations buffer and report overflow as an error.
	Write(ctx context.Context, pcm []byte, durationMs int) error
	// Flush drops all queued outbound audio (barge-in cut on the media path).
	Flush(ctx context.Context) error
	// Close tears the stream down.
	Close() error
}

// DTMF is the signaling-path digit stream — independent of the audio
// pipeline by design (ADR-006: DTMF must survive STT outages).
type DTMF interface {
	Digits() <-chan string
}
