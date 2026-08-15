package asterisk

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"math/rand/v2"
	"net"
	"sync"
	"time"

	"github.com/maarkn/frontdesk/internal/media"
	"github.com/maarkn/frontdesk/internal/telnyx"
)

// ExternalMediaServer is the gateway side of ARI External Media: one UDP
// socket receiving the RTP legs (slin16, 16kHz, 20ms) that Asterisk creates
// per call. Each distinct SSRC becomes a MediaSession — the Asterisk
// implementation of the gateway's media contract (internal/telnyx
// MediaStream): inbound RTP is downsampled into the same 20ms 8kHz frames
// the Telnyx stream produces, outbound PCM is upsampled and paced back as
// RTP to the leg's source address.
//
// Correlating a session with its call is the wiring's job: the gateway
// creates the external media leg for a known channel and matches the next
// accepted session (lab traffic is serialized per call setup).
type ExternalMediaServer struct {
	conn *net.UDPConn

	accept chan *MediaSession

	mu       sync.Mutex
	sessions map[uint32]*MediaSession
	closed   bool
}

// NewExternalMediaServer binds the UDP socket (e.g. ":4000"). Serve must be
// called to start reading.
func NewExternalMediaServer(addr string) (*ExternalMediaServer, error) {
	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return nil, fmt.Errorf("asterisk: resolve %s: %w", addr, err)
	}
	conn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		return nil, fmt.Errorf("asterisk: listen %s: %w", addr, err)
	}
	return &ExternalMediaServer{
		conn:     conn,
		accept:   make(chan *MediaSession, 16),
		sessions: make(map[uint32]*MediaSession),
	}, nil
}

// Addr returns the bound UDP address (the ExternalHost handed to ARI, port
// side).
func (s *ExternalMediaServer) Addr() net.Addr { return s.conn.LocalAddr() }

// Sessions delivers each new media leg once. The channel closes when the
// server stops.
func (s *ExternalMediaServer) Sessions() <-chan *MediaSession { return s.accept }

// Serve reads the socket until ctx is done or the server is closed. It owns
// the read loop and the per-session pacing goroutines (uber-go: goroutines
// have an owner and a lifecycle).
func (s *ExternalMediaServer) Serve(ctx context.Context) error {
	stop := context.AfterFunc(ctx, func() { _ = s.Close() })
	defer stop()

	buf := make([]byte, 2048)
	for {
		n, remote, err := s.conn.ReadFromUDP(buf)
		if err != nil {
			s.shutdown()
			if ctx.Err() != nil || errors.Is(err, net.ErrClosed) {
				return nil
			}
			return fmt.Errorf("asterisk: external media read: %w", err)
		}
		pkt, err := parseRTP(buf[:n])
		if err != nil || len(pkt.Payload) == 0 {
			continue // not RTP we understand: drop, never crash the media path
		}
		s.session(ctx, pkt.SSRC, remote).deliver(pkt)
	}
}

// Close stops the server and every session.
func (s *ExternalMediaServer) Close() error {
	err := s.conn.Close()
	s.shutdown()
	return err
}

// session returns the session of an SSRC, creating (and announcing) it on
// first sight.
func (s *ExternalMediaServer) session(ctx context.Context, ssrc uint32, remote *net.UDPAddr) *MediaSession {
	s.mu.Lock()
	defer s.mu.Unlock()
	if sess, ok := s.sessions[ssrc]; ok {
		return sess
	}
	sess := newMediaSession(s, ssrc, remote)
	s.sessions[ssrc] = sess
	go sess.sendLoop(ctx)
	select {
	case s.accept <- sess:
	default: // wiring not consuming: drop the announce, never block media
	}
	return sess
}

// shutdown closes every session exactly once.
func (s *ExternalMediaServer) shutdown() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	sessions := make([]*MediaSession, 0, len(s.sessions))
	for _, sess := range s.sessions {
		sessions = append(sessions, sess)
	}
	s.sessions = map[uint32]*MediaSession{}
	s.mu.Unlock()

	for _, sess := range sessions {
		_ = sess.Close()
	}
	close(s.accept)
}

// drop removes a session that closed individually.
func (s *ExternalMediaServer) drop(ssrc uint32) {
	s.mu.Lock()
	if !s.closed {
		delete(s.sessions, ssrc)
	}
	s.mu.Unlock()
}

// MediaSession is the media stream of one call over the Asterisk backend.
type MediaSession struct {
	server *ExternalMediaServer
	ssrc   uint32
	remote *net.UDPAddr

	frames chan media.Frame

	// Inbound state, touched only by the server read loop.
	started  bool
	lastSeq  uint16
	frameSeq uint64

	mu      sync.Mutex
	out     [][]byte // queued outbound 20ms payloads (sender-paced)
	closed  bool
	closeCh chan struct{}

	closeOnce sync.Once

	// Outbound RTP state, touched only by sendLoop.
	sendSeq uint16
	sendTS  uint32
	sendSSR uint32
}

