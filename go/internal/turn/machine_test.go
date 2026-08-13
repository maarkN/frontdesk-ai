package turn_test

// Scenarios mirroring the nota 06 harness: barge-in truncation, backchannel,
// number dictation, slow LLM → filler, EN→FR switch without restart, low
// noise → zero false barge-ins, and leak-free cancellation in every state.

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/maarkn/frontdesk/internal/audiobank"
	"github.com/maarkn/frontdesk/internal/event"
	"github.com/maarkn/frontdesk/internal/turn"
)

// TestBargeInTruncatesToWhatWasHeard: the caller talks over the bot at
// ~1.2s of playout; Spoken() must match exactly the words that made it out
// of the line, with the "—" cut marker, and the pipeline must be flushed.
func TestBargeInTruncatesToWhatWasHeard(t *testing.T) {
	agent := &fakeAgent{reply: "I can send a technician tomorrow morning okay"}
	h := newHarness(t, agent, &fakeTTS{wordMs: 300},
		turn.WithPlayoutOffsetMs(0), // calibration under test elsewhere
	)

	// User turn: partial at media clock 0, then 600ms of silence (default
	// endpoint profile) closes the turn.
	h.partial("my kitchen sink is leaking", event.LocaleENCA)
	h.silence(30)
	ut := recv(t, h.rec.userTurns, "user turn")
	require.Equal(t, "my kitchen sink is leaking", ut.text)

	// Bot speaks; 1200ms of playout goes by.
	h.waitState(turn.StSpeaking)
	h.silence(60)

	// Caller talks over: energy onset (3 frames = 60ms) ducks the line...
	h.speech(3)
	require.Eventually(t, func() bool { return len(h.play.duckEvents()) == 1 },
		testTimeout, time.Millisecond, "ducking within the 100ms budget")
	require.True(t, h.play.duckEvents()[0])

	// ...and a real partial (≥ minChars, not backchannel) confirms the cut.
	h.partial("wait actually can you come today", event.LocaleENCA)

	at := recv(t, h.rec.agentTurns, "interrupted agent turn")
	require.True(t, at.interrupted)
	// 63 frames of playout = 1260ms: words ending at 300..1200ms were heard
	// ("I can send a"), "technician" (ends 1500ms) was not.
	require.Equal(t, "I can send a —", at.text)

	require.Equal(t, 1, h.play.flushCount(), "FlushPlayback exactly once")
	require.Equal(t, "wait actually can you come today", recv(t, h.rec.bargeIns, "barge-in"))

	// The interrupting words opened the caller's next turn. The energy
	// meter's hangover (10 frames) still counts as speech, then 600ms of
	// silence closes the turn.
	h.silence(45)
	ut2 := recv(t, h.rec.userTurns, "post-barge user turn")
	require.Equal(t, "wait actually can you come today", ut2.text)
}

// TestBackchannelDoesNotInterrupt: "mhm" (EN) and "ouais" (FR) undo ducking
// and never cut the bot; the utterance completes and history carries the
// full text.
func TestBackchannelDoesNotInterrupt(t *testing.T) {
	cases := []struct {
		name        string
		locale      event.Locale
		backchannel string
	}{
		{name: "en mhm", locale: event.LocaleENCA, backchannel: "mhm"},
		{name: "fr ouais", locale: event.LocaleFRCA, backchannel: "ouais"},
		{name: "fr han han", locale: event.LocaleFRCA, backchannel: "han han"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			agent := &fakeAgent{reply: "we can fit you in tomorrow at nine"}
			h := newHarness(t, agent, &fakeTTS{wordMs: 300},
				turn.WithLocale(tc.locale), turn.WithPlayoutOffsetMs(0))

			h.partial("hello there friend", tc.locale)
			h.silence(30)
			recv(t, h.rec.userTurns, "user turn")

			h.waitState(turn.StSpeaking)
			h.silence(20)

			// Backchannel: energy ducks, the partial un-ducks, nothing cut.
			h.speech(3)
			require.Eventually(t, func() bool { return len(h.play.duckEvents()) == 1 },
				testTimeout, time.Millisecond)
			h.partial(tc.backchannel, tc.locale)
			require.Eventually(t, func() bool {
				d := h.play.duckEvents()
				return len(d) == 2 && !d[1]
			}, testTimeout, time.Millisecond, "backchannel undoes ducking")

			// Let the utterance play to the end (8 words × 300ms = 2400ms).
			h.silence(130)
			at := recv(t, h.rec.agentTurns, "agent turn")
			require.False(t, at.interrupted, "backchannel must not cut")
			require.Equal(t, "we can fit you in tomorrow at nine", at.text)
			require.Zero(t, h.play.flushCount(), "no flush on backchannel")
		})
	}
}

