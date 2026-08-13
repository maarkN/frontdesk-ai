package event_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/maarkn/frontdesk/internal/event"
)

func kinds(spans []event.Redaction) []event.RedactionKind {
	out := make([]event.RedactionKind, len(spans))
	for i, s := range spans {
		out[i] = s.Kind
	}
	return out
}

func TestDetectCardDigits(t *testing.T) {
	for _, text := range []string{
		"my card number is 4111 1111 1111 1111 thanks",
		"card: 4111-1111-1111-1111",
		"it's 4242424242424242 ok",
	} {
		spans := event.Detect(text)
		require.Equal(t, []event.RedactionKind{event.KindCard}, kinds(spans), "text=%q", text)
	}
}

func TestDetectCardSpokenEnglish(t *testing.T) {
	// STT often normalizes dictated digits into words; detectors must not
	// depend on digit glyphs (ADR-003 decision 5).
	text := "the number is four one one one one one one one one one one one one one one one"
	spans := event.Detect(text)
	require.Equal(t, []event.RedactionKind{event.KindCard}, kinds(spans))
	require.Equal(t,
		"four one one one one one one one one one one one one one one one",
		text[spans[0].Start:spans[0].End])
}

func TestDetectCardMixedSpokenAndDigits(t *testing.T) {
	text := "so it's 4111 1111 1111 one one one one on the visa"
	spans := event.Detect(text)
	require.Equal(t, []event.RedactionKind{event.KindCard}, kinds(spans))
}

func TestDetectSINDigits(t *testing.T) {
	spans := event.Detect("my SIN is 046 454 286 eh")
	require.Equal(t, []event.RedactionKind{event.KindSIN}, kinds(spans))
}

func TestDetectSINSpokenFrench(t *testing.T) {
	text := "mon NAS c'est zéro quatre six quatre cinq quatre deux huit six merci"
	spans := event.Detect(text)
	require.Equal(t, []event.RedactionKind{event.KindSIN}, kinds(spans))
	require.Equal(t,
		"zéro quatre six quatre cinq quatre deux huit six",
		text[spans[0].Start:spans[0].End])
}

func TestDetectPostalCode(t *testing.T) {
	for _, text := range []string{
		"I'm at H2X 1Y4 downtown",
		"code postal h2x1y4",
		"H2X-1Y4",
	} {
		spans := event.Detect(text)
		require.Equal(t, []event.RedactionKind{event.KindPostalCode}, kinds(spans), "text=%q", text)
	}
}

func TestDetectDOB(t *testing.T) {
	for _, text := range []string{
		"my date of birth is 1990-01-05 ok",
		"I was born on January 5, 1990",
		"born 5th of January 1990",
		"je suis née le 5 janvier 1990",
		"date de naissance 05/01/1990",
	} {
		spans := event.Detect(text)
		require.Equal(t, []event.RedactionKind{event.KindDOB}, kinds(spans), "text=%q", text)
	}
}

func TestDetectNegatives(t *testing.T) {
	for _, text := range []string{
		// Luhn-invalid runs are not cards nor SINs.
		"call me at 1234 5678 9012 3456",
		"account 123 456 789",
		// Appointment dates without birth context stay untouched.
		"book me for January 5, 2027 please",
		"demain 9 heures c'est parfait",
		// French articles that map to digits must not fire alone.
		"j'ai un rendez-vous avec une amie",
		"no pii here at all",
	} {
		require.Empty(t, event.Detect(text), "text=%q", text)
	}
}

func TestDetectMultipleSpansSorted(t *testing.T) {
	text := "SIN 046 454 286, card 4111 1111 1111 1111, postal H2X 1Y4"
	spans := event.Detect(text)
	require.Equal(t,
		[]event.RedactionKind{event.KindSIN, event.KindCard, event.KindPostalCode},
		kinds(spans))
	for i := 1; i < len(spans); i++ {
		require.GreaterOrEqual(t, spans[i].Start, spans[i-1].End, "spans must not overlap")
	}
}

type fakeVault struct {
	n     int
	store map[string]string
}

func (v *fakeVault) Store(kind event.RedactionKind, original string) (string, error) {
	v.n++
	ref := fmt.Sprintf("vault-%d", v.n)
	if v.store == nil {
		v.store = make(map[string]string)
	}
	v.store[ref] = original
	return ref, nil
}

func TestRedactReplacesAndVaults(t *testing.T) {
	vault := &fakeVault{}
	r := event.NewRedactor(vault)

	text := "my card is 4111 1111 1111 1111 and postal H2X 1Y4"
	redacted, spans, err := r.Redact(text)
	require.NoError(t, err)

	require.Equal(t, "my card is [redacted:card] and postal [redacted:postal_code]", redacted)
	require.Len(t, spans, 2)
	require.Equal(t, "4111 1111 1111 1111", vault.store[spans[0].Vault])
	require.Equal(t, "H2X 1Y4", vault.store[spans[1].Vault])
	// Offsets refer to the ORIGINAL text.
	require.Equal(t, "4111 1111 1111 1111", text[spans[0].Start:spans[0].End])
}

func TestRedactZeroValueNoVault(t *testing.T) {
	var r event.Redactor
	redacted, spans, err := r.Redact("SIN 046 454 286")
	require.NoError(t, err)
	require.Equal(t, "SIN [redacted:sin]", redacted)
	require.Len(t, spans, 1)
	require.Empty(t, spans[0].Vault)
}

func TestRedactCleanTextUntouched(t *testing.T) {
	var r event.Redactor
	redacted, spans, err := r.Redact("I have a leak under the sink")
	require.NoError(t, err)
	require.Equal(t, "I have a leak under the sink", redacted)
	require.Empty(t, spans)
}
