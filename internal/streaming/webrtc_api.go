package streaming

import (
	"github.com/pion/interceptor"
	"github.com/pion/interceptor/pkg/nack"
	"github.com/pion/interceptor/pkg/report"
	"github.com/pion/interceptor/pkg/stats"
	"github.com/pion/interceptor/pkg/twcc"
	"github.com/pion/rtcp"
	pion "github.com/pion/webrtc/v4"
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
func NewWebRTCAPI(streamID string) (*pion.API, error) {
	m := &pion.MediaEngine{}
	if err := registerCodecs(m); err != nil {
		return nil, err
	}

	i := &interceptor.Registry{}
	if err := configureInterceptors(m, i); err != nil {
		return nil, err
	}

	// Add RTCP monitoring interceptor for Prometheus metrics
	i.Add(&rtcpMonitorInterceptorFactory{streamID: streamID})

	s := pion.SettingEngine{}
	s.SetDTLSInsecureSkipHelloVerify(true)
	// Set SRTP replay protection window to match NACK buffer
	s.SetSRTPReplayProtectionWindow(SRTPReplayProtectionWindow)

	return pion.NewAPI(
		pion.WithMediaEngine(m),
		pion.WithInterceptorRegistry(i),
		pion.WithSettingEngine(s),
	), nil
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
			PayloadType: 101,
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

	m.RegisterFeedback(pion.RTCPFeedback{Type: "nack"}, pion.RTPCodecTypeVideo)
	m.RegisterFeedback(pion.RTCPFeedback{Type: "nack", Parameter: "pli"}, pion.RTPCodecTypeVideo)
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
}

func (f *rtcpMonitorInterceptorFactory) NewInterceptor(_ string) (interceptor.Interceptor, error) {
	return &rtcpMonitorInterceptor{streamID: f.streamID}, nil
}

// rtcpMonitorInterceptor monitors RTCP packets and updates Prometheus metrics.
type rtcpMonitorInterceptor struct {
	interceptor.NoOp
	streamID string
}

func (r *rtcpMonitorInterceptor) BindRTCPReader(reader interceptor.RTCPReader) interceptor.RTCPReader {
	return &rtcpMonitorReader{reader: reader, streamID: r.streamID}
}

type rtcpMonitorReader struct {
	reader   interceptor.RTCPReader
	streamID string
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
		IncrementRTCPPackets(r.streamID)
		switch p := pkt.(type) {
		case *rtcp.TransportLayerNack:
			count := 0
			for _, nack := range p.Nacks {
				count += 1 + len(nack.PacketList())
			}
			IncrementNACKs(r.streamID, count)
		case *rtcp.PictureLossIndication:
			IncrementPLIs(r.streamID)
		case *rtcp.FullIntraRequest:
			IncrementFIRs()
		}
	}

	return n, attr, err
}
