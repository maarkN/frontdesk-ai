package event_test

import (
	"math/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/maarkn/frontdesk/internal/event"
)

const (
	testCallID = "call-123"
	testTenant = "tenant-abc"
	testFlow   = "flow-plumber"
)

// mkEvent builds an envelope with a deterministic timestamp derived from seq.
func mkEvent(t *testing.T, seq uint64, typ event.Type, nodeID string, payload any, reds []event.Redaction) event.Envelope {
	t.Helper()
	ts := time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC).Add(time.Duration(seq) * time.Second)
	ev, err := event.New(testCallID, seq, ts, testTenant, testFlow, 3, typ, nodeID, payload, reds)
	require.NoError(t, err)
	return ev
}

// callFixture is a full call: greeting in EN, switch to FR, qualification,
// booking, transfer-free happy end.
func callFixture(t *testing.T) []event.Envelope {
	t.Helper()
	return []event.Envelope{
		mkEvent(t, 1, event.TypeCallStarted, "", event.CallStarted{From: "+15145550101", To: "+15145550100", Mode: event.ModeOverflow}, nil),
		mkEvent(t, 2, event.TypeCallAnswered, "", event.CallAnswered{AnsweredBy: "agent", AnswerDelayMs: 4000}, nil),
		mkEvent(t, 3, event.TypeConsentCaptured, "consent", event.ConsentCaptured{Granted: true, Method: "verbal"}, nil),
		mkEvent(t, 4, event.TypeRecordingStarted, "consent", event.RecordingStarted{RecordingID: "rec-1"}, nil),
		mkEvent(t, 5, event.TypeLanguageDetected, "greeting", event.LanguageDetected{Locale: event.LocaleENCA, Confidence: 0.97}, nil),
		mkEvent(t, 6, event.TypeNodeEntered, "greeting", event.NodeEntered{NodeType: "say"}, nil),
		mkEvent(t, 7, event.TypeSTTFinal, "greeting", event.STTFinal{
			Text: "hi my postal code is [redacted:postal_code]", Locale: event.LocaleENCA, AudioMs: 1500, VadEndToFinalMs: 140,
		}, []event.Redaction{{Start: 21, End: 28, Kind: event.KindPostalCode, Vault: "v-1"}}),
		mkEvent(t, 8, event.TypeAgentTurnCompleted, "greeting", event.AgentTurnCompleted{
			Text: "Thanks! What service do you need?", LLMTokensIn: 400, LLMTokensOut: 30, TTSChars: 33,
			Latency: event.TurnLatency{VadEndToSTTFinalMs: 140, STTFinalToLLMFirstTokenMs: 380, LLMFirstTokenToTTSFirstChunkMs: 190, PerceivedMs: 1100},
		}, nil),
		mkEvent(t, 9, event.TypeLanguageSwitched, "greeting", event.LanguageSwitched{From: event.LocaleENCA, To: event.LocaleFRCA}, nil),
		mkEvent(t, 10, event.TypeNodeExited, "greeting", event.NodeExited{Outcome: "ok"}, nil),
		mkEvent(t, 11, event.TypeNodeEntered, "qualify", event.NodeEntered{NodeType: "collect"}, nil),
		mkEvent(t, 12, event.TypeSTTFinal, "qualify", event.STTFinal{
			Text: "j'ai un dégât d'eau urgent", Locale: event.LocaleFRCA, AudioMs: 2500, VadEndToFinalMs: 160,
		}, nil),
		mkEvent(t, 13, event.TypeVarAssigned, "qualify", event.VarAssigned{Name: "service", Value: []byte(`"plomberie"`)}, nil),
		mkEvent(t, 14, event.TypeDTMFReceived, "qualify", event.DTMFReceived{Digits: "1"}, nil),
		mkEvent(t, 15, event.TypeNodeEntered, "book", event.NodeEntered{NodeType: "tool"}, nil),
		mkEvent(t, 16, event.TypeToolInvoked, "book", event.ToolInvoked{Tool: "calendar.book"}, nil),
		mkEvent(t, 17, event.TypeToolCompleted, "book", event.ToolCompleted{Tool: "calendar.book", OK: true, DurationMs: 800}, nil),
		mkEvent(t, 18, event.TypeAgentTurnCompleted, "book", event.AgentTurnCompleted{
			Text: "C'est réservé pour demain 9h —", Interrupted: true, LLMTokensIn: 900, LLMTokensOut: 45, TTSChars: 30,
			Latency: event.TurnLatency{VadEndToSTTFinalMs: 160, STTFinalToLLMFirstTokenMs: 420, LLMFirstTokenToTTSFirstChunkMs: 210, PerceivedMs: 1300},
		}, nil),
		mkEvent(t, 19, event.TypeErrorRaised, "book", event.ErrorRaised{Code: "tts_slow", Message: "tts p99 above budget", Stage: "fast"}, nil),
		mkEvent(t, 20, event.TypeCallEnded, "", event.CallEnded{Reason: event.EndCompleted, DurationMs: 125_000}, nil),
	}
}

