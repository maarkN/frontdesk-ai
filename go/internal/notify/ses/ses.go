// Package ses is the Amazon SES v2 e-mail provider of the notifier
// (EPIC-005), pinned to ca-central-1 by default for data residency
// (CONTEXT.md). It is a sketch: the SendEmail request shape is real, but the
// Authorization header is a placeholder — production wiring must add SigV4
// signing (or route through a signing proxy) before go-live.
package ses

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

// DefaultRegion keeps tenant data in Canada (residency requirement).
const DefaultRegion = "ca-central-1"

// Client sends e-mail through the SES v2 API. Build it with New.
type Client struct {
	region    string
	from      string
	accessKey string
	secretKey string
	baseURL   string
	httpc     *http.Client
}

var _ notify.EmailSender = (*Client)(nil)

// Option configures a Client.
type Option func(*Client)

// WithBaseURL overrides the API endpoint (tests, mocks).
func WithBaseURL(base string) Option {
	return func(c *Client) { c.baseURL = base }
}

// WithHTTPClient overrides the HTTP client.
func WithHTTPClient(httpc *http.Client) Option {
	return func(c *Client) { c.httpc = httpc }
}

// WithRegion overrides DefaultRegion. Anything outside Canada must be
// rejected upstream (ErrResidencyViolation belongs to the provider factory).
func WithRegion(region string) Option {
	return func(c *Client) { c.region = region }
}

// New returns a client sending from the given verified identity.
func New(from, accessKey, secretKey string, opts ...Option) *Client {
	c := &Client{
		region:    DefaultRegion,
		from:      from,
		accessKey: accessKey,
		secretKey: secretKey,
		httpc:     &http.Client{Timeout: 15 * time.Second},
	}
	for _, opt := range opts {
		opt(c)
	}
	if c.baseURL == "" {
		c.baseURL = fmt.Sprintf("https://email.%s.amazonaws.com", c.region)
	}
	return c
}

// sendEmailRequest is the SES v2 SendEmail body (simple content).
type sendEmailRequest struct {
	FromEmailAddress string      `json:"FromEmailAddress"`
	Destination      destination `json:"Destination"`
	Content          content     `json:"Content"`
}

type destination struct {
	ToAddresses []string `json:"ToAddresses"`
}

type content struct {
	Simple simpleContent `json:"Simple"`
}

type simpleContent struct {
	Subject data     `json:"Subject"`
	Body    bodyData `json:"Body"`
}

type data struct {
	Data string `json:"Data"`
}

type bodyData struct {
	Text data `json:"Text"`
}

// Send implements notify.EmailSender.
func (c *Client) Send(ctx context.Context, msg notify.Email) error {
	body, err := json.Marshal(sendEmailRequest{
		FromEmailAddress: c.from,
		Destination:      destination{ToAddresses: []string{msg.To}},
		Content: content{Simple: simpleContent{
			Subject: data{Data: msg.Subject},
			Body:    bodyData{Text: data{Data: msg.Body}},
		}},
	})
	if err != nil {
		return fmt.Errorf("ses: marshal request: %w", err)
	}
	req, err := http.NewRequestWithContext(
		ctx, http.MethodPost, c.baseURL+"/v2/email/outbound-emails", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("ses: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	// TODO(EPIC-005 hardening): replace with real AWS SigV4 signing. The
	// placeholder keeps the request shape testable without the AWS SDK.
	req.Header.Set("Authorization", "AWS4-HMAC-SHA256 Credential="+c.accessKey)

	resp, err := c.httpc.Do(req)
	if err != nil {
		return fmt.Errorf("ses: send e-mail: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		detail, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("ses: send e-mail: status %d: %s", resp.StatusCode, detail)
	}
	return nil
}
