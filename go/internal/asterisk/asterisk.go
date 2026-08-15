// Package asterisk is the alternative media backend of the telephony gateway
// (ADR-009): call control via ARI (Stasis app) and audio via ARI External
// Media (RTP/UDP, slin16), behind the SAME call-control and media interfaces
// the Telnyx backend implements (internal/telnyx.CallControl / MediaStream).
// Telnyx remains the MVP default; this backend is selected with
// MEDIA_BACKEND=asterisk and is the living proof that those interfaces do
// not leak the carrier.
//
// The package also carries the two operational patterns of nota 07:
//
//   - Boot rehydration: the channel's source of truth is Asterisk. On boot,
//     list the live channels of the Stasis app; a channel with a snapshot is
//     resumed with a re-anchoring phrase from the audio bank; a channel
//     without one is transferred to the owner's cell ("recovered_orphan") —
//     never improvised.
//   - Blue/green drain: deploys switch the dialplan global ACTIVE_APP to the
//     new Stasis app name; the old process flips readiness to not-ready and
//     stays alive until its calls end, capped by maxDrainSec.
//
// Everything talks to Asterisk through the thin ARI interface below, so the
// whole package is testable against FakeARI with no network.
package asterisk

import (
	"context"
	"fmt"
	"time"
)

// ARI is the thin slice of the Asterisk REST Interface this backend uses.
// It is deliberately small and consumer-defined (uber-go/guide): the real
// implementation is Client (over github.com/CyCoreSystems/ari/v6), tests use
// FakeARI. Methods carry no context because ari/v6 calls are synchronous
// HTTP with the client's own timeout.
type ARI interface {
	// Answer answers the channel.
	Answer(channelID string) error
	// Hangup hangs the channel up with a normal cause.
	Hangup(channelID string) error
	// ContinueTo returns the channel to the dialplan at the given context
	// and extension (how a Stasis app hands a call to transfer-owner).
	ContinueTo(channelID, dialplanContext, extension string, priority int) error
	// Play starts a playback with a caller-chosen playback id, so it can be
	// stopped explicitly later.
	Play(channelID, playbackID, mediaURI string) error
	// StopPlayback stops one playback NOW. FlushPlayback (nota 06) is an
	// explicit StopPlayback of everything queued — there is no implicit
	// flush in ARI.
	StopPlayback(playbackID string) error
	// SendDTMF plays DTMF digits on the channel.
	SendDTMF(channelID, digits string) error
	// Record starts recording the channel into the named file.
	Record(channelID, name string) error
	// ExternalMedia creates the external media leg of a call: an RTP/UDP
	// stream of the given format towards externalHost (the gateway's
	// ExternalMediaServer).
	ExternalMedia(channelID, externalHost, format string) error
	// LiveChannels lists the channel ids currently owned by the Stasis app
	// (the boot-rehydration source of truth).
	LiveChannels() ([]string, error)
	// Close tears the ARI connection down.
	Close() error
}

// Config is the boot configuration of the Asterisk backend.
type Config struct {
	// URL is the ARI root, e.g. http://asterisk:8088/ari.
	URL string
	// WebsocketURL is the ARI event socket, e.g.
	// ws://asterisk:8088/ari/events. Empty derives it from URL.
	WebsocketURL string
	// Application is the Stasis app name (blue/green: each deploy color has
	// its own, e.g. frontdesk-v1 / frontdesk-v2).
	Application string
	// Username and Password authenticate against ari.conf.
	Username string
	Password string

	// TransferContext is the dialplan context executing warm transfers
	// (extensions.conf: transfer-owner).
	TransferContext string
	// ExternalMediaAddr is the host:port the gateway's ExternalMediaServer
	// listens on, as reachable FROM Asterisk.
	ExternalMediaAddr string
}

// validate rejects configs that cannot possibly work.
func (c Config) validate() error {
	if c.URL == "" {
		return fmt.Errorf("asterisk: config: URL is required")
	}
	if c.Application == "" {
		return fmt.Errorf("asterisk: config: Application is required")
	}
	return nil
}

// Backend bundles the ready-to-wire pieces of the Asterisk media backend.
// The telephony-gw main constructs one when MEDIA_BACKEND=asterisk and wires
// Control/Media where the Telnyx equivalents would go.
type Backend struct {
	// Control implements internal/telnyx.CallControl over ARI.
	Control *CallControl
	// Media is the External Media RTP server producing/consuming the same
	// 20ms frames as the Telnyx media stream.
	Media *ExternalMediaServer
	// Drainer implements the blue/green drain of nota 07.
	Drainer *Drainer

	ari ARI
}

// New dials ARI and builds a Backend ready for wiring. mediaListenAddr is
// the local UDP address the External Media server binds (e.g. ":4000");
// cfg.ExternalMediaAddr is that same socket as seen from Asterisk.
func New(ctx context.Context, cfg Config, mediaListenAddr string) (*Backend, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	client, err := Dial(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("asterisk: dial ARI: %w", err)
	}
	return NewBackendWith(client, cfg, mediaListenAddr)
}

// NewBackendWith builds a Backend on an existing ARI client (tests inject
// FakeARI here; New is the production path).
func NewBackendWith(client ARI, cfg Config, mediaListenAddr string) (*Backend, error) {
	media, err := NewExternalMediaServer(mediaListenAddr)
	if err != nil {
		return nil, fmt.Errorf("asterisk: external media server: %w", err)
	}
	return &Backend{
		Control: NewCallControl(client, cfg.TransferContext),
		Media:   media,
		Drainer: NewDrainer(),
		ari:     client,
	}, nil
}

// Close releases the backend's connections.
func (b *Backend) Close() error {
	err := b.Media.Close()
	if cerr := b.ari.Close(); err == nil {
		err = cerr
	}
	return err
}

// DefaultMaxDrain is the drain ceiling (maxDrainSec, nota 07) used when the
// wiring does not override it.
const DefaultMaxDrain = 15 * time.Minute
