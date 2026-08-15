package asterisk

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRTPRoundTrip(t *testing.T) {
	payload := encodePayload16k([]int16{1, -1, 32000, -32000})
	datagram := buildRTP(42, 1234, 0xdeadbeef, payload)

	pkt, err := parseRTP(datagram)
	require.NoError(t, err)
	require.Equal(t, uint16(42), pkt.Seq)
	require.Equal(t, uint32(1234), pkt.Timestamp)
	require.Equal(t, uint32(0xdeadbeef), pkt.SSRC)
	require.Equal(t, []int16{1, -1, 32000, -32000}, decodePayload16k(pkt.Payload))
}

func TestParseRTPRejectsGarbage(t *testing.T) {
	_, err := parseRTP([]byte{1, 2, 3})
	require.Error(t, err, "short datagram")

	bad := make([]byte, 20)
	bad[0] = 1 << 6 // version 1
	_, err = parseRTP(bad)
	require.Error(t, err, "wrong version")
}

func TestResampleRoundTrip(t *testing.T) {
	// A constant signal is preserved exactly by both directions.
	in := make([]int16, 160)
	for i := range in {
		in[i] = 1000
	}
	up := upsample8to16(in)
	require.Len(t, up, 320)
	down := downsample16to8(up)
	require.Equal(t, in, down)
}

func TestDownsampleAveragesPairs(t *testing.T) {
	require.Equal(t, []int16{15, -15}, downsample16to8([]int16{10, 20, -10, -20}))
}
