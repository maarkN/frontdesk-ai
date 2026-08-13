// Package audiobank is the pre-synthesized phrase bank: ~15 canned phrases
// per locale, generated at build time with the persona's voice and versioned
// together with the persona (ADR-006 §4). It is critical infrastructure — a
// missing phrase for an enabled locale is a build error, not a warning —
// because it is the 0ms answer when TTS is down and the voice of degScripted,
// degTransfer and the turn-loop filler.
package audiobank

import (
	"errors"
	"fmt"
	"sort"
	"sync"

	"github.com/maarkn/frontdesk/internal/event"
)

// Key names one canned phrase.
type Key string

// Catalog of required phrases. Every enabled locale must provide all of them
// (Validate enforces it).
const (
	KeyGreeting        Key = "greeting"
	KeyRecordingNotice Key = "recording_notice"
	KeyOneMoment       Key = "one_moment"
	KeyStillThere      Key = "still_there"
	KeyDidntCatch      Key = "didnt_catch"
	KeySayAgain        Key = "say_again"
	KeyTransferring    Key = "transferring"
	KeyTransferFailed  Key = "transfer_failed"
	KeyTechnicalIssue  Key = "technical_issue"
	KeyEmergencyAck    Key = "emergency_ack"
	KeyVoicemailIntro  Key = "voicemail_intro"
	KeyVoicemailThanks Key = "voicemail_thanks"
	KeyDTMFMenu        Key = "dtmf_menu"
	KeyDTMFInvalid     Key = "dtmf_invalid"
	KeyGoodbye         Key = "goodbye"
)

// Required lists every key a locale must cover for the bank to be valid.
func Required() []Key {
	return []Key{
		KeyGreeting, KeyRecordingNotice, KeyOneMoment, KeyStillThere,
		KeyDidntCatch, KeySayAgain, KeyTransferring, KeyTransferFailed,
		KeyTechnicalIssue, KeyEmergencyAck, KeyVoicemailIntro,
		KeyVoicemailThanks, KeyDTMFMenu, KeyDTMFInvalid, KeyGoodbye,
	}
}

// Clip is one pre-synthesized phrase: the text that was synthesized (goes to
// the conversation history when played) and the PCM audio served from local
// disk.
type Clip struct {
	Text       string
	Audio      []byte
	DurationMs int
}

// ErrNotFound is returned by Lookup when the (key, locale) pair is absent.
var ErrNotFound = errors.New("audiobank: clip not found")

// Bank holds the clips of one persona version. Safe for concurrent lookup
// after construction.
type Bank struct {
	persona string
	version string

	mu    sync.RWMutex
	clips map[event.Locale]map[Key]Clip
}

// New returns an empty bank for a persona. Version identifies the persona
// revision the audio was synthesized with: change the persona's voice or
// wording and the build regenerates the bank under a new version — a bank
// whose version does not match the deployed persona must be rejected at boot.
func New(persona, version string) *Bank {
	return &Bank{
		persona: persona,
		version: version,
		clips:   make(map[event.Locale]map[Key]Clip),
	}
}

// Persona returns the persona the bank was synthesized for.
func (b *Bank) Persona() string { return b.persona }

// Version returns the persona revision of the audio.
func (b *Bank) Version() string { return b.version }

// Add registers a clip for (locale, key), replacing any previous one.
func (b *Bank) Add(loc event.Locale, key Key, c Clip) {
	b.mu.Lock()
	defer b.mu.Unlock()
	byKey, ok := b.clips[loc]
	if !ok {
		byKey = make(map[Key]Clip)
		b.clips[loc] = byKey
	}
	byKey[key] = c
}

// Lookup returns the clip for (key, locale).
func (b *Bank) Lookup(key Key, loc event.Locale) (Clip, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	c, ok := b.clips[loc][key]
	if !ok {
		return Clip{}, fmt.Errorf("%w: %s/%s (persona %s@%s)", ErrNotFound, key, loc, b.persona, b.version)
	}
	return c, nil
}

// Validate fails if any required key is missing for any of the given locales.
// It is meant to run at build/boot time: an incomplete bank is an error, the
// process must not take calls with it.
func (b *Bank) Validate(locales ...event.Locale) error {
	b.mu.RLock()
	defer b.mu.RUnlock()
	var missing []string
	for _, loc := range locales {
		for _, key := range Required() {
			if _, ok := b.clips[loc][key]; !ok {
				missing = append(missing, fmt.Sprintf("%s/%s", key, loc))
			}
		}
	}
	if len(missing) == 0 {
		return nil
	}
	sort.Strings(missing)
	return fmt.Errorf("audiobank: persona %s@%s missing required clips: %v", b.persona, b.version, missing)
}
