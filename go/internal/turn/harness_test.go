package turn_test

// This file is the synthetic-audio harness of nota 06: fakes for STT, agent,
// TTS and playback, plus a driver that runs one Machine per test with a
// deterministic media clock (frames ARE the clock — no wall-time flakiness
// on the endpointing/barge-in paths).

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"github.com/maarkn/frontdesk/internal/audiobank"
	"github.com/maarkn/frontdesk/internal/event"
	"github.com/maarkn/frontdesk/internal/media"
	"github.com/maarkn/frontdesk/internal/turn"
)

// TestMain enforces US-2.5: no test in this package may leak a goroutine —
// cascading cancellation must reap every producer.
func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

const testTimeout = 5 * time.Second

// recv waits for one event with a timeout.
func recv[T any](t *testing.T, ch <-chan T, what string) T {
	t.Helper()
	select {
	case v := <-ch:
		return v
	case <-time.After(testTimeout):
		t.Fatalf("timeout waiting for %s", what)
		panic("unreachable")
	}
}

// --- fakes ----------------------------------------------------------------

// fakeSTT counts frames (fed in ALL states) and lets the test inject
// results through an unbuffered channel, which doubles as a synchronization
// point: when the send returns, the machine has dequeued the result.
type fakeSTT struct {
	frames  atomic.Int64
	results chan turn.Result
}

func newFakeSTT() *fakeSTT {
	return &fakeSTT{results: make(chan turn.Result)}
}

func (s *fakeSTT) SendFrame(media.Frame) error { s.frames.Add(1); return nil }
func (s *fakeSTT) Results() <-chan turn.Result { return s.results }
func (s *fakeSTT) framesSeen() int64           { return s.frames.Load() }

// fakeAgent streams the configured reply word by word. never=true blocks
// until cancelled (LLM hang); delay simulates first-token latency.
type fakeAgent struct {
	reply string
	delay time.Duration
	never bool

	mu     sync.Mutex
	inputs []turn.TurnInput
}

func (a *fakeAgent) Reply(ctx context.Context, in turn.TurnInput) (<-chan string, error) {
	a.mu.Lock()
	a.inputs = append(a.inputs, in)
	a.mu.Unlock()

	out := make(chan string)
	go func() {
		defer close(out)
		if a.never {
			<-ctx.Done()
			return
		}
		if a.delay > 0 {
			select {
			case <-time.After(a.delay):
			case <-ctx.Done():
				return
			}
		}
		for _, w := range strings.Fields(a.reply) {
			select {
			case out <- w:
			case <-ctx.Done():
				return
			}
		}
	}()
	return out, nil
}

func (a *fakeAgent) lastInput() turn.TurnInput {
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.inputs) == 0 {
		return turn.TurnInput{}
	}
	return a.inputs[len(a.inputs)-1]
}

// fakeTTS collects the token stream and emits ONE chunk whose word timings
// give every word the same duration — the deterministic word-timing source
// the PlayoutTracker assertions are computed against.
type fakeTTS struct {
	wordMs int64
}

func (f *fakeTTS) Speak(ctx context.Context, text <-chan string) (<-chan turn.Chunk, error) {
	out := make(chan turn.Chunk, 1)
	go func() {
		defer close(out)
		var b strings.Builder
	collect:
		for {
			select {
			case <-ctx.Done():
				return
			case tok, ok := <-text:
				if !ok {
					break collect
				}
				b.WriteString(tok)
				b.WriteByte(' ')
			}
		}
		words := strings.Fields(b.String())
		if len(words) == 0 {
			return
		}
		timings := make([]turn.WordTiming, len(words))
		for i, w := range words {
			timings[i] = turn.WordTiming{
				Word:    w,
				StartMs: int64(i) * f.wordMs,
				EndMs:   int64(i+1) * f.wordMs,
			}
		}
		chunk := turn.Chunk{
			PCM:        make([]byte, len(words)),
			DurationMs: int64(len(words)) * f.wordMs,
			Words:      timings,
		}
		select {
		case out <- chunk:
		case <-ctx.Done():
		}
	}()
	return out, nil
}

// fakePlay records the outbound half of the line.
type fakePlay struct {
	mu      sync.Mutex
	writes  [][]byte
	ducks   []bool
	flushes int
}

func (p *fakePlay) Write(_ context.Context, pcm []byte, _ int) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.writes = append(p.writes, append([]byte(nil), pcm...))
	return nil
}

func (p *fakePlay) Duck(on bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.ducks = append(p.ducks, on)
}

func (p *fakePlay) Flush(context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.flushes++
	return nil
}

func (p *fakePlay) flushCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.flushes
}

func (p *fakePlay) duckEvents() []bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]bool(nil), p.ducks...)
}

