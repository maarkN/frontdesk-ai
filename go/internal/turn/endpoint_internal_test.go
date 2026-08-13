package turn

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/maarkn/frontdesk/internal/event"
)

func TestWaitForMsProfiles(t *testing.T) {
	d := DefaultEndpointDelays()
	cases := []struct {
		name string
		text string
		loc  event.Locale
		want int64
	}{
		{"en hesitation um", "I need um", event.LocaleENCA, d.HesitationMs},
		{"en hesitation uh", "so it's uh", event.LocaleENCA, d.HesitationMs},
		{"fr hesitation euh", "j'ai besoin euh", event.LocaleFRCA, d.HesitationMs},
		{"fr hesitation tsé", "c'est brisé tsé", event.LocaleFRCA, d.HesitationMs},
		{"en number words", "five five five", event.LocaleENCA, d.NumberMs},
		{"en digits", "514 555", event.LocaleENCA, d.NumberMs},
		{"fr numbers", "cinq cinq cinq", event.LocaleFRCA, d.NumberMs},
		{"en incomplete to", "I want to", event.LocaleENCA, d.IncompleteMs},
		{"en incomplete the", "it's about the", event.LocaleENCA, d.IncompleteMs},
		{"fr incomplete pour", "c'est pour", event.LocaleFRCA, d.IncompleteMs},
		{"en question mark", "can you come tomorrow?", event.LocaleENCA, d.QuestionMs},
		{"en question lead", "how much does it cost", event.LocaleENCA, d.QuestionMs},
		{"fr question lead", "combien ça coûte", event.LocaleFRCA, d.QuestionMs},
		{"en default", "my kitchen sink is leaking", event.LocaleENCA, d.DefaultMs},
		{"empty", "   ", event.LocaleENCA, d.DefaultMs},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, waitForMs(tc.text, tc.loc, d))
		})
	}
}

// TestENRulesNeverApplyToFR: "euh" is FR hesitation; in an EN call it gets
// no special treatment — and vice-versa with "um".
func TestENRulesNeverApplyToFR(t *testing.T) {
	d := DefaultEndpointDelays()
	require.Equal(t, d.DefaultMs, waitForMs("I said euh", event.LocaleENCA, d),
		"FR dictionary must not leak into EN")
	require.Equal(t, d.DefaultMs, waitForMs("j'ai dit um", event.LocaleFRCA, d),
		"EN dictionary must not leak into FR")
}

func TestIsBackchannel(t *testing.T) {
	for _, tc := range []struct {
		text string
		loc  event.Locale
		want bool
	}{
		{"mhm", event.LocaleENCA, true},
		{"uh huh", event.LocaleENCA, true},
		{"yeah ok", event.LocaleENCA, true},
		{"Yeah, okay!", event.LocaleENCA, true},
		{"ouais", event.LocaleFRCA, true},
		{"han han", event.LocaleFRCA, true},
		{"c'est ça", event.LocaleFRCA, true},
		{"wait actually stop", event.LocaleENCA, false},
		{"yeah but wait", event.LocaleENCA, false},
		{"non attends une minute", event.LocaleFRCA, false},
		{"", event.LocaleENCA, false},
	} {
		require.Equal(t, tc.want, isBackchannel(tc.text, tc.loc), "%q (%s)", tc.text, tc.loc)
	}
}

func TestPlayoutTrackerSpoken(t *testing.T) {
	tr := NewPlayoutTracker(100) // 100ms written→heard offset
	tr.AddChunk(Chunk{
		DurationMs: 900,
		Words: []WordTiming{
			{Word: "one", StartMs: 0, EndMs: 300},
			{Word: "two", StartMs: 300, EndMs: 600},
			{Word: "three", StartMs: 600, EndMs: 900},
		},
	})
	// Second chunk: word timings are chunk-relative and get rebased.
	tr.AddChunk(Chunk{
		DurationMs: 300,
		Words:      []WordTiming{{Word: "four", StartMs: 0, EndMs: 300}},
	})
	require.Equal(t, int64(1200), tr.WrittenMs())
	require.Equal(t, "one two three four", tr.FullText())

	// 750ms played − 100ms offset = 650ms heard: only fully played words.
	text, cut := tr.Spoken(750)
	require.True(t, cut)
	require.Equal(t, "one two —", text)

	// Everything played (plus offset) → no cut marker.
	text, cut = tr.Spoken(1300)
	require.False(t, cut)
	require.Equal(t, "one two three four", text)

	// Cut before anything was heard → history records just the marker.
	text, cut = tr.Spoken(50)
	require.True(t, cut)
	require.Equal(t, "—", text)
}
