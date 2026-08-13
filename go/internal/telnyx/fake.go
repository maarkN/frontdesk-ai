package telnyx

import (
	"context"
	"fmt"
	"sync"

	"github.com/maarkn/frontdesk/internal/media"
)

// FakeCallControl is a complete in-memory CallControl for tests: it records
// every command and can be told to fail.
type FakeCallControl struct {
	mu sync.Mutex

	// Err, when set, is returned by every command.
	Err error

	answered   []string
	hungup     []string
	transfers  []FakeTransfer
	recordings []string
	playbacks  []FakePlayback
	flushes    []string
	dtmf       []FakeDTMFSend
}

// FakeTransfer records one Transfer command.
type FakeTransfer struct {
	CallControlID string
	To            string
}

// FakePlayback records one Playback command.
type FakePlayback struct {
	CallControlID string
	AudioURL      string
}

// FakeDTMFSend records one SendDTMF command.
type FakeDTMFSend struct {
	CallControlID string
	Digits        string
}

var _ CallControl = (*FakeCallControl)(nil)

// Answer implements CallControl.
func (f *FakeCallControl) Answer(_ context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return f.Err
	}
	f.answered = append(f.answered, id)
	return nil
}

// Hangup implements CallControl.
func (f *FakeCallControl) Hangup(_ context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return f.Err
	}
	f.hungup = append(f.hungup, id)
	return nil
}

// Transfer implements CallControl.
func (f *FakeCallControl) Transfer(_ context.Context, id, to string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return f.Err
	}
	f.transfers = append(f.transfers, FakeTransfer{CallControlID: id, To: to})
	return nil
}

// StartRecording implements CallControl.
func (f *FakeCallControl) StartRecording(_ context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return f.Err
	}
	f.recordings = append(f.recordings, id)
	return nil
}

// Playback implements CallControl.
func (f *FakeCallControl) Playback(_ context.Context, id, audioURL string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return f.Err
	}
	f.playbacks = append(f.playbacks, FakePlayback{CallControlID: id, AudioURL: audioURL})
	return nil
}

// FlushPlayback implements CallControl.
func (f *FakeCallControl) FlushPlayback(_ context.Context, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return f.Err
	}
	f.flushes = append(f.flushes, id)
	return nil
}

// SendDTMF implements CallControl.
func (f *FakeCallControl) SendDTMF(_ context.Context, id, digits string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return f.Err
	}
	f.dtmf = append(f.dtmf, FakeDTMFSend{CallControlID: id, Digits: digits})
	return nil
}

// Answered returns the answered call ids.
func (f *FakeCallControl) Answered() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.answered...)
}

// Hungup returns the hung-up call ids.
func (f *FakeCallControl) Hungup() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.hungup...)
}

// Transfers returns the recorded transfers.
func (f *FakeCallControl) Transfers() []FakeTransfer {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]FakeTransfer(nil), f.transfers...)
}

// Flushes returns the FlushPlayback calls.
func (f *FakeCallControl) Flushes() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.flushes...)
}

// FakeWrite records one outbound media write.
type FakeWrite struct {
	PCM        []byte
	DurationMs int
}

// FakeMediaStream is an in-memory MediaStream: the test pushes inbound
// frames with PushFrame and inspects outbound writes/flushes.
type FakeMediaStream struct {
	mu      sync.Mutex
	in      chan media.Frame
	writes  []FakeWrite
	flushes int
	closed  bool
}

var _ MediaStream = (*FakeMediaStream)(nil)

// NewFakeMediaStream returns a stream whose inbound channel holds buf frames.
func NewFakeMediaStream(buf int) *FakeMediaStream {
	return &FakeMediaStream{in: make(chan media.Frame, buf)}
}

// PushFrame feeds one inbound frame to the consumer.
func (f *FakeMediaStream) PushFrame(fr media.Frame) { f.in <- fr }

// EndInbound closes the inbound side (caller hangup).
func (f *FakeMediaStream) EndInbound() { close(f.in) }

// Frames implements MediaStream.
func (f *FakeMediaStream) Frames() <-chan media.Frame { return f.in }

// Write implements MediaStream.
func (f *FakeMediaStream) Write(_ context.Context, pcm []byte, durationMs int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.closed {
		return fmt.Errorf("telnyx: write on closed fake stream")
	}
	f.writes = append(f.writes, FakeWrite{PCM: append([]byte(nil), pcm...), DurationMs: durationMs})
	return nil
}

// Flush implements MediaStream.
func (f *FakeMediaStream) Flush(context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.flushes++
	return nil
}

// Close implements MediaStream.
func (f *FakeMediaStream) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed = true
	return nil
}

// Writes returns all outbound writes so far.
func (f *FakeMediaStream) Writes() []FakeWrite {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]FakeWrite(nil), f.writes...)
}

// FlushCount returns how many times Flush was called.
func (f *FakeMediaStream) FlushCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.flushes
}
