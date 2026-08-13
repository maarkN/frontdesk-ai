package turn

import (
	"context"
	"strings"
	"sync/atomic"
	"time"

	"github.com/maarkn/frontdesk/internal/audiobank"
	"github.com/maarkn/frontdesk/internal/event"
	"github.com/maarkn/frontdesk/internal/media"
)

// Deps are the collaborators of one call's machine. All providers sit behind
// interfaces (fakes in tests, degradation-wrapped implementations in prod).
type Deps struct {
	// Frames is the inbound caller audio (post jitter buffer). The channel
	// closing means the media stream ended.
	Frames <-chan media.Frame
	STT    STTStream
	Agent  Agent
	TTS    TTS
	Play   Playback
	Clips  ClipSource
	Hooks  Hooks
}

// Machine is the turn state machine of ONE call, run by exactly one
// goroutine (Run). All mutable state below is owned by that goroutine —
// no mutex by design; the only cross-goroutine reads go through the atomic
// state.
type Machine struct {
	cfg   Config
	deps  Deps
	meter *media.Meter

	state atomic.Int32

	// media clock: advances 20ms per inbound frame. All endpointing and
	// playout arithmetic uses it, which makes the machine testable at full
	// speed with synthetic frames (nota 06 harness).
	mediaMs      int64
	lastSpeechMs int64
	prevSpeaking bool

	// user-turn draft
	activeLocale event.Locale
	carryText    string   // text of a turn whose thinking was cancelled
	finals       []string // finalized STT segments of the draft
	lastPart     string   // latest partial

	// active agent turn (stThinking/stSpeaking)
	turnCancel   context.CancelFunc
	sttCh        <-chan Result
	ttsCh        <-chan Chunk
	fillerTimer  *time.Timer
	fillerC      <-chan time.Time
	tracker      *PlayoutTracker
	ttsDone      bool
	speakStartMs int64
	ducked       bool
	duckStartMs  int64
}

// NewMachine builds the machine for one call.
func NewMachine(deps Deps, opts ...Option) *Machine {
	cfg := defaultConfig()
	for _, o := range opts {
		o(&cfg)
	}
	return &Machine{
		cfg:          cfg,
		deps:         deps,
		meter:        media.NewMeter(cfg.Meter...),
		activeLocale: cfg.Locale,
	}
}

// State returns the current state (safe from any goroutine).
func (m *Machine) State() State { return State(m.state.Load()) }

func (m *Machine) curLocale() event.Locale    { return m.activeLocale }
func (m *Machine) setLocale(loc event.Locale) { m.activeLocale = loc }

// Run executes the loop until ctx is done or the media stream closes. It
// owns every turn goroutine: on return, cascading cancellation has reaped
// agent and TTS producers (goleak-verified).
func (m *Machine) Run(ctx context.Context) error {
	defer m.cancelTurn()
	m.setState(StListening)
	m.sttCh = m.deps.STT.Results()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()

		case f, ok := <-m.deps.Frames:
			if !ok {
				return nil
			}
			m.onFrame(ctx, f)

		case r, ok := <-m.sttCh:
			if !ok {
				m.sttCh = nil // stream gone; degradation wiring reprovisions
				continue
			}
			m.onSTT(ctx, r)

		case c, ok := <-m.ttsCh:
			m.onChunk(ctx, c, ok)

		case <-m.fillerC:
			m.onFiller(ctx)
		}
	}
}

// --- frame path -----------------------------------------------------------

func (m *Machine) onFrame(ctx context.Context, f media.Frame) {
	m.mediaMs += media.FrameMs

	// STT is fed in EVERY state — the prerequisite of barge-in (nota 06).
	if err := m.deps.STT.SendFrame(f); err != nil {
		m.deps.Hooks.error("stt", err)
	}

	speaking := m.meter.Process(f)
	onset := speaking && !m.prevSpeaking
	m.prevSpeaking = speaking
	if speaking {
		m.lastSpeechMs = m.mediaMs
	}

	switch m.State() {
	case StListening:
		if m.hasDraft() && !speaking && m.mediaMs > m.lastSpeechMs {
			m.setState(StEndpointing)
		}

	case StEndpointing:
		if speaking {
			m.setState(StListening)
			return
		}
		wait := waitForMs(m.draft(), m.curLocale(), m.cfg.Endpoint)
		if m.mediaMs-m.lastSpeechMs >= wait {
			m.finalizeUserTurn(ctx)
		}

	case StThinking:
		// The caller kept talking while we think: what we captured is
		// stale. Cancel the reply and fold the turn back into listening.
		if onset {
			m.rethink()
		}

	case StSpeaking:
		if onset && !m.ducked {
			m.duck(true)
		}
		// Energy that STT never confirmed within the window: undo ducking
		// (cough, noise burst — not a barge-in).
		if m.ducked && m.mediaMs-m.duckStartMs >= m.cfg.ConfirmWindowMs {
			m.duck(false)
		}
		m.maybeFinishSpeaking()
	}
}

