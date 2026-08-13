// Package media holds the audio primitives of the telephony gateway: 20ms
// PCM frames, the energy meter that feeds barge-in level 1, and a small
// jitter buffer for the inbound media stream. Everything is provider-neutral:
// the Telnyx layer produces and consumes these types, nothing here knows the
// carrier (ADR-004 service boundary).
package media

import (
	"math"
	"time"
)

// Audio format constants for the PSTN leg (Telnyx media streams: 8kHz mono
// PCM16, 20ms frames).
const (
	// SampleRate is samples per second on the telephony leg.
	SampleRate = 8000
	// FrameDuration is the wall duration of one frame.
	FrameDuration = 20 * time.Millisecond
	// FrameMs is FrameDuration in milliseconds, the unit of the media clock.
	FrameMs = 20
	// SamplesPerFrame is the sample count of one 20ms frame.
	SamplesPerFrame = SampleRate / 1000 * FrameMs
)

// Frame is 20ms of mono PCM16 audio plus its position in the stream. Seq is
// assigned by the transport and used by the jitter buffer to reorder; the
// media clock of a call is Seq*20ms.
type Frame struct {
	// Seq is the 0-based frame sequence number within the call.
	Seq uint64
	// Samples is SamplesPerFrame signed 16-bit samples.
	Samples []int16
}

// Duration returns the frame's audio duration.
func (f Frame) Duration() time.Duration { return FrameDuration }

// EnergyDBFS returns the RMS energy of the frame in dBFS (0 dBFS = full
// scale). An all-zero (or empty) frame returns SilenceDBFS.
func (f Frame) EnergyDBFS() float64 {
	if len(f.Samples) == 0 {
		return SilenceDBFS
	}
	var sum float64
	for _, s := range f.Samples {
		v := float64(s)
		sum += v * v
	}
	rms := math.Sqrt(sum / float64(len(f.Samples)))
	if rms <= 0 {
		return SilenceDBFS
	}
	return 20 * math.Log10(rms/math.MaxInt16)
}

// SilenceDBFS is the floor value reported for digital silence.
const SilenceDBFS = -120.0

// Synth returns a frame of a 440Hz tone at the given level in dBFS. It is
// the synthetic-audio building block of the unit-test harness (nota 06): a
// speech-like frame is Synth(seq, -12), background noise Synth(seq, -30).
func Synth(seq uint64, dbfs float64) Frame {
	amp := math.Pow(10, dbfs/20) * math.MaxInt16
	samples := make([]int16, SamplesPerFrame)
	for i := range samples {
		t := float64(seq)*float64(SamplesPerFrame) + float64(i)
		samples[i] = int16(amp * math.Sin(2*math.Pi*440*t/SampleRate))
	}
	return Frame{Seq: seq, Samples: samples}
}

// Silence returns a frame of digital silence.
func Silence(seq uint64) Frame {
	return Frame{Seq: seq, Samples: make([]int16, SamplesPerFrame)}
}
