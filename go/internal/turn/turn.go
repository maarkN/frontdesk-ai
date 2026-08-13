// Package turn implements the per-call turn state machine of the telephony
// gateway (nota 06, EPIC-002): one goroutine per call running a select loop
// over stListening → stEndpointing → stThinking → stSpeaking, with STT fed in
// EVERY state (the prerequisite of barge-in), language-adaptive endpointing
// (EN / FR-CA), two-level barge-in (energy → ducking → partial confirmation),
// a PlayoutTracker so history records only what the caller actually HEARD,
// and cascading cancellation with zero goroutine leaks.
package turn

import (
	"context"

	"github.com/maarkn/frontdesk/internal/audiobank"
	"github.com/maarkn/frontdesk/internal/event"
	"github.com/maarkn/frontdesk/internal/media"
)

// State is one state of the turn machine.
type State int32

// The four states of the loop.
const (
	StListening State = iota
	StEndpointing
	StThinking
	StSpeaking
)

// String returns the canonical st* name.
func (s State) String() string {
	switch s {
	case StListening:
		return "stListening"
	case StEndpointing:
		return "stEndpointing"
	case StThinking:
		return "stThinking"
	case StSpeaking:
		return "stSpeaking"
	default:
		return "stUnknown"
	}
}

// Result is one STT hypothesis on the hot path. Partials NEVER reach the
// event bus (ADR-003); they exist only to drive endpointing and barge-in
// confirmation.
type Result struct {
	Text string
	// Final marks an endpointed hypothesis; partials keep mutating.
	Final bool
	// Locale is the language the recognizer heard, empty when unchanged.
	Locale event.Locale
	// Confidence is optional ([0,1], 0 = unknown).
	Confidence float64
}

// STTStream is the speech recognizer of one call. SendFrame is called for
// every inbound frame in every state. The Results channel closes when the
// stream ends for good.
type STTStream interface {
	SendFrame(f media.Frame) error
	Results() <-chan Result
}

// TurnInput is the user turn handed to the agent.
type TurnInput struct {
	Text   string
	Locale event.Locale
}

// Agent produces the reply to one user turn as a token stream. The stream
// must be produced with select on ctx.Done — cancellation is the barge-in
// path and must never leak the producer goroutine.
type Agent interface {
	Reply(ctx context.Context, in TurnInput) (<-chan string, error)
}

// WordTiming is one synthesized word with its position in the utterance
// audio, relative to the start of the CHUNK that carries it.
type WordTiming struct {
	Word    string
	StartMs int64
	EndMs   int64
}

// Chunk is one piece of synthesized audio with its word timings. Word
// timings are mandatory (CONTEXT.md: TTS provider must supply them — the
// PlayoutTracker depends on it).
type Chunk struct {
	PCM        []byte
	DurationMs int64
	Words      []WordTiming
}

// TTS synthesizes a streamed text into audio chunks. The returned channel
// closes when the utterance is fully synthesized; the producer must select
// on ctx.Done.
type TTS interface {
	Speak(ctx context.Context, text <-chan string) (<-chan Chunk, error)
}

// Playback is the outbound half of the line. Write must NOT block the
// caller (the machine loop is latency-critical); implementations buffer.
type Playback interface {
	Write(ctx context.Context, pcm []byte, durationMs int) error
	// Duck lowers (true) / restores (false) playout volume — barge-in
	// level 1, applied on energy onset within ~60ms.
	Duck(on bool)
	// Flush drops everything queued on the line — the barge-in hard cut.
	Flush(ctx context.Context) error
}

// ClipSource looks up pre-synthesized phrases (filler, canned fallbacks).
// *audiobank.Bank satisfies it.
type ClipSource interface {
	Lookup(key audiobank.Key, loc event.Locale) (audiobank.Clip, error)
}

// Hooks observe the machine. All fields are optional; the wiring layer uses
// them to emit business events (stt.final → bus, agent.turn.completed, ...)
// and metrics, tests use them as the harness probes.
type Hooks struct {
	// OnStateChange fires on every transition.
	OnStateChange func(from, to State)
	// OnUserTurn fires when endpointing closes a user turn.
	OnUserTurn func(text string, loc event.Locale)
	// OnAgentTurn fires when an agent turn ends; text is what the caller
	// HEARD (truncated at the last fully played word, "—" marks a cut).
	OnAgentTurn func(text string, interrupted bool)
	// OnFiller fires when the filler clip is played (LLM slower than
	// fillerAfter).
	OnFiller func(key audiobank.Key)
	// OnLanguageSwitch fires on mid-call language change — the machine keeps
	// running, no node restart.
	OnLanguageSwitch func(from, to event.Locale)
	// OnDucking fires on duck (true) / unduck (false).
	OnDucking func(on bool)
	// OnBargeIn fires when a barge-in is CONFIRMED by a real partial.
	OnBargeIn func(text string)
	// OnError reports non-fatal pipeline errors (degradation hooks).
	OnError func(stage string, err error)
}

func (h Hooks) stateChange(from, to State) {
	if h.OnStateChange != nil {
		h.OnStateChange(from, to)
	}
}

func (h Hooks) userTurn(text string, loc event.Locale) {
	if h.OnUserTurn != nil {
		h.OnUserTurn(text, loc)
	}
}

func (h Hooks) agentTurn(text string, interrupted bool) {
	if h.OnAgentTurn != nil {
		h.OnAgentTurn(text, interrupted)
	}
}

func (h Hooks) filler(key audiobank.Key) {
	if h.OnFiller != nil {
		h.OnFiller(key)
	}
}

func (h Hooks) languageSwitch(from, to event.Locale) {
	if h.OnLanguageSwitch != nil {
		h.OnLanguageSwitch(from, to)
	}
}

func (h Hooks) ducking(on bool) {
	if h.OnDucking != nil {
		h.OnDucking(on)
	}
}

func (h Hooks) bargeIn(text string) {
	if h.OnBargeIn != nil {
		h.OnBargeIn(text)
	}
}

func (h Hooks) error(stage string, err error) {
	if h.OnError != nil {
		h.OnError(stage, err)
	}
}
