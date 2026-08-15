package asterisk

import (
	"encoding/binary"
	"fmt"
)

// Wire format of the External Media leg: RTP over UDP carrying slin16 —
// PCM linear 16-bit mono at 16kHz, big-endian payload (the RTP L16
// convention). One packet per 20ms: 320 samples, 640 payload bytes.
const (
	wireSampleRate      = 16000
	wireSamplesPerFrame = wireSampleRate / 1000 * 20
	wirePayloadBytes    = wireSamplesPerFrame * 2
	rtpHeaderBytes      = 12
	rtpVersion          = 2
	// rtpPayloadTypeL16 is the dynamic payload type Asterisk assigns to
	// slin16 on external media legs; we echo it on the return path.
	rtpPayloadTypeL16 = 118
)

// rtpPacket is one parsed RTP datagram (the fields this backend uses).
type rtpPacket struct {
	Seq       uint16
	Timestamp uint32
	SSRC      uint32
	Payload   []byte
}

// parseRTP decodes the fixed RTP header. CSRC lists and header extensions
// are not used by Asterisk external media and are rejected as malformed.
func parseRTP(datagram []byte) (rtpPacket, error) {
	if len(datagram) < rtpHeaderBytes {
		return rtpPacket{}, fmt.Errorf("asterisk: rtp datagram too short: %d bytes", len(datagram))
	}
	if v := datagram[0] >> 6; v != rtpVersion {
		return rtpPacket{}, fmt.Errorf("asterisk: rtp version %d unsupported", v)
	}
	if csrcCount := datagram[0] & 0x0f; csrcCount != 0 {
		return rtpPacket{}, fmt.Errorf("asterisk: rtp csrc count %d unsupported", csrcCount)
	}
	return rtpPacket{
		Seq:       binary.BigEndian.Uint16(datagram[2:4]),
		Timestamp: binary.BigEndian.Uint32(datagram[4:8]),
		SSRC:      binary.BigEndian.Uint32(datagram[8:12]),
		Payload:   datagram[rtpHeaderBytes:],
	}, nil
}

// buildRTP encodes one outbound RTP datagram.
func buildRTP(seq uint16, timestamp, ssrc uint32, payload []byte) []byte {
	out := make([]byte, rtpHeaderBytes+len(payload))
	out[0] = rtpVersion << 6
	out[1] = rtpPayloadTypeL16
	binary.BigEndian.PutUint16(out[2:4], seq)
	binary.BigEndian.PutUint32(out[4:8], timestamp)
	binary.BigEndian.PutUint32(out[8:12], ssrc)
	copy(out[rtpHeaderBytes:], payload)
	return out
}

// decodePayload16k turns a big-endian slin16 payload into samples.
func decodePayload16k(payload []byte) []int16 {
	samples := make([]int16, len(payload)/2)
	for i := range samples {
		samples[i] = int16(binary.BigEndian.Uint16(payload[i*2:]))
	}
	return samples
}

// encodePayload16k turns samples into a big-endian slin16 payload.
func encodePayload16k(samples []int16) []byte {
	payload := make([]byte, len(samples)*2)
	for i, s := range samples {
		binary.BigEndian.PutUint16(payload[i*2:], uint16(s))
	}
	return payload
}

// downsample16to8 halves the sample rate by averaging sample pairs (a cheap
// low-pass + decimate — enough for the 8kHz telephony leg the gateway's
// internal/media contract expects).
func downsample16to8(in []int16) []int16 {
	out := make([]int16, len(in)/2)
	for i := range out {
		out[i] = int16((int32(in[2*i]) + int32(in[2*i+1])) / 2)
	}
	return out
}

// upsample8to16 doubles the sample rate by linear interpolation.
func upsample8to16(in []int16) []int16 {
	out := make([]int16, len(in)*2)
	for i := range in {
		out[2*i] = in[i]
		next := in[i]
		if i+1 < len(in) {
			next = in[i+1]
		}
		out[2*i+1] = int16((int32(in[i]) + int32(next)) / 2)
	}
	return out
}
