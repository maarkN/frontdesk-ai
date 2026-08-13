package media

// Meter is the barge-in level-1 detector: it declares speech onset only after
// onsetFrames consecutive frames above thresholdDBFS, and keeps the "speaking"
// state through short dips (hangoverFrames) so natural intra-word gaps do not
// flicker the detector. It is intentionally dumb — confirming that the energy
// is actual speech (and not a cough or line noise) is level 2's job, done with
// STT partials in the turn machine.
//
// The zero value is not ready; use NewMeter.
type Meter struct {
	thresholdDBFS float64
	onsetFrames   int
	hangover      int

	above    int
	quiet    int
	speaking bool
}

// MeterOption configures a Meter.
type MeterOption func(*Meter)

// WithThresholdDBFS sets the RMS level above which a frame counts as voice
// candidate. Default -25 dBFS: a -30 dBFS noise floor never trips it.
func WithThresholdDBFS(db float64) MeterOption {
	return func(m *Meter) { m.thresholdDBFS = db }
}

// WithOnsetFrames sets how many consecutive loud frames declare onset.
// Default 3 (60ms) — inside the ~100ms ducking budget.
func WithOnsetFrames(n int) MeterOption {
	return func(m *Meter) { m.onsetFrames = n }
}

// WithHangoverFrames sets how many quiet frames end a speech run. Default 10
// (200ms).
func WithHangoverFrames(n int) MeterOption {
	return func(m *Meter) { m.hangover = n }
}

// NewMeter returns a Meter with the given options applied over defaults.
func NewMeter(opts ...MeterOption) *Meter {
	m := &Meter{thresholdDBFS: -25, onsetFrames: 3, hangover: 10}
	for _, o := range opts {
		o(m)
	}
	return m
}

// Process consumes one frame and reports whether the caller is speaking after
// it. The transition false→true is the speech onset used to duck playback.
func (m *Meter) Process(f Frame) bool {
	if f.EnergyDBFS() >= m.thresholdDBFS {
		m.above++
		m.quiet = 0
		if m.above >= m.onsetFrames {
			m.speaking = true
		}
		return m.speaking
	}
	m.above = 0
	if m.speaking {
		m.quiet++
		if m.quiet >= m.hangover {
			m.speaking = false
			m.quiet = 0
		}
	}
	return m.speaking
}

// Speaking reports the current state without consuming a frame.
func (m *Meter) Speaking() bool { return m.speaking }
