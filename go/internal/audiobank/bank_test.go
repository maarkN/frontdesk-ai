package audiobank_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/maarkn/frontdesk/internal/audiobank"
	"github.com/maarkn/frontdesk/internal/event"
)

// full returns a bank covering every required key for both locales.
func full(t *testing.T, persona, version string) *audiobank.Bank {
	t.Helper()
	b := audiobank.New(persona, version)
	for _, loc := range []event.Locale{event.LocaleENCA, event.LocaleFRCA} {
		for _, key := range audiobank.Required() {
			b.Add(loc, key, audiobank.Clip{
				Text:       string(key) + "/" + string(loc),
				Audio:      []byte{1, 2, 3},
				DurationMs: 800,
			})
		}
	}
	return b
}

func TestLookup(t *testing.T) {
	b := full(t, "dispatch-emma", "v3")

	c, err := b.Lookup(audiobank.KeyOneMoment, event.LocaleFRCA)
	require.NoError(t, err)
	require.Equal(t, "one_moment/fr-CA", c.Text)
	require.Equal(t, 800, c.DurationMs)

	_, err = b.Lookup("nope", event.LocaleENCA)
	require.ErrorIs(t, err, audiobank.ErrNotFound)
}

func TestValidateCompleteBank(t *testing.T) {
	b := full(t, "dispatch-emma", "v3")
	require.NoError(t, b.Validate(event.LocaleENCA, event.LocaleFRCA))
}

func TestValidateMissingClipIsError(t *testing.T) {
	// A locale missing even one required phrase must fail the build.
	b := audiobank.New("dispatch-emma", "v3")
	for _, key := range audiobank.Required() {
		b.Add(event.LocaleENCA, key, audiobank.Clip{Text: string(key), Audio: []byte{1}})
	}
	b.Add(event.LocaleFRCA, audiobank.KeyGreeting, audiobank.Clip{Text: "bonjour", Audio: []byte{1}})

	err := b.Validate(event.LocaleENCA, event.LocaleFRCA)
	require.Error(t, err)
	require.Contains(t, err.Error(), "one_moment/fr-CA")
	require.Contains(t, err.Error(), "dispatch-emma@v3")
}

func TestVersionTracksPersona(t *testing.T) {
	b := full(t, "dispatch-emma", "v4")
	require.Equal(t, "dispatch-emma", b.Persona())
	require.Equal(t, "v4", b.Version())
}