// TestNumberDictationSurvives900msPause: dictating digits selects the
// ~1100ms profile, so a 900ms pause does NOT close the turn.
func TestNumberDictationSurvives900msPause(t *testing.T) {
	agent := &fakeAgent{reply: "got it"}
	h := newHarness(t, agent, &fakeTTS{wordMs: 200})

	h.partial("my number is five five five", event.LocaleENCA)
	h.silence(45) // 900ms of silence — inside the 1100ms number window
	require.Eventually(t, func() bool { return h.stt.framesSeen() == 45 },
		testTimeout, time.Millisecond)
	require.Empty(t, h.rec.userTurns, "900ms pause must not close a number dictation")
	require.Equal(t, turn.StEndpointing, h.m.State())

	// The caller resumes and finishes; 1120ms of silence closes the turn.
	h.partial("my number is five five five one two one two", event.LocaleENCA)
	h.silence(56)

	ut := recv(t, h.rec.userTurns, "dictation turn")
	require.Equal(t, "my number is five five five one two one two", ut.text)
	require.Len(t, h.rec.userTurns, 0, "exactly one user turn")
}

// TestSlowLLMPlaysFiller: first token beyond fillerAfter → the audio-bank
// filler covers the silence before the reply starts.
func TestSlowLLMPlaysFiller(t *testing.T) {
	agent := &fakeAgent{reply: "sure thing", delay: 150 * time.Millisecond}
	h := newHarness(t, agent, &fakeTTS{wordMs: 200},
		turn.WithFillerAfter(40*time.Millisecond))

	h.partial("can you book me for tomorrow morning", event.LocaleENCA)
	h.silence(30) // question profile would be 400ms; 600ms covers any path
	recv(t, h.rec.userTurns, "user turn")

	key := recv(t, h.rec.fillers, "filler")
	require.Equal(t, audiobank.KeyOneMoment, key)
	require.Equal(t, []byte("clip:one_moment:en-CA"), h.play.firstWrite(),
		"filler clip audio hits the line before the reply")

	// The reply still arrives and plays after the filler.
	h.waitState(turn.StSpeaking)
	h.silence(30)
	at := recv(t, h.rec.agentTurns, "agent turn")
	require.False(t, at.interrupted)
	require.Equal(t, "sure thing", at.text)
}

// TestLanguageSwitchMidCallWithoutRestart: the recognizer flips to FR-CA in
// the middle of the call; the same machine keeps going, and the FR
// endpointing profile (hesitation "euh" → ~1400ms) applies immediately.
func TestLanguageSwitchMidCallWithoutRestart(t *testing.T) {
	agent := &fakeAgent{reply: "hello how can I help"}
	h := newHarness(t, agent, &fakeTTS{wordMs: 100})

	// Turn 1 in EN.
	h.partial("hi I have a problem", event.LocaleENCA)
	h.silence(30)
	ut := recv(t, h.rec.userTurns, "turn 1")
	require.Equal(t, event.LocaleENCA, ut.loc)
	h.waitState(turn.StSpeaking)
	h.silence(40)
	recv(t, h.rec.agentTurns, "agent turn 1")

	// Turn 2 starts in FR: switch fires, machine does NOT restart.
	h.partial("euh", event.LocaleFRCA)
	sw := recv(t, h.rec.switches, "language switch")
	require.Equal(t, switchEv{from: event.LocaleENCA, to: event.LocaleFRCA}, sw)

	// FR hesitation window: 900ms of silence does not close the turn...
	h.silence(45)
	require.Eventually(t, func() bool { return h.m.State() == turn.StEndpointing },
		testTimeout, time.Millisecond)
	require.Empty(t, h.rec.userTurns, `"euh" holds the turn open (~1400ms window)`)

	// ...the caller finishes in French and the turn closes normally.
	h.final("euh j'ai un dégât d'eau chez moi", event.LocaleFRCA)
	h.silence(30)
	ut2 := recv(t, h.rec.userTurns, "turn 2")
	require.Equal(t, "euh j'ai un dégât d'eau chez moi", ut2.text)
	require.Equal(t, event.LocaleFRCA, ut2.loc)
	require.Equal(t, event.LocaleFRCA, agent.lastInput().Locale,
		"agent sees the switched locale, same machine, no node restart")
}

