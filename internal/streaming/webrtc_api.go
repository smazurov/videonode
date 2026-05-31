package streaming

import (
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"strings"

	"github.com/pion/interceptor"
	"github.com/pion/interceptor/pkg/nack"
	"github.com/pion/interceptor/pkg/report"
	"github.com/pion/interceptor/pkg/stats"
	"github.com/pion/interceptor/pkg/twcc"
	"github.com/pion/rtcp"
	pion "github.com/pion/webrtc/v4"
	"github.com/smazurov/videonode/internal/logging"
)

// NACKBufferSize is the number of packets to buffer for NACK retransmission.
// At 50Mbit/s with ~1400 byte packets, we get ~4500 packets/second.
// 8192 packets = ~1.8 seconds of buffer, which should be sufficient for
// Firefox's NACK requests even under adverse conditions.
const NACKBufferSize = 8192

// SRTPReplayProtectionWindow must be at least as large as NACKBufferSize.
// Google reportedly uses 10000.
const SRTPReplayProtectionWindow = 10000

// NewWebRTCAPI creates a WebRTC API with optimized settings for high-bitrate
// streaming. This uses a larger NACK buffer than the default (64 packets) to
// support retransmission requests from browsers like Firefox that are more
// sensitive to packet loss.
// A sanitized form of peerID is used as the ICE username fragment (ice-ufrag),
// which is visible to the client in the SDP answer for identification.
func NewWebRTCAPI(streamID, peerID string) (*pion.API, error) {
	m := &pion.MediaEngine{}
	if err := registerCodecs(m); err != nil {
		return nil, err
	}

	i := &interceptor.Registry{}
	if err := configureInterceptors(m, i); err != nil {
		return nil, err
	}

	// Add RTCP monitoring interceptor for Prometheus metrics
	i.Add(&rtcpMonitorInterceptorFactory{streamID: streamID, peerID: peerID})

	s := pion.SettingEngine{}
	s.SetDTLSInsecureSkipHelloVerify(true)
	// Set SRTP replay protection window to match NACK buffer
	s.SetSRTPReplayProtectionWindow(SRTPReplayProtectionWindow)
	// Use a sanitized peer ID as ice-ufrag (visible to client in SDP answer)
	s.SetICECredentials(iceUfragFromPeerID(peerID), generateICEPassword())
	// Filter out Docker bridge and veth interfaces from ICE candidates
	s.SetInterfaceFilter(func(iface string) bool {
		if strings.HasPrefix(iface, "docker") || strings.HasPrefix(iface, "br-") || strings.HasPrefix(iface, "veth") {
			return false
		}
		return true
	})

	return pion.NewAPI(
		pion.WithMediaEngine(m),
		pion.WithInterceptorRegistry(i),
		pion.WithSettingEngine(s),
	), nil
}

// generateICEPassword generates a secure password for ICE authentication.
// ICE requires at least 128 bits of randomness for the password. Hex keeps the
// output within RFC 5245's ice-char set (ALPHA / DIGIT / "+" / "/"); base64's
// URL alphabet ("-", "_") is rejected by strict SDP parsers like gstreamer.
func generateICEPassword() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		slog.Error("Failed to generate random bytes for ICE password", logging.KeyError, err)
	}
	return hex.EncodeToString(b)
}

// iceUfragFromPeerID derives an RFC 5245-compliant ice-ufrag from the peer ID.
// Petname-style peer IDs ("causal-treefrog") contain a hyphen, which is outside
// the ice-char set; lenient clients (Chrome, pion) accept it but strict parsers
// (gstreamer) reject the SDP with "invalid 'ice-ufrag' attribute". We keep only
// alphanumerics so the name stays recognizable, padding to the 4-char minimum.
func iceUfragFromPeerID(peerID string) string {
	var b strings.Builder
	for _, r := range peerID {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
		}
	}
	ufrag := b.String()
	if len(ufrag) < 4 {
		ufrag += "peer"
	}
	return ufrag
}

