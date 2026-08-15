package asterisk_test

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/maarkn/frontdesk/internal/asterisk"
	"github.com/maarkn/frontdesk/internal/audiobank"
	"github.com/maarkn/frontdesk/internal/domain"
	"github.com/maarkn/frontdesk/internal/event"
	"github.com/maarkn/frontdesk/internal/media"
)

// ---------------------------------------------------------------------------
// Test doubles for the rehydration wiring (consumer-side interfaces only —
// no store import, per ADR-009).
// ---------------------------------------------------------------------------

// fakeCalls serves snapshots for a fixed set of call ids.
type fakeCalls struct {
	calls map[string]domain.Call
}

func (f *fakeCalls) GetCall(_ context.Context, callID string) (domain.Call, error) {
	c, ok := f.calls[callID]
	if !ok {
		return domain.Call{}, fmt.Errorf("fake store: call %s not found", callID)
	}
	return c, nil
}

// fakePlayer records the clips played per channel.
type fakePlayer struct {
	mu     sync.Mutex
	played map[string]audiobank.Clip
	err    error
}

func (f *fakePlayer) PlayClip(_ context.Context, channelID string, clip audiobank.Clip) error {
	if f.err != nil {
		return f.err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.played == nil {
		f.played = make(map[string]audiobank.Clip)
	}
	f.played[channelID] = clip
	return nil
}

// newBank builds an audio bank covering both locales.
func newBank(t *testing.T) *audiobank.Bank {
	t.Helper()
	bank := audiobank.New("test-persona", "v1")
	for _, loc := range []event.Locale{event.LocaleENCA, event.LocaleFRCA} {
		for _, key := range audiobank.Required() {
			bank.Add(loc, key, audiobank.Clip{
				Text:       string(key) + "/" + string(loc),
				Audio:      []byte{1, 2, 3},
				DurationMs: 500,
			})
		}
	}
	return bank
}

const ownerCell = "+15145550100"

// newRehydrator wires a Rehydrator over the fakes.
func newRehydrator(t *testing.T, ari *asterisk.FakeARI, calls *fakeCalls, player *fakePlayer, hooks asterisk.RehydrateHooks) *asterisk.Rehydrator {
	t.Helper()
	r, err := asterisk.NewRehydrator(asterisk.RehydratorConfig{
		ARI:       ari,
		Calls:     calls,
		Clips:     newBank(t),
		Player:    player,
		Control:   asterisk.NewCallControl(ari, "transfer-owner"),
		OwnerCell: ownerCell,
		Hooks:     hooks,
	})
	require.NoError(t, err)
	return r
}

// snapshotCall builds a live-call snapshot in the given locale.
func snapshotCall(id string, loc event.Locale, status event.CallStatus) domain.Call {
	return domain.Call{Snapshot: event.CallSnapshot{
		CallID: id,
		Status: status,
		Locale: loc,
	}}
}

// ---------------------------------------------------------------------------
// Rehydration (nota 07): with snapshot → re-anchor; without → recovered_orphan.
// ---------------------------------------------------------------------------

func TestRehydrateWithSnapshotResumesWithReanchorPhrase(t *testing.T) {
	ari := &asterisk.FakeARI{Live: []string{"ch-1"}}
	calls := &fakeCalls{calls: map[string]domain.Call{
		"ch-1": snapshotCall("ch-1", event.LocaleFRCA, event.StatusAnswered),
	}}
	player := &fakePlayer{}

	var resumed []string
	r := newRehydrator(t, ari, calls, player, asterisk.RehydrateHooks{
		OnResumed: func(chID string, _ event.CallSnapshot) { resumed = append(resumed, chID) },
	})

	report, err := r.Rehydrate(context.Background())
	require.NoError(t, err)
	require.Equal(t, []string{"ch-1"}, report.Resumed)
	require.Empty(t, report.Orphaned)
	require.Equal(t, []string{"ch-1"}, resumed)

	// The re-anchoring phrase came from the audio bank, in the CALL's locale.
	require.Equal(t, "say_again/fr-CA", player.played["ch-1"].Text)
	// Never improvised into a transfer or hangup.
	require.Empty(t, ari.Continues())
	require.Empty(t, ari.Hungup())
}

func TestRehydrateWithoutSnapshotTransfersRecoveredOrphan(t *testing.T) {
	ari := &asterisk.FakeARI{Live: []string{"ch-orphan"}}
	player := &fakePlayer{}

	var orphaned []string
	r := newRehydrator(t, ari, &fakeCalls{}, player, asterisk.RehydrateHooks{
		OnOrphan: func(chID string) { orphaned = append(orphaned, chID) },
	})

	report, err := r.Rehydrate(context.Background())
	require.NoError(t, err)
	require.Equal(t, []string{"ch-orphan"}, report.Orphaned)
	require.Empty(t, report.Resumed)
	require.Equal(t, []string{"ch-orphan"}, orphaned)

	// The orphan went to the owner's cell through the dialplan transfer
	// context — never left in silence, never improvised.
	conts := ari.Continues()
	require.Len(t, conts, 1)
	require.Equal(t, "ch-orphan", conts[0].ChannelID)
	require.Equal(t, "transfer-owner", conts[0].Context)
	require.Equal(t, ownerCell, conts[0].Extension)
	require.Empty(t, player.played)
}

func TestRehydrateMixedChannels(t *testing.T) {
	ari := &asterisk.FakeARI{Live: []string{"ch-live", "ch-orphan", "ch-done"}}
	calls := &fakeCalls{calls: map[string]domain.Call{
		"ch-live": snapshotCall("ch-live", event.LocaleENCA, event.StatusAnswered),
		"ch-done": snapshotCall("ch-done", event.LocaleENCA, event.StatusEnded),
	}}
	player := &fakePlayer{}
	r := newRehydrator(t, ari, calls, player, asterisk.RehydrateHooks{})

	report, err := r.Rehydrate(context.Background())
	require.NoError(t, err)
	require.Equal(t, []string{"ch-live"}, report.Resumed)
	require.Equal(t, []string{"ch-orphan"}, report.Orphaned)
	require.Equal(t, []string{"ch-done"}, report.Stale)
	// The stale channel was hung up, not resumed.
	require.Equal(t, []string{"ch-done"}, ari.Hungup())
}

func TestRehydratePlayFailureDegradesToOrphan(t *testing.T) {
	// Resume path breaking mid-way must still end at a human (nota 07:
	// never improvise), not in silence.
	ari := &asterisk.FakeARI{Live: []string{"ch-1"}}
	calls := &fakeCalls{calls: map[string]domain.Call{
		"ch-1": snapshotCall("ch-1", event.LocaleENCA, event.StatusAnswered),
	}}
	player := &fakePlayer{err: errors.New("media leg gone")}
	r := newRehydrator(t, ari, calls, player, asterisk.RehydrateHooks{})

	report, err := r.Rehydrate(context.Background())
	require.NoError(t, err)
	require.Equal(t, []string{"ch-1"}, report.Orphaned)
	require.Len(t, ari.Continues(), 1)
}

// ---------------------------------------------------------------------------
// FlushPlayback = explicit StopPlayback (nota 06) — the barge-in hard cut.
// ---------------------------------------------------------------------------

func TestFlushPlaybackStopsEveryActivePlayback(t *testing.T) {
	ari := &asterisk.FakeARI{}
	cc := asterisk.NewCallControl(ari, "")
	ctx := context.Background()

	require.NoError(t, cc.Playback(ctx, "ch-1", "http://audio/greeting.wav"))
	require.NoError(t, cc.Playback(ctx, "ch-1", "http://audio/menu.wav"))
	require.NoError(t, cc.Playback(ctx, "ch-2", "http://audio/other.wav"))
	require.Len(t, cc.ActivePlaybacks("ch-1"), 2)

	// Barge-in on ch-1: both of its playbacks are stopped EXPLICITLY, the
	// other call is untouched.
	require.NoError(t, cc.FlushPlayback(ctx, "ch-1"))

	plays := ari.Plays()
	require.Len(t, plays, 3)
	stopped := ari.Stops()
	require.ElementsMatch(t, []string{plays[0].PlaybackID, plays[1].PlaybackID}, stopped)
	require.Empty(t, cc.ActivePlaybacks("ch-1"))
	require.Len(t, cc.ActivePlaybacks("ch-2"), 1)

	// A second flush is a no-op: nothing tracked, nothing stopped twice.
	require.NoError(t, cc.FlushPlayback(ctx, "ch-1"))
	require.Len(t, ari.Stops(), 2)
}

func TestPlaybackFinishedReleasesTracking(t *testing.T) {
	ari := &asterisk.FakeARI{}
	cc := asterisk.NewCallControl(ari, "")
	ctx := context.Background()

	require.NoError(t, cc.Playback(ctx, "ch-1", "http://audio/a.wav"))
	pbID := ari.Plays()[0].PlaybackID

	cc.PlaybackFinished("ch-1", pbID)
	require.Empty(t, cc.ActivePlaybacks("ch-1"))

	// Flush after natural end stops nothing (the playback is gone).
	require.NoError(t, cc.FlushPlayback(ctx, "ch-1"))
	require.Empty(t, ari.Stops())
}

func TestTransferLeavesStasisViaDialplan(t *testing.T) {
	ari := &asterisk.FakeARI{}
	cc := asterisk.NewCallControl(ari, "transfer-owner")

	require.NoError(t, cc.Transfer(context.Background(), "ch-1", "+14385550199"))
	conts := ari.Continues()
	require.Len(t, conts, 1)
	require.Equal(t, asterisk.FakeContinue{
		ChannelID: "ch-1",
		Context:   "transfer-owner",
		Extension: "+14385550199",
		Priority:  1,
	}, conts[0])
}

// ---------------------------------------------------------------------------
// Blue/green drain (nota 07): not-ready while alive, released by the last
// call, capped by maxDrainSec.
// ---------------------------------------------------------------------------

func TestDrainWaitsForActiveCalls(t *testing.T) {
	d := asterisk.NewDrainer()
	d.CallStarted("ch-1")
	d.CallStarted("ch-2")
	require.True(t, d.Ready())
	require.Equal(t, 2, d.Active())

	done := make(chan error, 1)
	go func() { done <- d.Drain(context.Background(), time.Minute) }()

	// Draining flips readiness while the process stays alive.
	require.Eventually(t, func() bool { return !d.Ready() }, time.Second, time.Millisecond)
	select {
	case err := <-done:
		t.Fatalf("drain returned with active calls: %v", err)
	case <-time.After(20 * time.Millisecond):
	}

	d.CallEnded("ch-1")
	require.Equal(t, 1, d.Active())
	d.CallEnded("ch-2")

	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("drain did not release after the last call ended")
	}
	require.Equal(t, 0, d.Active())
}