// maybeFinishSpeaking completes the agent turn once TTS closed and the line
// played everything written.
func (m *Machine) maybeFinishSpeaking() {
	if !m.ttsDone || m.tracker == nil {
		return
	}
	if m.playedMs() >= m.tracker.WrittenMs() {
		m.completeAgentTurn()
	}
}

// playedMs is how much of the current utterance the line has played out,
// on the media clock.
func (m *Machine) playedMs() int64 {
	played := m.mediaMs - m.speakStartMs
	if m.tracker != nil && played > m.tracker.WrittenMs() {
		played = m.tracker.WrittenMs()
	}
	return played
}

// --- STT path -------------------------------------------------------------

func (m *Machine) onSTT(ctx context.Context, r Result) {
	if r.Locale != "" && r.Locale != m.curLocale() {
		from := m.curLocale()
		m.setLocale(r.Locale)
		// Mid-call switch: endpointing profile and clips follow the new
		// locale immediately; the machine (and the flow node) keeps going.
		m.deps.Hooks.languageSwitch(from, r.Locale)
	}
	if strings.TrimSpace(r.Text) == "" {
		return
	}

	switch m.State() {
	case StListening, StEndpointing:
		m.absorb(r)
		// Text is speech evidence even when the energy gate missed it.
		m.lastSpeechMs = m.mediaMs
		if m.State() == StEndpointing {
			m.setState(StListening)
		}

	case StThinking:
		// Real new content invalidates the reply being computed.
		if isBackchannel(r.Text, m.curLocale()) {
			return
		}
		m.rethink()
		m.absorb(r)
		m.lastSpeechMs = m.mediaMs

	case StSpeaking:
		m.onSpeakingSTT(ctx, r)
	}
}

// onSpeakingSTT is barge-in level 2: the partial decides.
func (m *Machine) onSpeakingSTT(ctx context.Context, r Result) {
	if isBackchannel(r.Text, m.curLocale()) {
		// "uh huh" / "ouais" / "han han": the caller is following along.
		// Undo ducking, never cut (nota 06).
		if m.ducked {
			m.duck(false)
		}
		return
	}
	if realTextLen(r.Text) < m.cfg.MinBargeChars {
		return // too short to confirm; ducking (if any) keeps waiting
	}
	m.confirmBargeIn(ctx, r)
}

// confirmBargeIn is the hard cut: cancel LLM/TTS, flush the line, close the
// agent turn with ONLY what was heard, and hand the floor to the caller.
func (m *Machine) confirmBargeIn(ctx context.Context, r Result) {
	played := m.playedMs()
	text, cut := "", true
	if m.tracker != nil {
		text, cut = m.tracker.Spoken(played)
	}
	if !cut {
		// TTS may still have been synthesizing beyond the tracked words:
		// the utterance is interrupted regardless.
		text = strings.TrimSpace(text + " —")
	}

	m.cancelTurn() // cascading cancellation: agent + TTS producers die
	if err := m.deps.Play.Flush(ctx); err != nil {
		m.deps.Hooks.error("playback", err)
	}
	if m.ducked {
		m.duck(false)
	}

	m.deps.Hooks.agentTurn(text, true)
	m.deps.Hooks.bargeIn(r.Text)

	// The confirming words open the caller's next turn.
	m.resetDraft()
	m.absorb(r)
	m.lastSpeechMs = m.mediaMs
	m.setState(StListening)
}

// absorb merges an STT result into the draft.
func (m *Machine) absorb(r Result) {
	if r.Final {
		m.finals = append(m.finals, strings.TrimSpace(r.Text))
		m.lastPart = ""
		return
	}
	m.lastPart = strings.TrimSpace(r.Text)
}

func (m *Machine) hasDraft() bool { return m.draft() != "" }

// draft assembles the caller-turn text captured so far.
func (m *Machine) draft() string {
	parts := make([]string, 0, len(m.finals)+2)
	if m.carryText != "" {
		parts = append(parts, m.carryText)
	}
	parts = append(parts, m.finals...)
	if m.lastPart != "" {
		parts = append(parts, m.lastPart)
	}
	return strings.TrimSpace(strings.Join(parts, " "))
}

func (m *Machine) resetDraft() {
	m.carryText = ""
	m.finals = nil
	m.lastPart = ""
}

// --- turn lifecycle -------------------------------------------------------

// finalizeUserTurn closes the caller's turn and starts the agent's.
func (m *Machine) finalizeUserTurn(ctx context.Context) {
	text := m.draft()
	if text == "" {
		m.setState(StListening)
		return
	}
	m.deps.Hooks.userTurn(text, m.curLocale())
	// Keep the text: if thinking gets cancelled by more caller speech, the
	// next turn must include it (the agent never saw a reply out).
	m.carryText = text
	m.finals = nil
	m.lastPart = ""
	m.startAgentTurn(ctx, text)
}

