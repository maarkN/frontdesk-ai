package asterisk

import (
	"context"
	"fmt"

	"github.com/CyCoreSystems/ari/v6"
	"github.com/CyCoreSystems/ari/v6/client/native"
)

// Client is the production ARI implementation, a thin adapter over
// github.com/CyCoreSystems/ari/v6's native client (REST + event websocket).
type Client struct {
	inner ari.Client
	app   string
}

var _ ARI = (*Client)(nil)

// Dial connects to Asterisk ARI (native client: HTTP for commands, the
// /ari/events websocket for events). The returned client is bound to the
// Stasis application of cfg.
func Dial(ctx context.Context, cfg Config) (*Client, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	inner, err := native.ConnectWithContext(ctx, &native.Options{
		Application:  cfg.Application,
		URL:          cfg.URL,
		WebsocketURL: cfg.WebsocketURL,
		Username:     cfg.Username,
		Password:     cfg.Password,
	})
	if err != nil {
		return nil, fmt.Errorf("asterisk: connect %s app %s: %w", cfg.URL, cfg.Application, err)
	}
	return &Client{inner: inner, app: cfg.Application}, nil
}

// NewClientWith wraps an already-connected ari.Client (used by wiring that
// manages the connection itself).
func NewClientWith(inner ari.Client) *Client {
	return &Client{inner: inner, app: inner.ApplicationName()}
}

// channelKey builds the resource key of one channel.
func channelKey(id string) *ari.Key { return ari.NewKey(ari.ChannelKey, id) }

// Answer implements ARI.
func (c *Client) Answer(channelID string) error {
	if err := c.inner.Channel().Answer(channelKey(channelID)); err != nil {
		return fmt.Errorf("asterisk: answer %s: %w", channelID, err)
	}
	return nil
}

// Hangup implements ARI.
func (c *Client) Hangup(channelID string) error {
	if err := c.inner.Channel().Hangup(channelKey(channelID), "normal"); err != nil {
		return fmt.Errorf("asterisk: hangup %s: %w", channelID, err)
	}
	return nil
}

// ContinueTo implements ARI.
func (c *Client) ContinueTo(channelID, dialplanContext, extension string, priority int) error {
	err := c.inner.Channel().Continue(channelKey(channelID), dialplanContext, extension, priority)
	if err != nil {
		return fmt.Errorf("asterisk: continue %s to %s,%s: %w", channelID, dialplanContext, extension, err)
	}
	return nil
}

// Play implements ARI.
func (c *Client) Play(channelID, playbackID, mediaURI string) error {
	if _, err := c.inner.Channel().Play(channelKey(channelID), playbackID, mediaURI); err != nil {
		return fmt.Errorf("asterisk: play %s on %s: %w", mediaURI, channelID, err)
	}
	return nil
}

// StopPlayback implements ARI.
func (c *Client) StopPlayback(playbackID string) error {
	if err := c.inner.Playback().Stop(ari.NewKey(ari.PlaybackKey, playbackID)); err != nil {
		return fmt.Errorf("asterisk: stop playback %s: %w", playbackID, err)
	}
	return nil
}

// SendDTMF implements ARI.
func (c *Client) SendDTMF(channelID, digits string) error {
	if err := c.inner.Channel().SendDTMF(channelKey(channelID), digits, nil); err != nil {
		return fmt.Errorf("asterisk: send dtmf on %s: %w", channelID, err)
	}
	return nil
}

// Record implements ARI. Recording only starts after consent.captured — that
// gate lives in the caller (ADR: consent first), not here.
func (c *Client) Record(channelID, name string) error {
	_, err := c.inner.Channel().Record(channelKey(channelID), name, &ari.RecordingOptions{
		Format: "wav",
		Exists: "overwrite",
	})
	if err != nil {
		return fmt.Errorf("asterisk: record %s: %w", channelID, err)
	}
	return nil
}

// ExternalMedia implements ARI: creates the RTP leg of the call towards the
// gateway's ExternalMediaServer. Format slin16 (16kHz PCM) avoids transcode
// inside Asterisk; the 16k↔8k conversion happens in the media server.
func (c *Client) ExternalMedia(channelID, externalHost, format string) error {
	_, err := c.inner.Channel().ExternalMedia(nil, ari.ExternalMediaOptions{
		ChannelID:     channelID,
		App:           c.app,
		ExternalHost:  externalHost,
		Format:        format,
		Encapsulation: "rtp",
		Transport:     "udp",
	})
	if err != nil {
		return fmt.Errorf("asterisk: external media %s -> %s: %w", channelID, externalHost, err)
	}
	return nil
}

// LiveChannels implements ARI: the ids the Stasis app currently owns,
// straight from Asterisk (the source of truth on boot, nota 07).
func (c *Client) LiveChannels() ([]string, error) {
	data, err := c.inner.Application().Data(ari.NewKey(ari.ApplicationKey, c.app))
	if err != nil {
		return nil, fmt.Errorf("asterisk: list channels of app %s: %w", c.app, err)
	}
	return data.ChannelIDs, nil
}

// Close implements ARI.
func (c *Client) Close() error {
	c.inner.Close()
	return nil
}
