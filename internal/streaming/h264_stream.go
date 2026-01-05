package streaming

import (
	"encoding/base64"
	"strings"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/pion/rtp"
)

// h264StreamHandler provides RTP passthrough for H264 with SPS/PPS injection.
// It forwards packets directly without reassembly. For late-joining clients,
// it injects SPS/PPS from the codec's fmtp line before the first IDR frame.
type h264StreamHandler struct {
	handler     func(*rtp.Packet)
	sps, pps    []byte
	payloadType uint8
	sentPS      bool // true once we've seen or injected SPS/PPS
}

func newH264StreamHandler(codec *core.Codec, handler func(*rtp.Packet)) *h264StreamHandler {
	sps, pps := parseSpsPps(codec.FmtpLine)
	return &h264StreamHandler{
		handler:     handler,
		sps:         sps,
		pps:         pps,
		payloadType: codec.PayloadType,
	}
}

func (h *h264StreamHandler) handlePacket(packet *rtp.Packet) {
	if len(packet.Payload) == 0 {
		return
	}

	nalType := packet.Payload[0] & 0x1F

	// Inject SPS/PPS before EVERY IDR frame, not just the first.
	// This helps Firefox/OpenH264 recover after packet loss.
	switch nalType {
	case 7, 8: // SPS or PPS in stream - pass through, skip injection for this IDR
		h.sentPS = true
	case 24: // STAP-A - if it contains SPS/PPS, skip injection for this IDR
		if h.stapAContainsPS(packet.Payload) {
			h.sentPS = true
		}
	case 5: // IDR frame - inject SPS/PPS before it (unless just received in-band)
		if !h.sentPS {
			h.injectParameterSets(packet)
		}
		h.sentPS = false // Reset so we inject again on next IDR
	case 28: // FU-A - check if start of IDR
		if len(packet.Payload) >= 2 {
			fuHeader := packet.Payload[1]
			isStart := fuHeader&0x80 != 0
			fragNalType := fuHeader & 0x1F
			if isStart && fragNalType == 5 {
				if !h.sentPS {
					h.injectParameterSets(packet)
				}
				h.sentPS = false // Reset for next IDR
			}
		}
	}

	h.handler(packet)
}

// stapAContainsPS checks if a STAP-A packet contains SPS or PPS.
func (h *h264StreamHandler) stapAContainsPS(payload []byte) bool {
	offset := 1
	for offset+2 <= len(payload) {
		nalSize := int(payload[offset])<<8 | int(payload[offset+1])
		offset += 2
		if offset+nalSize > len(payload) || nalSize == 0 {
			break
		}
		nalType := payload[offset] & 0x1F
		if nalType == 7 || nalType == 8 {
			return true
		}
		offset += nalSize
	}
	return false
}

func (h *h264StreamHandler) injectParameterSets(template *rtp.Packet) {
	if len(h.sps) > 0 {
		h.sendNAL(template, h.sps)
	}
	if len(h.pps) > 0 {
		h.sendNAL(template, h.pps)
	}
	h.sentPS = true
}

func (h *h264StreamHandler) sendNAL(template *rtp.Packet, nal []byte) {
	pkt := &rtp.Packet{
		Header: rtp.Header{
			Version:     2,
			PayloadType: h.payloadType,
			Timestamp:   template.Timestamp,
			SSRC:        template.SSRC,
		},
		Payload: nal,
	}
	h.handler(pkt)
}

// parseSpsPps extracts SPS and PPS from the codec's fmtp line.
func parseSpsPps(fmtpLine string) (sps, pps []byte) {
	const prefix = "sprop-parameter-sets="

	idx := strings.Index(fmtpLine, prefix)
	if idx < 0 {
		return nil, nil
	}

	value := fmtpLine[idx+len(prefix):]
	if semi := strings.Index(value, ";"); semi >= 0 {
		value = value[:semi]
	}

	parts := strings.SplitN(value, ",", 2)
	if len(parts) != 2 {
		return nil, nil
	}

	sps, _ = base64.StdEncoding.DecodeString(parts[0])
	pps, _ = base64.StdEncoding.DecodeString(parts[1])
	return sps, pps
}