// startAgentTurn spins the reply pipeline: agent tokens → TTS → playback.
// Every producer hangs off turnCtx; cancelling it reaps the whole cascade.
func (m *Machine) startAgentTurn(ctx context.Context, text string) {
	turnCtx, cancel := context.WithCancel(ctx)
	m.turnCancel = cancel

	tokens, err := m.deps.Agent.Reply(turnCtx, TurnInput{Text: text, Locale: m.curLocale()})
	if err != nil {
		m.deps.Hooks.error("agent", err)
		m.failTurn(ctx)
		return
	}
	ttsIn := make(chan string)
	chunks, err := m.deps.TTS.Speak(turnCtx, ttsIn)
	if err != nil {
		m.deps.Hooks.error("tts", err)
		m.failTurn(ctx)
		return
	}

	// Pump agent tokens into TTS. Producer pattern of CONTEXT.md §3: every
	// send selects on ctx.Done — zero leaked goroutines on cancellation.
	go func() {
		defer close(ttsIn)
		for {
			select {
			case <-turnCtx.Done():
				return
			case tok, ok := <-tokens:
				if !ok {
					return
				}
				select {
				case ttsIn <- tok:
				case <-turnCtx.Done():
					return
				}
			}
		}
	}()

	m.ttsCh = chunks
	m.ttsDone = false
	m.tracker = NewPlayoutTracker(m.cfg.PlayoutOffsetMs)
	m.fillerTimer = time.NewTimer(m.cfg.FillerAfter)
	m.fillerC = m.fillerTimer.C
	m.setState(StThinking)
}

// failTurn plays the canned technical-issue phrase (audio bank: 0ms, no
// provider) so the caller never gets silence, then goes back to listening.
// Escalation beyond this (degTransfer etc.) is the wiring's job via OnError.
func (m *Machine) failTurn(ctx context.Context) {
	m.cancelTurn()
	if clip, err := m.deps.Clips.Lookup(audiobank.KeyTechnicalIssue, m.curLocale()); err == nil {
		if werr := m.deps.Play.Write(ctx, clip.Audio, clip.DurationMs); werr != nil {
			m.deps.Hooks.error("playback", werr)
		}
	}
	m.setState(StListening)
}

// rethink cancels the in-flight reply because the caller kept talking; the
// captured text is preserved (carryText) and the draft reopens.
func (m *Machine) rethink() {
	m.cancelTurn()
	m.setState(StListening)
}

// onChunk handles TTS output. The first chunk flips thinking→speaking and
// kills the filler; the channel closing marks synthesis done.
func (m *Machine) onChunk(ctx context.Context, c Chunk, ok bool) {
	if !ok {
		m.ttsCh = nil
		m.ttsDone = true
		if m.State() == StThinking {
			// Empty utterance (agent produced no speakable text): nothing
			// to play, turn over.
			m.completeAgentTurn()
		}
		return
	}
	if m.State() == StThinking {
		m.stopFiller()
		m.speakStartMs = m.mediaMs
		m.setState(StSpeaking)
	}
	m.tracker.AddChunk(c)
	if err := m.deps.Play.Write(ctx, c.PCM, int(c.DurationMs)); err != nil {
		m.deps.Hooks.error("playback", err)
	}
}

// onFiller plays the audio-bank filler once: the LLM is taking longer than
// the persona's silence budget (~600ms).
func (m *Machine) onFiller(ctx context.Context) {
	m.fillerC = nil
	if m.State() != StThinking {
		return
	}
	clip, err := m.deps.Clips.Lookup(m.cfg.FillerKey, m.curLocale())
	if err != nil {
		m.deps.Hooks.error("audiobank", err)
		return
	}
	if err := m.deps.Play.Write(ctx, clip.Audio, clip.DurationMs); err != nil {
		m.deps.Hooks.error("playback", err)
	}
	m.deps.Hooks.filler(m.cfg.FillerKey)
}

// completeAgentTurn closes a fully played utterance. Spoken() may still
// truncate the very tail (playout offset): the history keeps honesty over
// polish and records only what was heard.
func (m *Machine) completeAgentTurn() {
	text := ""
	if m.tracker != nil {
		text, _ = m.tracker.Spoken(m.playedMs() + m.cfg.PlayoutOffsetMs)
	}
	m.cancelTurn()
	m.deps.Hooks.agentTurn(text, false)
	m.resetDraft()
	m.setState(StListening)
}

// --- plumbing -------------------------------------------------------------

// cancelTurn tears the active reply pipeline down (idempotent).
func (m *Machine) cancelTurn() {
	if m.turnCancel != nil {
		m.turnCancel()
		m.turnCancel = nil
	}
	m.ttsCh = nil
	m.ttsDone = false
	m.stopFiller()
}

func (m *Machine) stopFiller() {
	if m.fillerTimer != nil {
		m.fillerTimer.Stop()
		m.fillerTimer = nil
	}
	m.fillerC = nil
}

func (m *Machine) duck(on bool) {
	m.ducked = on
	if on {
		m.duckStartMs = m.mediaMs
	}
	m.deps.Play.Duck(on)
	m.deps.Hooks.ducking(on)
}

func (m *Machine) setState(s State) {
	from := State(m.state.Swap(int32(s)))
	if from != s {
		m.deps.Hooks.stateChange(from, s)
	}
}