var _ telnyx.MediaStream = (*MediaSession)(nil)

// outQueueMax bounds the outbound queue: 5s of audio. Beyond that the
// writer is misbehaving and gets an overflow error (Write must never block).
const outQueueMax = 250

func newMediaSession(server *ExternalMediaServer, ssrc uint32, remote *net.UDPAddr) *MediaSession {
	return &MediaSession{
		server:  server,
		ssrc:    ssrc,
		remote:  remote,
		frames:  make(chan media.Frame, 64),
		closeCh: make(chan struct{}),
		sendSSR: rand.Uint32(),
	}
}

// SSRC identifies the RTP source of this session.
func (m *MediaSession) SSRC() uint32 { return m.ssrc }

// Remote is the Asterisk-side address of the leg (outbound RTP destination).
func (m *MediaSession) Remote() *net.UDPAddr { return m.remote }

// Frames implements telnyx.MediaStream: inbound 20ms 8kHz frames. The
// channel closes when the session ends.
func (m *MediaSession) Frames() <-chan media.Frame { return m.frames }

// Write implements telnyx.MediaStream: queues outbound PCM (8kHz mono
// PCM16, little-endian bytes — the gateway's internal format) for paced
// playout. It never blocks: a full queue reports overflow.
func (m *MediaSession) Write(_ context.Context, pcm []byte, _ int) error {
	samples := make([]int16, len(pcm)/2)
	for i := range samples {
		samples[i] = int16(binary.LittleEndian.Uint16(pcm[i*2:]))
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return fmt.Errorf("asterisk: write on closed media session %d", m.ssrc)
	}
	for start := 0; start < len(samples); start += media.SamplesPerFrame {
		end := min(start+media.SamplesPerFrame, len(samples))
		chunk := samples[start:end]
		if len(chunk) < media.SamplesPerFrame { // pad the tail to a full frame
			chunk = append(append([]int16(nil), chunk...), make([]int16, media.SamplesPerFrame-len(chunk))...)
		}
		if len(m.out) >= outQueueMax {
			return fmt.Errorf("asterisk: outbound overflow on session %d (%d packets queued)", m.ssrc, len(m.out))
		}
		m.out = append(m.out, encodePayload16k(upsample8to16(chunk)))
	}
	return nil
}

// Flush implements telnyx.MediaStream: the barge-in cut on the media path.
// Everything queued and not yet on the wire is dropped immediately.
func (m *MediaSession) Flush(context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.out = m.out[:0]
	return nil
}

// QueuedPackets reports the outbound queue depth (tests, backpressure
// metrics).
func (m *MediaSession) QueuedPackets() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.out)
}

// Close implements telnyx.MediaStream.
func (m *MediaSession) Close() error {
	m.closeOnce.Do(func() {
		m.mu.Lock()
		m.closed = true
		m.out = nil
		m.mu.Unlock()
		close(m.closeCh)
		close(m.frames)
		m.server.drop(m.ssrc)
	})
	return nil
}

// deliver converts one inbound RTP packet into a media.Frame. Called only
// from the server read loop.
func (m *MediaSession) deliver(pkt rtpPacket) {
	if len(pkt.Payload) != wirePayloadBytes {
		return // partial/foreign payload: drop
	}
	if !m.started {
		m.started = true
	} else {
		// uint16 arithmetic makes the delta wrap-safe.
		delta := pkt.Seq - m.lastSeq
		if delta == 0 || delta > 0x8000 {
			return // duplicate or late reorder: the jitter buffer downstream owns repair
		}
		m.frameSeq += uint64(delta)
	}
	m.lastSeq = pkt.Seq

	frame := media.Frame{Seq: m.frameSeq, Samples: downsample16to8(decodePayload16k(pkt.Payload))}

	m.mu.Lock()
	closed := m.closed
	m.mu.Unlock()
	if closed {
		return
	}
	select {
	case m.frames <- frame:
	default: // consumer stalled: dropping beats blocking the socket loop
	}
}

// sendLoop paces the outbound queue at one packet per 20ms. It owns the
// session's outbound RTP state and exits on session close or ctx done.
func (m *MediaSession) sendLoop(ctx context.Context) {
	ticker := time.NewTicker(media.FrameDuration)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-m.closeCh:
			return
		case <-ticker.C:
			if payload, ok := m.nextPayload(); ok {
				datagram := buildRTP(m.sendSeq, m.sendTS, m.sendSSR, payload)
				m.sendSeq++
				m.sendTS += wireSamplesPerFrame
				_, _ = m.server.conn.WriteToUDP(datagram, m.remote)
			}
		}
	}
}

// nextPayload pops the next queued payload, if any.
func (m *MediaSession) nextPayload() ([]byte, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.out) == 0 {
		return nil, false
	}
	payload := m.out[0]
	m.out = m.out[1:]
	return payload, true
}
