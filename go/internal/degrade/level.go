// Package degrade implements the degradation ladder and the failure patterns
// of the voice pipeline (ADR-006): per-call degradation levels, per-provider
// circuit breakers, LLM hedging (never sequential retry), and the STT ring
// buffer with fast reconnection and replay. The design constraint everywhere
// is that the failure budget is measured in milliseconds of audible silence —
// exponential backoff is forbidden on the call path.
package degrade

import "sync"

// Level is one step of the degradation ladder. Every level is functional,
// just worse; the last resort is the owner's cell phone, never silence.
type Level int

// The ladder, in order.
const (
	// Normal: full pipeline (streaming STT, cascaded LLM, streaming TTS).
	Normal Level = iota
	// Fast: smaller model, reduced prompt — trades reasoning for latency.
	Fast
	// Scripted: canned audio-bank phrases + DTMF navigation.
	Scripted
	// Transfer: warm transfer to a human with the context collected so far.
	Transfer
	// Voicemail: record a structured message — the last functional step.
	Voicemail
)

// String returns the canonical deg* name used in metrics and logs.
func (l Level) String() string {
	switch l {
	case Normal:
		return "degNormal"
	case Fast:
		return "degFast"
	case Scripted:
		return "degScripted"
	case Transfer:
		return "degTransfer"
	case Voicemail:
		return "degVoicemail"
	default:
		return "degUnknown"
	}
}

// Ladder tracks the degradation level of ONE call. The decision is per call
// and temporary: it is informed by global provider health (the breakers) but
// never shared — one degraded call must not drag the others down, and Relax
// lets a call climb back when the provider recovers mid-call.
//
// The zero value is a ladder at Normal, ready to use.
type Ladder struct {
	mu       sync.Mutex
	level    Level
	onChange func(from, to Level, reason string)
}

// NewLadder returns a ladder at Normal. onChange, when non-nil, observes every
// transition (metrics: degradation_level distribution).
func NewLadder(onChange func(from, to Level, reason string)) *Ladder {
	return &Ladder{onChange: onChange}
}

// Level returns the current level.
func (l *Ladder) Level() Level {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.level
}

// Escalate moves one step down the ladder and returns the new level. Beyond
// Voicemail there is nothing: it stays at Voicemail (the product floor is the
// owner's phone / voicemail, never a silent hangup).
func (l *Ladder) Escalate(reason string) Level {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.level >= Voicemail {
		return l.level
	}
	return l.set(l.level+1, reason)
}

// EscalateTo moves directly down to at least min (never up).
func (l *Ladder) EscalateTo(min Level, reason string) Level {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.level >= min {
		return l.level
	}
	return l.set(min, reason)
}

// Relax moves one step back toward Normal — degradation is temporary; a
// recovered provider gives the call its full pipeline back on the next turn.
func (l *Ladder) Relax(reason string) Level {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.level == Normal {
		return l.level
	}
	return l.set(l.level-1, reason)
}

// set must be called with mu held.
func (l *Ladder) set(to Level, reason string) Level {
	from := l.level
	l.level = to
	if l.onChange != nil {
		l.onChange(from, to, reason)
	}
	return to
}
