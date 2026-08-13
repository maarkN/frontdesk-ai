package media_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/maarkn/frontdesk/internal/media"
)

func TestFrameEnergy(t *testing.T) {
	require.Equal(t, media.SilenceDBFS, media.Silence(0).EnergyDBFS())

	// A -12 dBFS sine has RMS ~= -15 dBFS (peak-to-RMS of a sine is 3dB).
	got := media.Synth(0, -12).EnergyDBFS()
	require.InDelta(t, -15, got, 1.0)
}

func TestMeterOnsetAndHangover(t *testing.T) {
	m := media.NewMeter(media.WithOnsetFrames(3), media.WithHangoverFrames(2))

	var seq uint64
	speech := func() bool { seq++; return m.Process(media.Synth(seq, -12)) }
	quiet := func() bool { seq++; return m.Process(media.Silence(seq)) }

	require.False(t, speech(), "1 loud frame is not onset")
	require.False(t, speech(), "2 loud frames are not onset")
	require.True(t, speech(), "3rd consecutive loud frame declares onset")

	require.True(t, quiet(), "hangover keeps speaking through 1 quiet frame")
	require.False(t, quiet(), "2nd quiet frame ends the run")
}

func TestMeterIgnoresLowNoise(t *testing.T) {
	m := media.NewMeter() // default threshold -25 dBFS
	for seq := uint64(0); seq < 3000; seq++ {
		require.False(t, m.Process(media.Synth(seq, -30)), "noise floor tripped the meter at frame %d", seq)
	}
}

func TestJitterBufferReorders(t *testing.T) {
	b := media.NewJitterBuffer(5)

	require.Len(t, b.Push(media.Silence(0)), 1)
	require.Empty(t, b.Push(media.Silence(2)), "gap holds frame 2")
	out := b.Push(media.Silence(1))
	require.Len(t, out, 2, "frame 1 releases 1 and 2")
	require.Equal(t, uint64(1), out[0].Seq)
	require.Equal(t, uint64(2), out[1].Seq)
}

func TestJitterBufferConcedesGap(t *testing.T) {
	b := media.NewJitterBuffer(2)
	require.Len(t, b.Push(media.Silence(0)), 1)

	// Frame 1 is lost; pending 2..4 exceed the window and the gap is conceded.
	require.Empty(t, b.Push(media.Silence(2)))
	require.Empty(t, b.Push(media.Silence(3)))
	out := b.Push(media.Silence(4))
	require.Len(t, out, 3)
	require.Equal(t, uint64(2), out[0].Seq)

	// A very late frame 1 is dropped.
	require.Empty(t, b.Push(media.Silence(1)))
}