func TestDrainNoActiveCallsReturnsImmediately(t *testing.T) {
	d := asterisk.NewDrainer()
	require.NoError(t, d.Drain(context.Background(), time.Minute))
	require.False(t, d.Ready())
}

func TestDrainCeiling(t *testing.T) {
	d := asterisk.NewDrainer()
	d.CallStarted("ch-stuck")

	err := d.Drain(context.Background(), 10*time.Millisecond)
	require.ErrorIs(t, err, asterisk.ErrDrainTimeout)
	require.Equal(t, 1, d.Active())
}

// ---------------------------------------------------------------------------
// External Media: RTP slin16 in → 20ms 8kHz frames out, and back.
// ---------------------------------------------------------------------------

// buildWirePacket builds one inbound RTP datagram: 320 big-endian slin16
// samples of the given constant value.
func buildWirePacket(seq uint16, ssrc uint32, sample int16) []byte {
	const payloadBytes = 640
	pkt := make([]byte, 12+payloadBytes)
	pkt[0] = 2 << 6
	binary.BigEndian.PutUint16(pkt[2:4], seq)
	binary.BigEndian.PutUint32(pkt[4:8], uint32(seq)*320)
	binary.BigEndian.PutUint32(pkt[8:12], ssrc)
	for i := 0; i < payloadBytes/2; i++ {
		binary.BigEndian.PutUint16(pkt[12+i*2:], uint16(sample))
	}
	return pkt
}