func (p *fakePlay) firstWrite() []byte {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.writes) == 0 {
		return nil
	}
	return p.writes[0]
}

// events collected through hooks.
type userTurnEv struct {
	text string
	loc  event.Locale
}

type agentTurnEv struct {
	text        string
	interrupted bool
}

type switchEv struct {
	from, to event.Locale
}

type recorder struct {
	userTurns  chan userTurnEv
	agentTurns chan agentTurnEv
	fillers    chan audiobank.Key
	switches   chan switchEv
	bargeIns   chan string
}

func newRecorder() *recorder {
	return &recorder{
		userTurns:  make(chan userTurnEv, 32),
		agentTurns: make(chan agentTurnEv, 32),
		fillers:    make(chan audiobank.Key, 32),
		switches:   make(chan switchEv, 32),
		bargeIns:   make(chan string, 32),
	}
}

func (r *recorder) hooks() turn.Hooks {
	return turn.Hooks{
		OnUserTurn:       func(text string, loc event.Locale) { r.userTurns <- userTurnEv{text, loc} },
		OnAgentTurn:      func(text string, interrupted bool) { r.agentTurns <- agentTurnEv{text, interrupted} },
		OnFiller:         func(key audiobank.Key) { r.fillers <- key },
		OnLanguageSwitch: func(from, to event.Locale) { r.switches <- switchEv{from, to} },
		OnBargeIn:        func(text string) { r.bargeIns <- text },
	}
}

// testBank returns a complete audio bank for both locales.
func testBank() *audiobank.Bank {
	b := audiobank.New("test-persona", "v1")
	for _, loc := range []event.Locale{event.LocaleENCA, event.LocaleFRCA} {
		for _, key := range audiobank.Required() {
			b.Add(loc, key, audiobank.Clip{
				Text:       string(key),
				Audio:      []byte("clip:" + string(key) + ":" + string(loc)),
				DurationMs: 700,
			})
		}
	}
	return b
}

// --- harness --------------------------------------------------------------

type harness struct {
	t       *testing.T
	m       *turn.Machine
	frames  chan media.Frame
	stt     *fakeSTT
	play    *fakePlay
	rec     *recorder
	agent   *fakeAgent
	done    chan error
	stopped chan struct{}
	cancel  context.CancelFunc
	seq     uint64
}

// newHarness starts one machine goroutine; cleanup cancels it and verifies a
// clean exit.
func newHarness(t *testing.T, agent *fakeAgent, tts turn.TTS, opts ...turn.Option) *harness {
	t.Helper()
	h := &harness{
		t:       t,
		frames:  make(chan media.Frame),
		stt:     newFakeSTT(),
		play:    &fakePlay{},
		rec:     newRecorder(),
		agent:   agent,
		done:    make(chan error, 1),
		stopped: make(chan struct{}),
	}
	deps := turn.Deps{
		Frames: h.frames,
		STT:    h.stt,
		Agent:  agent,
		TTS:    tts,
		Play:   h.play,
		Clips:  testBank(),
		Hooks:  h.rec.hooks(),
	}
	h.m = turn.NewMachine(deps, opts...)

	ctx, cancel := context.WithCancel(context.Background())
	h.cancel = cancel
	go func() {
		h.done <- h.m.Run(ctx)
		close(h.stopped)
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-h.stopped:
		case <-time.After(testTimeout):
			t.Fatal("machine did not stop on cancel")
		}
	})
	return h
}

// silence feeds n frames of digital silence.
func (h *harness) silence(n int) {
	for range n {
		h.seq++
		h.frames <- media.Silence(h.seq)
	}
}

// speech feeds n frames of -12 dBFS speech-level audio.
func (h *harness) speech(n int) {
	for range n {
		h.seq++
		h.frames <- media.Synth(h.seq, -12)
	}
}

// noise feeds n frames of -30 dBFS background noise.
func (h *harness) noise(n int) {
	for range n {
		h.seq++
		h.frames <- media.Synth(h.seq, -30)
	}
}

// partial injects an STT partial; returns once the machine consumed it.
func (h *harness) partial(text string, loc event.Locale) {
	h.stt.results <- turn.Result{Text: text, Locale: loc}
}

// final injects an endpointed STT hypothesis.
func (h *harness) final(text string, loc event.Locale) {
	h.stt.results <- turn.Result{Text: text, Final: true, Locale: loc}
}

// waitState polls the machine state.
func (h *harness) waitState(s turn.State) {
	h.t.Helper()
	require.Eventually(h.t, func() bool { return h.m.State() == s },
		testTimeout, time.Millisecond, "waiting for %s (got %s)", s, h.m.State())
}
