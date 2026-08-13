package turn

import (
	"time"

	"github.com/maarkn/frontdesk/internal/audiobank"
	"github.com/maarkn/frontdesk/internal/event"
	"github.com/maarkn/frontdesk/internal/media"
)

// EndpointDelays are the adaptive endpointing waits, in media-clock
// milliseconds (nota 06 / CONTEXT.md §3). They are persona configuration,
// not code.
type EndpointDelays struct {
	// HesitationMs applies when the draft ends in a hesitation token
	// ("um"/"uh" EN, "euh"/"ben"/"tsé" FR-CA): the caller is thinking.
	HesitationMs int64
	// NumberMs applies while the caller dictates numbers (phone, postal
	// code): digit groups come with long pauses.
	NumberMs int64
	// IncompleteMs applies when the draft ends in a word that never ends a
	// sentence (preposition, article, conjunction).
	IncompleteMs int64
	// QuestionMs applies to questions: turn over fast, the caller expects
	// an answer.
	QuestionMs int64
	// DefaultMs is everything else.
	DefaultMs int64
}

// DefaultEndpointDelays returns the CONTEXT.md reference profile
// (~1400/1100/900/400/600ms).
func DefaultEndpointDelays() EndpointDelays {
	return EndpointDelays{
		HesitationMs: 1400,
		NumberMs:     1100,
		IncompleteMs: 900,
		QuestionMs:   400,
		DefaultMs:    600,
	}
}

// Config is the per-persona timing/behavior configuration of the machine.
type Config struct {
	// Locale is the initial conversation locale (language detection may
	// switch it mid-call without restarting the machine).
	Locale event.Locale
	// Endpoint holds the adaptive endpointing waits.
	Endpoint EndpointDelays
	// MinBargeChars is the minimum non-space length of an STT partial that
	// CONFIRMS a barge-in (level 2). Shorter partials keep the ducking.
	MinBargeChars int
	// ConfirmWindowMs is how long ducking waits for a confirming partial
	// before undoing itself (energy blip that STT never corroborated).
	ConfirmWindowMs int64
	// FillerAfter is how long stThinking may stay silent before the filler
	// clip plays (~600ms).
	FillerAfter time.Duration
	// FillerKey is the audio-bank phrase used as filler.
	FillerKey audiobank.Key
	// PlayoutOffsetMs compensates writtenMs vs heard: audio written to the
	// carrier is heard 100–200ms later. Calibrated per deployment.
	PlayoutOffsetMs int64
	// Meter configures the barge-in energy detector.
	Meter []media.MeterOption
}

// Option mutates the Config (functional options, uber-go style).
type Option func(*Config)

// defaultConfig returns the reference persona timing.
func defaultConfig() Config {
	return Config{
		Locale:          event.LocaleENCA,
		Endpoint:        DefaultEndpointDelays(),
		MinBargeChars:   6,
		ConfirmWindowMs: 700,
		FillerAfter:     600 * time.Millisecond,
		FillerKey:       audiobank.KeyOneMoment,
		PlayoutOffsetMs: 150,
	}
}

// WithLocale sets the initial locale.
func WithLocale(loc event.Locale) Option {
	return func(c *Config) { c.Locale = loc }
}

// WithEndpointDelays replaces the endpointing profile.
func WithEndpointDelays(d EndpointDelays) Option {
	return func(c *Config) { c.Endpoint = d }
}

// WithMinBargeChars sets the barge-in confirmation threshold.
func WithMinBargeChars(n int) Option {
	return func(c *Config) { c.MinBargeChars = n }
}

// WithConfirmWindowMs sets the ducking confirmation window.
func WithConfirmWindowMs(ms int64) Option {
	return func(c *Config) { c.ConfirmWindowMs = ms }
}

// WithFillerAfter sets the thinking-silence budget before the filler plays.
func WithFillerAfter(d time.Duration) Option {
	return func(c *Config) { c.FillerAfter = d }
}

// WithFillerKey sets the audio-bank phrase used as filler.
func WithFillerKey(k audiobank.Key) Option {
	return func(c *Config) { c.FillerKey = k }
}

// WithPlayoutOffsetMs sets the written→heard calibration offset.
func WithPlayoutOffsetMs(ms int64) Option {
	return func(c *Config) { c.PlayoutOffsetMs = ms }
}

// WithMeterOptions configures the energy meter (threshold, onset frames).
func WithMeterOptions(opts ...media.MeterOption) Option {
	return func(c *Config) { c.Meter = opts }
}
