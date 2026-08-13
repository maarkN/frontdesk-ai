package telnyx

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// DefaultBaseURL is the production Telnyx API root.
const DefaultBaseURL = "https://api.telnyx.com/v2"

// Client is the real CallControl implementation over the Telnyx Call Control
// v2 HTTP API. Media streaming is NOT here: the WS leg has its own transport,
// injected where the call is wired (this keeps the HTTP client free of
// connection state and trivially testable against httptest).
type Client struct {
	httpc   *http.Client
	baseURL string
	apiKey  string
}

var _ CallControl = (*Client)(nil)

// ClientOption configures a Client.
type ClientOption func(*Client)

// WithBaseURL overrides the API root (tests point it at httptest).
func WithBaseURL(u string) ClientOption {
	return func(c *Client) { c.baseURL = u }
}

// WithHTTPClient injects the HTTP client (timeouts, instrumentation).
func WithHTTPClient(h *http.Client) ClientOption {
	return func(c *Client) { c.httpc = h }
}

// NewClient returns a CallControl talking to the Telnyx API. The default
// HTTP timeout is intentionally short: on the call path, a slow command is a
// failed command (the silence budget, ADR-006).
func NewClient(apiKey string, opts ...ClientOption) *Client {
	c := &Client{
		httpc:   &http.Client{Timeout: 2 * time.Second},
		baseURL: DefaultBaseURL,
		apiKey:  apiKey,
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// action posts a call-control action with the given body.
func (c *Client) action(ctx context.Context, callControlID, name string, body any) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("telnyx: marshal %s: %w", name, err)
	}
	url := fmt.Sprintf("%s/calls/%s/actions/%s", c.baseURL, callControlID, name)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("telnyx: build %s request: %w", name, err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpc.Do(req)
	if err != nil {
		return fmt.Errorf("telnyx: %s %s: %w", name, callControlID, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("telnyx: %s %s: status %d: %s", name, callControlID, resp.StatusCode, msg)
	}
	return nil
}

// Answer implements CallControl.
func (c *Client) Answer(ctx context.Context, callControlID string) error {
	return c.action(ctx, callControlID, "answer", struct{}{})
}

// Hangup implements CallControl.
func (c *Client) Hangup(ctx context.Context, callControlID string) error {
	return c.action(ctx, callControlID, "hangup", struct{}{})
}

// Transfer implements CallControl.
func (c *Client) Transfer(ctx context.Context, callControlID, to string) error {
	return c.action(ctx, callControlID, "transfer", struct {
		To string `json:"to"`
	}{To: to})
}

// StartRecording implements CallControl.
func (c *Client) StartRecording(ctx context.Context, callControlID string) error {
	return c.action(ctx, callControlID, "record_start", struct {
		Format  string `json:"format"`
		Channel string `json:"channels"`
	}{Format: "mp3", Channel: "dual"})
}

// Playback implements CallControl.
func (c *Client) Playback(ctx context.Context, callControlID, audioURL string) error {
	return c.action(ctx, callControlID, "playback_start", struct {
		AudioURL string `json:"audio_url"`
	}{AudioURL: audioURL})
}

// FlushPlayback implements CallControl.
func (c *Client) FlushPlayback(ctx context.Context, callControlID string) error {
	return c.action(ctx, callControlID, "playback_stop", struct {
		Stop string `json:"stop"`
	}{Stop: "all"})
}

// SendDTMF implements CallControl.
func (c *Client) SendDTMF(ctx context.Context, callControlID, digits string) error {
	return c.action(ctx, callControlID, "send_dtmf", struct {
		Digits string `json:"digits"`
	}{Digits: digits})
}