// TestLowNoiseNeverTriggersBargeIn: 60s of -30 dBFS background noise during
// playback → zero ducks, zero flushes, and STT stays fed the whole time.
func TestLowNoiseNeverTriggersBargeIn(t *testing.T) {
	reply := make([]string, 100) // 100 words × 600ms = 60s of speech
	for i := range reply {
		reply[i] = "word"
	}
	agent := &fakeAgent{reply: joinWords(reply)}
	h := newHarness(t, agent, &fakeTTS{wordMs: 600})

	h.partial("tell me everything about your services", event.LocaleENCA)
	h.silence(30)
	recv(t, h.rec.userTurns, "user turn")
	h.waitState(turn.StSpeaking)

	framesBefore := h.stt.framesSeen()
	h.noise(3000) // 60 seconds of background noise under the bot's speech
	require.Eventually(t, func() bool { return h.stt.framesSeen() == framesBefore+3000 },
		testTimeout, time.Millisecond,
		"STT is fed in stSpeaking too — barge-in depends on it")
	require.Empty(t, h.play.duckEvents(), "zero false barge-ins in 60s of noise")
	require.Zero(t, h.play.flushCount())
}

// TestHangupMidThinkingCancelsCascade: the caller hangs up while the LLM is
// mid-flight; Run returns and the goleak TestMain proves the producers died.
func TestHangupMidThinkingCancelsCascade(t *testing.T) {
	agent := &fakeAgent{never: true} // LLM hangs forever
	h := newHarness(t, agent, &fakeTTS{wordMs: 200})

	h.partial("hello I need some help please", event.LocaleENCA)
	h.silence(30)
	recv(t, h.rec.userTurns, "user turn")
	h.waitState(turn.StThinking)

	h.cancel() // hangup
	require.ErrorIs(t, recv(t, h.done, "machine exit"), context.Canceled)
}

// TestHangupMidSpeakingCancelsCascade: hangup during playback.
func TestHangupMidSpeakingCancelsCascade(t *testing.T) {
	agent := &fakeAgent{reply: "a very long reply that keeps playing for a while"}
	h := newHarness(t, agent, &fakeTTS{wordMs: 1000})

	h.partial("hello I need some help please", event.LocaleENCA)
	h.silence(30)
	recv(t, h.rec.userTurns, "user turn")
	h.waitState(turn.StSpeaking)

	h.cancel()
	require.ErrorIs(t, recv(t, h.done, "machine exit"), context.Canceled)
}

// TestMediaStreamEndStopsMachine: the frames channel closing (carrier ended
// the stream) stops Run cleanly.
func TestMediaStreamEndStopsMachine(t *testing.T) {
	agent := &fakeAgent{reply: "hi"}
	h := newHarness(t, agent, &fakeTTS{wordMs: 100})

	h.silence(5)
	close(h.frames)
	require.NoError(t, recv(t, h.done, "machine exit"))
}

// TestFullTurnStateSequence: the canonical loop listening → endpointing →
// thinking → speaking → listening.
func TestFullTurnStateSequence(t *testing.T) {
	agent := &fakeAgent{reply: "hello there"}
	h := newHarness(t, agent, &fakeTTS{wordMs: 100})

	h.partial("good morning to you", event.LocaleENCA)
	h.silence(1)
	h.waitState(turn.StEndpointing)
	h.silence(29)
	recv(t, h.rec.userTurns, "user turn")
	h.waitState(turn.StSpeaking)
	h.silence(15) // 300ms plays the 200ms utterance out
	at := recv(t, h.rec.agentTurns, "agent turn")
	require.Equal(t, "hello there", at.text)
	require.False(t, at.interrupted)
	h.waitState(turn.StListening)
}

func joinWords(ws []string) string {
	out := ""
	for i, w := range ws {
		if i > 0 {
			out += " "
		}
		out += w
	}
	return out
}