func TestFoldSnapshot(t *testing.T) {
	snap, err := event.Fold(callFixture(t))
	require.NoError(t, err)

	require.Equal(t, testCallID, snap.CallID)
	require.Equal(t, testTenant, snap.TenantID)
	require.Equal(t, testFlow, snap.FlowID)
	require.Equal(t, 3, snap.FlowVersion)
	require.Equal(t, event.StatusEnded, snap.Status)
	require.Equal(t, event.EndCompleted, snap.EndReason)
	require.True(t, snap.ConsentCaptured)
	require.True(t, snap.ConsentGranted)
	require.True(t, snap.Recording)

	// Locale: detected EN, switched to FR; history keeps both.
	require.Equal(t, event.LocaleFRCA, snap.Locale)
	require.Equal(t, []event.Locale{event.LocaleENCA, event.LocaleFRCA}, snap.LocaleHistory)

	// Cursor and path follow node.entered.
	require.Equal(t, "book", snap.Cursor)
	require.Equal(t, []string{"greeting", "qualify", "book"}, snap.Path)

	// Vars hold the redacted values.
	require.JSONEq(t, `"plomberie"`, string(snap.Vars["service"]))

	// Transcript: caller/agent interleaved, redactions preserved,
	// interrupted turn marked.
	require.Len(t, snap.Transcript, 4)
	require.Equal(t, event.SpeakerCaller, snap.Transcript[0].Speaker)
	require.Equal(t, event.KindPostalCode, snap.Transcript[0].Redactions[0].Kind)
	require.Equal(t, event.SpeakerAgent, snap.Transcript[3].Speaker)
	require.True(t, snap.Transcript[3].Interrupted)

	// Turn latency metrics, in order.
	require.Len(t, snap.TurnLatencies, 2)
	require.Equal(t, int64(140), snap.TurnLatencies[0].VadEndToSTTFinalMs)
	require.Equal(t, int64(1300), snap.TurnLatencies[1].PerceivedMs)

	// Budget follows the degradation ladder events.
	require.Equal(t, "fast", snap.Budget.Stage)
	require.Equal(t, 1, snap.Budget.ErrorCount)

	require.Equal(t, uint64(20), snap.LastSeq)
}

func TestFoldUsage(t *testing.T) {
	snap, err := event.Fold(callFixture(t))
	require.NoError(t, err)

	// 125s → 3 billed minutes (round up).
	require.Equal(t, int64(3), snap.Usage.CallMinutes)
	// 1500ms + 2500ms of transcribed audio.
	require.InDelta(t, 4.0, snap.Usage.STTSeconds, 1e-9)
	require.Equal(t, int64(1300), snap.Usage.LLMTokensIn)
	require.Equal(t, int64(75), snap.Usage.LLMTokensOut)
	require.Equal(t, int64(63), snap.Usage.TTSChars)
}

func TestFoldDeterministicUnderShuffleAndDuplicates(t *testing.T) {
	base := callFixture(t)
	want, err := event.Fold(base)
	require.NoError(t, err)

	rng := rand.New(rand.NewSource(42))
	for i := 0; i < 20; i++ {
		// Duplicate a few events (at-least-once) and shuffle (no order).
		mixed := append([]event.Envelope{}, base...)
		mixed = append(mixed, base[rng.Intn(len(base))], base[rng.Intn(len(base))], base[0])
		rng.Shuffle(len(mixed), func(a, b int) { mixed[a], mixed[b] = mixed[b], mixed[a] })

		got, err := event.Fold(mixed)
		require.NoError(t, err)
		require.Equal(t, want, got, "iteration %d", i)
	}
}

func TestFoldDedupBySeq(t *testing.T) {
	base := callFixture(t)
	dup := append(append([]event.Envelope{}, base...), base[6], base[6], base[7])

	snap, err := event.Fold(dup)
	require.NoError(t, err)

	// Duplicated stt.final/agent.turn.completed must not double transcript
	// entries nor usage counters.
	require.Len(t, snap.Transcript, 4)
	require.InDelta(t, 4.0, snap.Usage.STTSeconds, 1e-9)
	require.Equal(t, int64(1300), snap.Usage.LLMTokensIn)
}

func TestFoldEmpty(t *testing.T) {
	snap, err := event.Fold(nil)
	require.NoError(t, err)
	require.Equal(t, event.StatusUnknown, snap.Status)
	require.Equal(t, "normal", snap.Budget.Stage)
	require.Zero(t, snap.LastSeq)
}