// startServer runs an ExternalMediaServer on loopback plus a UDP peer
// standing in for Asterisk.
func startServer(t *testing.T) (*asterisk.ExternalMediaServer, *net.UDPConn) {
	t.Helper()
	srv, err := asterisk.NewExternalMediaServer("127.0.0.1:0")
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	served := make(chan struct{})
	go func() {
		defer close(served)
		_ = srv.Serve(ctx)
	}()
	t.Cleanup(func() {
		cancel()
		<-served // Serve owns the sessions; wait for it to wind down
	})

	peer, err := net.DialUDP("udp", nil, srv.Addr().(*net.UDPAddr))
	require.NoError(t, err)
	t.Cleanup(func() { _ = peer.Close() })
	return srv, peer
}

func TestExternalMediaInboundBecomes20msFrames(t *testing.T) {
	srv, peer := startServer(t)

	const ssrc = 0x42
	for seq := uint16(0); seq < 3; seq++ {
		_, err := peer.Write(buildWirePacket(1000+seq, ssrc, 1000))
		require.NoError(t, err)
	}

	var sess *asterisk.MediaSession
	select {
	case sess = <-srv.Sessions():
	case <-time.After(2 * time.Second):
		t.Fatal("no media session announced")
	}
	require.Equal(t, uint32(ssrc), sess.SSRC())

	var frames []media.Frame
	for len(frames) < 3 {
		select {
		case f := <-sess.Frames():
			frames = append(frames, f)
		case <-time.After(2 * time.Second):
			t.Fatalf("got %d frames, want 3", len(frames))
		}
	}
	// Each 320-sample 16kHz packet became one 160-sample 8kHz frame (the
	// internal/media contract), sequence preserved.
	for i, f := range frames {
		require.Len(t, f.Samples, media.SamplesPerFrame)
		require.Equal(t, uint64(i), f.Seq)
		require.Equal(t, int16(1000), f.Samples[0]) // constant signal survives decimation
	}
}

