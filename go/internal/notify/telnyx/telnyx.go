// Package telnyx is the Telnyx SMS provider of the notifier (EPIC-005). It
// posts to the v2 Messages API of the same carrier that terminates the calls
// (ADR-004). It is a sketch: request shape and error handling are real, but
// delivery-status webhooks and messaging-profile configuration land with the
// production wiring.
package telnyx

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/maarkn/frontdesk/internal/notify"
)

const defaultBaseURL = "https://api.telnyx.com"

// Client sends SMS through Telnyx. Build it with New.
type Client struct {
	apiKey  string
	from    string
	baseURL string
	httpc   *http.Client
}

var _ notify.SMSSender = (*Client)(nil)

// Option configures a Client.
type Option func(*Client)

// WithBaseURL overrides the API endpoint (tests, mocks).
func WithBaseURL(base string) Option {
	return func(c *Client) { c.baseURL = base }
}

// WithHTTPClient overrides the HTTP client (timeouts, instrumentation).
func WithHTTPClient(httpc *http.Client) Option {
	return func(c *Client) { c.httpc = httpc }
}

// New returns a client sending from the given E.164 number.
func New(apiKey, from string, opts ...Option) *Client {
	c := &Client{
		apiKey:  apiKey,
		from:    from,
		baseURL: defaultBaseURL,
		httpc:   &http.Client{Timeout: 10 * time.Second},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// message is the v2 Messages request body.
type message struct {
	From string `json:"from"`
	To   string `json:"to"`
	Text string `json:"text"`
}

// Send implements notify.SMSSender. A non-2xx carrier response is an error,
// so the caller's redelivery/dedup machinery decides the retry.
func (c *Client) Send(ctx context.Context, msg notify.SMS) error {
	body, err := json.Marshal(message{From: c.from, To: msg.To, Text: msg.Body})
	if err != nil {
		return fmt.Errorf("telnyx: marshal message: %w", err)
	}
	req, err := http.NewRequestWithContext(
		ctx, http.MethodPost, c.baseURL+"/v2/messages", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("telnyx: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpc.Do(req)
	if err != nil {
		return fmt.Errorf("telnyx: send SMS: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		detail, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("telnyx: send SMS: status %d: %s", resp.StatusCode, detail)
	}
	return nil
}
