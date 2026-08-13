package event

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Vault stores the original content of a redacted span, encrypted under the
// call DEK (crypto-shredding: deleting the DEK destroys every stored span).
// Implementations live near the storage layer; the zero-value Redactor works
// without one and emits empty vault references.
type Vault interface {
	// Store persists the original span and returns an opaque reference.
	Store(kind RedactionKind, original string) (string, error)
}

// Redactor detects and redacts PII at emission time, before any text enters
// an event payload (ADR-003, decision 5). Detectors cover credit cards (Luhn),
// Canadian SIN, Canadian postal codes, and dates of birth, in both English and
// French — including digits the STT normalized from spoken number words
// ("four one one …", "quatre cinq un …").
//
// The zero value is a working Redactor with no vault: spans are still
// redacted, with empty vault references.
type Redactor struct {
	vault Vault
}

// NewRedactor returns a Redactor storing originals in vault. A nil vault is
// valid and yields empty vault references.
func NewRedactor(vault Vault) *Redactor {
	return &Redactor{vault: vault}
}

// Mask is the replacement text for a redacted span, by kind.
func Mask(kind RedactionKind) string {
	return "[redacted:" + string(kind) + "]"
}

// Redact replaces every detected PII span in text and returns the redacted
// text plus the spans (offsets refer to the ORIGINAL text). Originals are
// stored in the vault when one is configured.
func (r *Redactor) Redact(text string) (string, []Redaction, error) {
	spans := Detect(text)
	if len(spans) == 0 {
		return text, nil, nil
	}

	var b strings.Builder
	prev := 0
	for i := range spans {
		s := &spans[i]
		if r != nil && r.vault != nil {
			ref, err := r.vault.Store(s.Kind, text[s.Start:s.End])
			if err != nil {
				return "", nil, fmt.Errorf("vault store %s span: %w", s.Kind, err)
			}
			s.Vault = ref
		}
		b.WriteString(text[prev:s.Start])
		b.WriteString(Mask(s.Kind))
		prev = s.End
	}
	b.WriteString(text[prev:])
	return b.String(), spans, nil
}

// Detect finds PII spans in text without modifying it. Spans are sorted by
// start offset and never overlap (first match wins on conflict).
func Detect(text string) []Redaction {
	var spans []Redaction
	spans = append(spans, detectDigitRuns(text)...)
	spans = append(spans, detectPostalCodes(text)...)
	spans = append(spans, detectDOB(text)...)

	sort.Slice(spans, func(i, j int) bool {
		if spans[i].Start != spans[j].Start {
			return spans[i].Start < spans[j].Start
		}
		return spans[i].End > spans[j].End
	})
	// Drop overlaps deterministically: keep the earliest (longest on tie).
	out := spans[:0]
	lastEnd := -1
	for _, s := range spans {
		if s.Start < lastEnd {
			continue
		}
		out = append(out, s)
		lastEnd = s.End
	}
	return out
}

// numberWords maps spoken number words (EN and FR-CA, as normalized by STT)
// to a single digit. Both accented and accent-stripped French spellings are
// included because STT output varies.
var numberWords = map[string]byte{
	// English.
	"zero": '0', "oh": '0', "one": '1', "two": '2', "three": '3',
	"four": '4', "five": '5', "six": '6', "seven": '7', "eight": '8',
	"nine": '9',
	// French (Canadian). "six" overlaps with English and maps identically.
	"zéro": '0', "un": '1', "une": '1', "deux": '2', "trois": '3',
	"quatre": '4', "cinq": '5', "sept": '7', "huit": '8', "neuf": '9',
}

// token is a maximal run of letters or digits with its byte offsets.
type token struct {
	start, end int
	text       string
}

func tokenize(text string) []token {
	var toks []token
	start := -1
	for i, r := range text {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			if start < 0 {
				start = i
			}
			continue
		}
		if start >= 0 {
			toks = append(toks, token{start: start, end: i, text: text[start:i]})
			start = -1
		}
	}
	if start >= 0 {
		toks = append(toks, token{start: start, end: len(text), text: text[start:]})
	}
	return toks
}

// digitsOf returns the digit string a token contributes to a digit run:
// its own digits for numeric tokens, a single digit for number words, and
// "" for anything else.
func digitsOf(tok string) string {
	lower := strings.ToLower(tok)
	if d, ok := numberWords[lower]; ok {
		return string(d)
	}
	for _, r := range lower {
		if r < '0' || r > '9' {
			return ""
		}
	}
	return lower
}