func TestExternalMediaOutboundIsPacedRTP(t *testing.T) {
	srv, peer := startServer(t)

	_, err := peer.Write(buildWirePacket(1, 7, 0))
	require.NoError(t, err)
	sess := <-srv.Sessions()

	// One 20ms frame of 8kHz PCM16 LE, constant value 2000.
	pcm := make([]byte, media.SamplesPerFrame*2)
	for i := 0; i < media.SamplesPerFrame; i++ {
		binary.LittleEndian.PutUint16(pcm[i*2:], uint16(int16(2000)))
	}
	require.NoError(t, sess.Write(context.Background(), pcm, media.FrameMs))

	require.NoError(t, peer.SetReadDeadline(time.Now().Add(2*time.Second)))
	buf := make([]byte, 2048)
	n, err := peer.Read(buf)
	require.NoError(t, err)

	// 12-byte RTP header + 640 bytes of big-endian slin16 (20ms at 16kHz).
	require.Equal(t, 12+640, n)
	require.Equal(t, byte(2<<6), buf[0])
	first := int16(binary.BigEndian.Uint16(buf[12:14]))
	require.Equal(t, int16(2000), first) // constant signal survives interpolation
}

func TestExternalMediaFlushDropsQueuedAudio(t *testing.T) {
	srv, peer := startServer(t)

	_, err := peer.Write(buildWirePacket(1, 9, 0))
	require.NoError(t, err)
	sess := <-srv.Sessions()

	// Queue 500ms of audio, then barge-in: Flush must empty the queue NOW.
	pcm := make([]byte, media.SamplesPerFrame*2*25)
	require.NoError(t, sess.Write(context.Background(), pcm, 25*media.FrameMs))
	require.Positive(t, sess.QueuedPackets())

	require.NoError(t, sess.Flush(context.Background()))
	require.Zero(t, sess.QueuedPackets())
}

func TestExternalMediaWriteOverflow(t *testing.T) {
	srv, peer := startServer(t)

	_, err := peer.Write(buildWirePacket(1, 11, 0))
	require.NoError(t, err)
	sess := <-srv.Sessions()

	// Write must never block: past the queue bound it reports overflow.
	pcm := make([]byte, media.SamplesPerFrame*2)
	var overflowed bool
	for i := 0; i < 400; i++ {
		if err := sess.Write(context.Background(), pcm, media.FrameMs); err != nil {
			overflowed = true
			break
		}
	}
	require.True(t, overflowed, "unbounded outbound queue")
}

// ---------------------------------------------------------------------------
// Backend bundle: the ready-to-wire constructor for the gateway main.
// ---------------------------------------------------------------------------

func TestNewBackendWith(t *testing.T) {
	ari := &asterisk.FakeARI{}
	b, err := asterisk.NewBackendWith(ari, asterisk.Config{
		URL:         "http://localhost:8088/ari",
		Application: "frontdesk-v1",
	}, "127.0.0.1:0")
	require.NoError(t, err)
	require.NotNil(t, b.Control)
	require.NotNil(t, b.Media)
	require.NotNil(t, b.Drainer)
	require.NoError(t, b.Close())
}
