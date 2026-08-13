package notify

import (
	"context"
	"sync"
)

// FakeSMS is an in-memory SMSSender for tests and local runs. Configure Err
// before use to simulate carrier failure; it is not safe to mutate Err
// concurrently with Send.
type FakeSMS struct {
	// Err, when non-nil, is returned by every Send without recording.
	Err error

	mu   sync.Mutex
	sent []SMS
}

var _ SMSSender = (*FakeSMS)(nil)

// Send implements SMSSender.
func (f *FakeSMS) Send(_ context.Context, msg SMS) error {
	if f.Err != nil {
		return f.Err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, msg)
	return nil
}

// Sent returns a copy of every accepted message, in send order.
func (f *FakeSMS) Sent() []SMS {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]SMS, len(f.sent))
	copy(out, f.sent)
	return out
}

// FakeEmail is an in-memory EmailSender for tests and local runs. Same
// contract as FakeSMS.
type FakeEmail struct {
	// Err, when non-nil, is returned by every Send without recording.
	Err error

	mu   sync.Mutex
	sent []Email
}

var _ EmailSender = (*FakeEmail)(nil)

// Send implements EmailSender.
func (f *FakeEmail) Send(_ context.Context, msg Email) error {
	if f.Err != nil {
		return f.Err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, msg)
	return nil
}

// Sent returns a copy of every accepted message, in send order.
func (f *FakeEmail) Sent() []Email {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]Email, len(f.sent))
	copy(out, f.sent)
	return out
}
