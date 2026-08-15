package asterisk

import (
	"sync"
)

// FakeARI is a complete in-memory ARI for tests (the package's own thin
// interface, mirroring telnyx.FakeCallControl): it records every command,
// serves a configurable live-channel list, and can be told to fail.
type FakeARI struct {
	mu sync.Mutex

	// Err, when set, is returned by every command.
	Err error
	// Live is what LiveChannels returns.
	Live []string

	answered      []string
	hungup        []string
	continues     []FakeContinue
	plays         []FakePlay
	stops         []string
	dtmf          []FakeDTMF
	recordings    []string
	externalMedia []FakeExternalMedia
	closed        bool
}

// FakeContinue records one ContinueTo command.
type FakeContinue struct {
	ChannelID string
	Context   string
	Extension string
	Priority  int
}

// FakePlay records one Play command.
type FakePlay struct {
	ChannelID  string
	PlaybackID string
	MediaURI   string
}

// FakeDTMF records one SendDTMF command.
type FakeDTMF struct {
	ChannelID string
	Digits    string
}

// FakeExternalMedia records one ExternalMedia command.
type FakeExternalMedia struct {
	ChannelID    string
	ExternalHost string
	Format       string
}

var _ ARI = (*FakeARI)(nil)

// Answer implements ARI.
func (f *FakeARI) Answer(channelID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return f.Err
	}
	f.answered = append(f.answered, channelID)
	return nil
}

// Hangup implements ARI.
func (f *FakeARI) Hangup(channelID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return f.Err
	}
	f.hungup = append(f.hungup, channelID)
	return nil
}

// ContinueTo implements ARI.
func (f *FakeARI) ContinueTo(channelID, dialplanContext, extension string, priority int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return f.Err
	}
	f.continues = append(f.continues, FakeContinue{
		ChannelID: channelID,
		Context:   dialplanContext,
		Extension: extension,
		Priority:  priority,
	})
	return nil
}

// Play implements ARI.
func (f *FakeARI) Play(channelID, playbackID, mediaURI string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return f.Err
	}
	f.plays = append(f.plays, FakePlay{ChannelID: channelID, PlaybackID: playbackID, MediaURI: mediaURI})
	return nil
}

// StopPlayback implements ARI.
func (f *FakeARI) StopPlayback(playbackID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return f.Err
	}
	f.stops = append(f.stops, playbackID)
	return nil
}

// SendDTMF implements ARI.
func (f *FakeARI) SendDTMF(channelID, digits string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return f.Err
	}
	f.dtmf = append(f.dtmf, FakeDTMF{ChannelID: channelID, Digits: digits})
	return nil
}

// Record implements ARI.
func (f *FakeARI) Record(channelID, name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return f.Err
	}
	f.recordings = append(f.recordings, channelID)
	return nil
}

// ExternalMedia implements ARI.
func (f *FakeARI) ExternalMedia(channelID, externalHost, format string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return f.Err
	}
	f.externalMedia = append(f.externalMedia, FakeExternalMedia{
		ChannelID:    channelID,
		ExternalHost: externalHost,
		Format:       format,
	})
	return nil
}

// LiveChannels implements ARI.
func (f *FakeARI) LiveChannels() ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Err != nil {
		return nil, f.Err
	}
	return append([]string(nil), f.Live...), nil
}

// Close implements ARI.
func (f *FakeARI) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed = true
	return nil
}

// Answered returns the answered channel ids.
func (f *FakeARI) Answered() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.answered...)
}

// Hungup returns the hung-up channel ids.
func (f *FakeARI) Hungup() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.hungup...)
}

// Continues returns the recorded ContinueTo commands.
func (f *FakeARI) Continues() []FakeContinue {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]FakeContinue(nil), f.continues...)
}

// Plays returns the recorded Play commands.
func (f *FakeARI) Plays() []FakePlay {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]FakePlay(nil), f.plays...)
}

// Stops returns the playback ids stopped so far.
func (f *FakeARI) Stops() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.stops...)
}

// Recordings returns the channel ids being recorded.
func (f *FakeARI) Recordings() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.recordings...)
}

// ExternalMediaCalls returns the recorded ExternalMedia commands.
func (f *FakeARI) ExternalMediaCalls() []FakeExternalMedia {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]FakeExternalMedia(nil), f.externalMedia...)
}
