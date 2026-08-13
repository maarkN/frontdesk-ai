package turn

import "strings"

// PlayoutTracker reconciles what was WRITTEN to the line with what the
// caller actually HEARD (nota 06): conversation history must contain only
// heard speech. Written audio is heard playoutOffsetMs later (transport +
// carrier buffer, calibrated per deployment); on barge-in the utterance is
// truncated at the last fully played word and the cut is marked with "—".
//
// It is owned by the machine goroutine; not safe for concurrent use.
type PlayoutTracker struct {
	offsetMs  int64
	writtenMs int64
	words     []WordTiming // StartMs/EndMs relative to utterance start
}

// NewPlayoutTracker returns a tracker with the given written→heard offset.
func NewPlayoutTracker(offsetMs int64) *PlayoutTracker {
	return &PlayoutTracker{offsetMs: offsetMs}
}

// AddChunk records a chunk about to be written: its word timings (relative
// to the chunk) are rebased onto the utterance clock, then the written
// counter advances by the chunk duration.
func (t *PlayoutTracker) AddChunk(c Chunk) {
	for _, w := range c.Words {
		t.words = append(t.words, WordTiming{
			Word:    w.Word,
			StartMs: w.StartMs + t.writtenMs,
			EndMs:   w.EndMs + t.writtenMs,
		})
	}
	t.writtenMs += c.DurationMs
}

// WrittenMs returns how much audio has been written to the line.
func (t *PlayoutTracker) WrittenMs() int64 { return t.writtenMs }

// FullText returns the complete utterance text as synthesized.
func (t *PlayoutTracker) FullText() string {
	parts := make([]string, len(t.words))
	for i, w := range t.words {
		parts[i] = w.Word
	}
	return strings.Join(parts, " ")
}

// Spoken returns what the caller heard after playedMs of line playout: every
// word fully played before (playedMs − offset), and cut=true when the
// utterance was truncated — the text then carries the "—" cut marker.
func (t *PlayoutTracker) Spoken(playedMs int64) (text string, cut bool) {
	heardMs := playedMs - t.offsetMs
	if heardMs < 0 {
		heardMs = 0
	}
	var parts []string
	for _, w := range t.words {
		if w.EndMs > heardMs {
			break
		}
		parts = append(parts, w.Word)
	}
	cut = len(parts) < len(t.words)
	text = strings.Join(parts, " ")
	if cut {
		if text == "" {
			text = "—"
		} else {
			text += " —"
		}
	}
	return text, cut
}