// registerCodecs registers audio and video codecs with RTCP feedback support.
func registerCodecs(m *pion.MediaEngine) error {
	// Audio codecs
	for _, codec := range []pion.RTPCodecParameters{
		{
			RTPCodecCapability: pion.RTPCodecCapability{
				MimeType: pion.MimeTypeOpus, ClockRate: 48000, Channels: 2,
				SDPFmtpLine: "minptime=10;useinbandfec=1",
			},
			PayloadType: 101,
		},
		{
			RTPCodecCapability: pion.RTPCodecCapability{
				MimeType: pion.MimeTypePCMU, ClockRate: 8000,
			},
			PayloadType: 0,
		},
		{
			RTPCodecCapability: pion.RTPCodecCapability{
				MimeType: pion.MimeTypePCMA, ClockRate: 8000,
			},
			PayloadType: 8,
		},
	} {
		if err := m.RegisterCodec(codec, pion.RTPCodecTypeAudio); err != nil {
			return err
		}
	}

	// Video codecs with RTCP feedback (NACK, PLI, FIR, REMB)
	videoRTCPFeedback := []pion.RTCPFeedback{
		{Type: "goog-remb"},
		{Type: "ccm", Parameter: "fir"},
		{Type: "nack"},
		{Type: "nack", Parameter: "pli"},
	}

	for _, codec := range []pion.RTPCodecParameters{
		{
			RTPCodecCapability: pion.RTPCodecCapability{
				MimeType:     pion.MimeTypeH264,
				ClockRate:    90000,
				SDPFmtpLine:  "level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=42001f",
				RTCPFeedback: videoRTCPFeedback,
			},
			PayloadType: 96,
		},
		{
			RTPCodecCapability: pion.RTPCodecCapability{
				MimeType:     pion.MimeTypeH264,
				ClockRate:    90000,
				SDPFmtpLine:  "level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=42e01f",
				RTCPFeedback: videoRTCPFeedback,
			},
			PayloadType: 97,
		},
		{
			// High Profile Level 3.1 (common browser default)
			RTPCodecCapability: pion.RTPCodecCapability{
				MimeType:     pion.MimeTypeH264,
				ClockRate:    90000,
				SDPFmtpLine:  "level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=64001f",
				RTCPFeedback: videoRTCPFeedback,
			},
			PayloadType: 98,
		},
		{
			// High Profile Level 4.0 (1080p30)
			RTPCodecCapability: pion.RTPCodecCapability{
				MimeType:     pion.MimeTypeH264,
				ClockRate:    90000,
				SDPFmtpLine:  "level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=640028",
				RTCPFeedback: videoRTCPFeedback,
			},
			PayloadType: 99,
		},
		{
			// High Profile Level 5.0 (4K30)
			RTPCodecCapability: pion.RTPCodecCapability{
				MimeType:     pion.MimeTypeH264,
				ClockRate:    90000,
				SDPFmtpLine:  "level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=640032",
				RTCPFeedback: videoRTCPFeedback,
			},
			PayloadType: 105,
		},
		{
			// High Profile Level 5.2 (4K60)
			RTPCodecCapability: pion.RTPCodecCapability{
				MimeType:     pion.MimeTypeH264,
				ClockRate:    90000,
				SDPFmtpLine:  "level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=640034",
				RTCPFeedback: videoRTCPFeedback,
			},
			PayloadType: 102,
		},
		{
			// High Profile Level 5.2 with constraint flags (common from x264)
			RTPCodecCapability: pion.RTPCodecCapability{
				MimeType:     pion.MimeTypeH264,
				ClockRate:    90000,
				SDPFmtpLine:  "level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=640c34",
				RTCPFeedback: videoRTCPFeedback,
			},
			PayloadType: 104,
		},
		{
			RTPCodecCapability: pion.RTPCodecCapability{
				MimeType:     pion.MimeTypeH265,
				ClockRate:    90000,
				RTCPFeedback: videoRTCPFeedback,
			},
			PayloadType: 103,
		},
	} {
		if err := m.RegisterCodec(codec, pion.RTPCodecTypeVideo); err != nil {
			return err
		}
	}

	return nil
}

