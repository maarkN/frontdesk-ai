package turn

import (
	"strings"
	"unicode"

	"github.com/maarkn/frontdesk/internal/event"
)

// langProfile holds the per-language dictionaries of the adaptive endpointer
// and of backchannel detection. EN rules are NEVER applied to FR and
// vice-versa (EPIC-002): each locale selects exactly one profile.
type langProfile struct {
	hesitation  map[string]struct{}
	backchannel map[string]struct{}
	incomplete  map[string]struct{}
	numbers     map[string]struct{}
	questionLed map[string]struct{}
}

func set(words ...string) map[string]struct{} {
	m := make(map[string]struct{}, len(words))
	for _, w := range words {
		m[w] = struct{}{}
	}
	return m
}

// profileEN covers en-CA.
var profileEN = langProfile{
	hesitation: set("um", "uh", "er", "erm", "hmm", "umm", "uhh", "hm"),
	backchannel: set(
		"uh", "huh", "uh-huh", "uhhuh", "mhm", "mm", "hmm", "mm-hmm", "mmhm",
		"yeah", "yep", "yes", "ya", "ok", "okay", "right", "sure", "gotcha",
		"cool", "alright",
	),
	incomplete: set(
		"and", "but", "or", "so", "to", "the", "a", "an", "of", "with", "for",
		"at", "in", "on", "my", "your", "his", "her", "our", "their", "is",
		"are", "was", "were", "i", "it's", "its", "that", "this", "about",
		"because", "if", "when", "like",
	),
	numbers: set(
		"zero", "one", "two", "three", "four", "five", "six", "seven",
		"eight", "nine", "ten", "oh", "double", "triple", "hundred",
	),
	questionLed: set(
		"what", "when", "where", "who", "why", "how", "which", "can",
		"could", "would", "will", "do", "does", "did", "is", "are", "may",
		"should",
	),
}

// profileFR covers fr-CA (Québécois markers included: tsé, ben, han han).
var profileFR = langProfile{
	hesitation: set("euh", "euhh", "ben", "bah", "tsé", "tse", "genre", "là", "la", "heu"),
	backchannel: set(
		"ouais", "oui", "ok", "d'accord", "daccord", "han", "hum", "hem",
		"mhm", "mm", "c'est", "ça", "ca", "bon", "parfait", "exact",
	),
	incomplete: set(
		"et", "mais", "ou", "donc", "de", "du", "des", "le", "les", "un",
		"une", "à", "a", "au", "aux", "avec", "pour", "dans", "sur", "mon",
		"ma", "mes", "ton", "ta", "votre", "vos", "je", "j'ai", "c'est",
		"est", "que", "qui", "parce", "si", "comme", "puis", "pis",
	),
	numbers: set(
		"zéro", "zero", "un", "deux", "trois", "quatre", "cinq", "six",
		"sept", "huit", "neuf", "dix", "cent",
	),
	questionLed: set(
		"quoi", "quand", "où", "qui", "pourquoi", "comment", "combien",
		"est-ce", "peux", "peut", "pouvez", "es-tu", "est-tu", "c'est-tu",
		"as-tu", "avez-vous",
	),
}

// profileFor selects the dictionary set of a locale; unknown locales get the
// EN profile (the least surprising default for en-CA-first deployments).
func profileFor(loc event.Locale) langProfile {
	if loc == event.LocaleFRCA {
		return profileFR
	}
	return profileEN
}

// tokenize lowercases and splits, stripping outer punctuation but keeping
// in-word apostrophes and hyphens ("c'est", "uh-huh").
func tokenize(text string) []string {
	fields := strings.Fields(strings.ToLower(text))
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		tok := strings.TrimFunc(f, func(r rune) bool {
			return unicode.IsPunct(r) && r != '\'' && r != '-'
		})
		if tok != "" {
			out = append(out, tok)
		}
	}
	return out
}

// isNumericToken reports whether tok is a digit group or a number word.
func isNumericToken(tok string, p langProfile) bool {
	if _, ok := p.numbers[tok]; ok {
		return true
	}
	for _, r := range tok {
		if !unicode.IsDigit(r) && r != '-' {
			return false
		}
	}
	return tok != ""
}

// waitForMs classifies the current draft and returns how long the endpointer
// waits (media-clock ms of silence) before closing the user turn.
//
// Precedence (nota 06): trailing hesitation ("um…" the caller is thinking)
// beats everything; number dictation beats incomplete phrase; questions turn
// over fastest.
func waitForMs(text string, loc event.Locale, d EndpointDelays) int64 {
	p := profileFor(loc)
	toks := tokenize(text)
	if len(toks) == 0 {
		return d.DefaultMs
	}
	last := toks[len(toks)-1]

	if _, ok := p.hesitation[last]; ok {
		return d.HesitationMs
	}
	if isNumericToken(last, p) {
		return d.NumberMs
	}
	if _, ok := p.incomplete[last]; ok {
		return d.IncompleteMs
	}
	if strings.HasSuffix(strings.TrimSpace(text), "?") {
		return d.QuestionMs
	}
	if _, ok := p.questionLed[toks[0]]; ok {
		return d.QuestionMs
	}
	return d.DefaultMs
}

// isBackchannel reports whether the utterance is a pure backchannel
// ("uh huh", "ouais", "han han"): at most three tokens, all in the locale's
// backchannel dictionary. Backchannels undo ducking and never cut the bot.
func isBackchannel(text string, loc event.Locale) bool {
	p := profileFor(loc)
	toks := tokenize(text)
	if len(toks) == 0 || len(toks) > 3 {
		return false
	}
	for _, tok := range toks {
		if _, ok := p.backchannel[tok]; !ok {
			return false
		}
	}
	return true
}

// realTextLen counts non-space runes — the measure compared against
// MinBargeChars for barge-in confirmation.
func realTextLen(text string) int {
	n := 0
	for _, r := range text {
		if !unicode.IsSpace(r) {
			n++
		}
	}
	return n
}