// gapIsSeparator reports whether the text between two tokens is only spacing
// or common digit-group separators, so both tokens belong to one dictated run.
func gapIsSeparator(gap string) bool {
	if utf8.RuneCountInString(gap) > 3 {
		return false
	}
	for _, r := range gap {
		switch {
		case unicode.IsSpace(r):
		case r == '-' || r == '.' || r == ',':
		default:
			return false
		}
	}
	return true
}

// detectDigitRuns finds credit cards (13–19 digits, Luhn-valid) and Canadian
// SINs (9 digits, Luhn-valid) in contiguous dictated digit runs, including
// number words in EN and FR.
func detectDigitRuns(text string) []Redaction {
	toks := tokenize(text)
	var spans []Redaction

	i := 0
	for i < len(toks) {
		if digitsOf(toks[i].text) == "" {
			i++
			continue
		}
		// Extend the run while tokens contribute digits and gaps are
		// plain separators.
		digits := digitsOf(toks[i].text)
		start, end := toks[i].start, toks[i].end
		j := i + 1
		for j < len(toks) {
			d := digitsOf(toks[j].text)
			if d == "" || !gapIsSeparator(text[toks[j-1].end:toks[j].start]) {
				break
			}
			digits += d
			end = toks[j].end
			j++
		}

		switch {
		case len(digits) >= 13 && len(digits) <= 19 && luhnValid(digits):
			spans = append(spans, Redaction{Start: start, End: end, Kind: KindCard})
		case len(digits) == 9 && luhnValid(digits):
			// SIN uses the Luhn checksum as well; 9 Luhn-valid digits
			// dictated together are treated as a SIN.
			spans = append(spans, Redaction{Start: start, End: end, Kind: KindSIN})
		}
		i = j
	}
	return spans
}

// luhnValid reports whether digits (ASCII '0'–'9') pass the Luhn checksum.
func luhnValid(digits string) bool {
	if digits == "" {
		return false
	}
	sum := 0
	double := false
	for i := len(digits) - 1; i >= 0; i-- {
		d := int(digits[i] - '0')
		if double {
			d *= 2
			if d > 9 {
				d -= 9
			}
		}
		sum += d
		double = !double
	}
	return sum%10 == 0
}

// Canadian postal code: letter-digit-letter [space] digit-letter-digit, with
// the first letter restricted to the valid forward sortation area alphabet.
var postalRE = regexp.MustCompile(`(?i)\b[ABCEGHJ-NPRSTVXY][0-9][A-Z][ \-]?[0-9][A-Z][0-9]\b`)

func detectPostalCodes(text string) []Redaction {
	var spans []Redaction
	for _, loc := range postalRE.FindAllStringIndex(text, -1) {
		spans = append(spans, Redaction{Start: loc[0], End: loc[1], Kind: KindPostalCode})
	}
	return spans
}

// Date-of-birth detection: a date expression preceded by a birth-context
// keyword (EN or FR) within a short window. Gating on context keeps
// appointment dates — the bread and butter of a scheduling agent — out of the
// redactor.
var (
	dobContextRE = regexp.MustCompile(`(?i)(date of birth|birth\s?date|born|dob|date de naissance|naissance|née?\b)`)

	dateRE = regexp.MustCompile(`(?i)\b(` +
		// ISO 8601: 1990-01-05.
		`\d{4}-\d{2}-\d{2}` +
		// Numeric: 05/01/1990, 5/1/90.
		`|\d{1,2}/\d{1,2}/\d{2,4}` +
		// English month names: January 5, 1990 / 5 January 1990.
		`|(january|february|march|april|may|june|july|august|september|october|november|december)\s+\d{1,2}(st|nd|rd|th)?,?\s+\d{4}` +
		`|\d{1,2}(st|nd|rd|th)?\s+(of\s+)?(january|february|march|april|may|june|july|august|september|october|november|december),?\s+\d{4}` +
		// French month names: 5 janvier 1990 / 1er décembre 1985.
		`|\d{1,2}(er)?\s+(janvier|février|fevrier|mars|avril|mai|juin|juillet|août|aout|septembre|octobre|novembre|décembre|decembre)\s+\d{4}` +
		`)\b`)
)

// dobContextWindow is how far (in bytes) before a date a birth keyword may
// appear for the date to count as a date of birth.
const dobContextWindow = 48

func detectDOB(text string) []Redaction {
	var spans []Redaction
	for _, loc := range dateRE.FindAllStringIndex(text, -1) {
		winStart := loc[0] - dobContextWindow
		if winStart < 0 {
			winStart = 0
		}
		if !dobContextRE.MatchString(text[winStart:loc[0]]) {
			continue
		}
		spans = append(spans, Redaction{Start: loc[0], End: loc[1], Kind: KindDOB})
	}
	return spans
}