// configureInterceptors sets up NACK, RTCP reports, and TWCC with optimized
// buffer sizes for high-bitrate streaming.
func configureInterceptors(m *pion.MediaEngine, i *interceptor.Registry) error {
	// NACK generator (for requesting retransmissions)
	generator, err := nack.NewGeneratorInterceptor()
	if err != nil {
		return err
	}

	// NACK responder with large buffer for high-bitrate streams
	responder, err := nack.NewResponderInterceptor(
		nack.ResponderSize(NACKBufferSize),
	)
	if err != nil {
		return err
	}

	i.Add(responder)
	i.Add(generator)

	// RTCP sender/receiver reports
	receiver, err := report.NewReceiverInterceptor()
	if err != nil {
		return err
	}
	sender, err := report.NewSenderInterceptor()
	if err != nil {
		return err
	}
	i.Add(receiver)
	i.Add(sender)

	// Stats interceptor
	statsInterceptor, err := stats.NewInterceptor()
	if err != nil {
		return err
	}
	i.Add(statsInterceptor)

	// TWCC for congestion control
	m.RegisterFeedback(pion.RTCPFeedback{Type: pion.TypeRTCPFBTransportCC}, pion.RTPCodecTypeVideo)
	m.RegisterFeedback(pion.RTCPFeedback{Type: pion.TypeRTCPFBTransportCC}, pion.RTPCodecTypeAudio)

	twccGenerator, err := twcc.NewSenderInterceptor()
	if err != nil {
		return err
	}
	i.Add(twccGenerator)

	return nil
}

// rtcpMonitorInterceptorFactory creates RTCP monitoring interceptors for metrics.
type rtcpMonitorInterceptorFactory struct {
	streamID string
	peerID   string
}

// NewInterceptor creates a new RTCP monitoring interceptor.
func (f *rtcpMonitorInterceptorFactory) NewInterceptor(_ string) (interceptor.Interceptor, error) {
	return &rtcpMonitorInterceptor{streamID: f.streamID, peerID: f.peerID}, nil
}

// rtcpMonitorInterceptor monitors RTCP packets and updates Prometheus metrics.
type rtcpMonitorInterceptor struct {
	interceptor.NoOp
	streamID string
	peerID   string
}

// BindRTCPReader wraps the RTCP reader to monitor incoming packets.
func (r *rtcpMonitorInterceptor) BindRTCPReader(reader interceptor.RTCPReader) interceptor.RTCPReader {
	return &rtcpMonitorReader{reader: reader, streamID: r.streamID, peerID: r.peerID}
}

type rtcpMonitorReader struct {
	reader   interceptor.RTCPReader
	streamID string
	peerID   string
}

func (r *rtcpMonitorReader) Read(b []byte, a interceptor.Attributes) (int, interceptor.Attributes, error) {
	n, attr, err := r.reader.Read(b, a)
	if err != nil {
		return n, attr, err
	}

	packets, parseErr := rtcp.Unmarshal(b[:n])
	if parseErr != nil {
		return n, attr, err
	}

	for _, pkt := range packets {
		IncrementRTCPPackets(r.streamID, r.peerID)
		switch p := pkt.(type) {
		case *rtcp.TransportLayerNack:
			count := 0
			for _, nack := range p.Nacks {
				count += len(nack.PacketList())
			}
			IncrementNACKs(r.streamID, r.peerID, count)
		case *rtcp.PictureLossIndication:
			IncrementPLIs(r.streamID, r.peerID)
		case *rtcp.FullIntraRequest:
			IncrementFIRs(r.streamID, r.peerID)
		case *rtcp.ReceiverReport:
			for _, report := range p.Reports {
				RecordJitter(r.streamID, r.peerID, report.Jitter)
			}
		}
	}

	return n, attr, err
}
