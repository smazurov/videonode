package streaming

import (
	"testing"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/pion/rtp"
)

func TestParseSpsPps(t *testing.T) {
	// Real-world fmtp line from an RTSP camera
	fmtp := "profile-level-id=42e01f;packetization-mode=1;sprop-parameter-sets=Z0IAKeKQFAe2AtwEBAaQeJEV,aM48gA=="

	sps, pps := parseSpsPps(fmtp)

	if len(sps) == 0 {
		t.Error("SPS should not be empty")
	}
	if len(pps) == 0 {
		t.Error("PPS should not be empty")
	}

	// Check SPS NAL type (should be 7)
	if sps[0]&0x1F != 7 {
		t.Errorf("SPS NAL type should be 7, got %d", sps[0]&0x1F)
	}

	// Check PPS NAL type (should be 8)
	if pps[0]&0x1F != 8 {
		t.Errorf("PPS NAL type should be 8, got %d", pps[0]&0x1F)
	}
}

func TestH264StreamHandler_Passthrough(t *testing.T) {
	codec := &core.Codec{
		Name:        core.CodecH264,
		PayloadType: 96,
		FmtpLine:    "sprop-parameter-sets=Z0IAKeKQFAe2AtwEBAaQeJEV,aM48gA==",
	}

	var received []*rtp.Packet
	handler := newH264StreamHandler(codec, func(pkt *rtp.Packet) {
		// Clone to avoid pointer reuse issues in test
		clone := *pkt
		received = append(received, &clone)
	})

	// Send a P-frame (NAL type 1) - should pass through directly
	pframe := &rtp.Packet{
		Header:  rtp.Header{PayloadType: 96, Timestamp: 1000, SSRC: 12345},
		Payload: []byte{0x01, 0x00, 0x00, 0x00}, // NAL type 1 (P-frame)
	}
	handler.handlePacket(pframe)

	if len(received) != 1 {
		t.Fatalf("Expected 1 packet, got %d", len(received))
	}

	if received[0].Payload[0]&0x1F != 1 {
		t.Errorf("Expected NAL type 1, got %d", received[0].Payload[0]&0x1F)
	}
}

func TestH264StreamHandler_IFrameWithInjection(t *testing.T) {
	codec := &core.Codec{
		Name:        core.CodecH264,
		PayloadType: 96,
		FmtpLine:    "sprop-parameter-sets=Z0IAKeKQFAe2AtwEBAaQeJEV,aM48gA==",
	}

	var received []*rtp.Packet
	handler := newH264StreamHandler(codec, func(pkt *rtp.Packet) {
		clone := *pkt
		clone.Payload = append([]byte{}, pkt.Payload...) // Deep copy payload
		received = append(received, &clone)
	})

	// Send an I-frame (NAL type 5) - should trigger SPS/PPS injection
	iframe := &rtp.Packet{
		Header:  rtp.Header{PayloadType: 96, SequenceNumber: 100, Timestamp: 1000, SSRC: 12345, Marker: true},
		Payload: []byte{0x65, 0x00, 0x00, 0x00}, // NAL type 5 (IDR)
	}
	handler.handlePacket(iframe)

	// Should receive SPS + PPS + IDR = 3 packets
	if len(received) != 3 {
		t.Fatalf("Expected 3 packets (SPS, PPS, IDR), got %d", len(received))
	}

	// Check SPS (type 7)
	if received[0].Payload[0]&0x1F != 7 {
		t.Errorf("First packet should be SPS (type 7), got %d", received[0].Payload[0]&0x1F)
	}

	// Check PPS (type 8)
	if received[1].Payload[0]&0x1F != 8 {
		t.Errorf("Second packet should be PPS (type 8), got %d", received[1].Payload[0]&0x1F)
	}

	// Check IDR (type 5)
	if received[2].Payload[0]&0x1F != 5 {
		t.Errorf("Third packet should be IDR (type 5), got %d", received[2].Payload[0]&0x1F)
	}

	// All should share the same timestamp
	if received[0].Timestamp != 1000 || received[1].Timestamp != 1000 || received[2].Timestamp != 1000 {
		t.Error("All packets should have the same timestamp")
	}

	// SPS/PPS should not have marker bit, IDR should preserve it
	if received[0].Marker || received[1].Marker {
		t.Error("SPS/PPS should not have marker bit")
	}
	if !received[2].Marker {
		t.Error("IDR should have marker bit")
	}

	// sentPS should be false (reset for next IDR)
	if handler.sentPS {
		t.Error("sentPS should be false after IDR (reset for next injection)")
	}

	// Next IDR should also trigger injection
	iframe2 := &rtp.Packet{
		Header:  rtp.Header{PayloadType: 96, SequenceNumber: 101, Timestamp: 2000, SSRC: 12345, Marker: true},
		Payload: []byte{0x65, 0x00, 0x00, 0x00}, // NAL type 5 (IDR)
	}
	handler.handlePacket(iframe2)

	// Should have 6 packets total (first 3 + SPS + PPS + second IDR)
	if len(received) != 6 {
		t.Fatalf("Expected 6 packets (second IDR should also inject), got %d", len(received))
	}
}

func TestH264StreamHandler_FUAPassthrough(t *testing.T) {
	codec := &core.Codec{
		Name:        core.CodecH264,
		PayloadType: 96,
		FmtpLine:    "sprop-parameter-sets=Z0IAKeKQFAe2AtwEBAaQeJEV,aM48gA==",
	}

	var received []*rtp.Packet
	handler := newH264StreamHandler(codec, func(pkt *rtp.Packet) {
		clone := *pkt
		received = append(received, &clone)
	})

	// First mark as having received SPS/PPS (simulating stream already running)
	handler.sentPS = true

	// Send FU-A start fragment (P-frame fragment, NAL type 1)
	fuaStart := &rtp.Packet{
		Header: rtp.Header{PayloadType: 96, Timestamp: 1000, SSRC: 12345},
		Payload: []byte{
			0x7C,       // FU indicator: NRI=3, Type=28 (FU-A)
			0x81,       // FU header: S=1, E=0, Type=1 (P-frame)
			0x00, 0x00, // fragment data
		},
	}
	handler.handlePacket(fuaStart)

	// Send FU-A end fragment
	fuaEnd := &rtp.Packet{
		Header: rtp.Header{PayloadType: 96, Timestamp: 1000, SSRC: 12345, Marker: true},
		Payload: []byte{
			0x7C,       // FU indicator
			0x41,       // FU header: S=0, E=1, Type=1
			0x00, 0x00, // fragment data
		},
	}
	handler.handlePacket(fuaEnd)

	// Both fragments should pass through directly
	if len(received) != 2 {
		t.Fatalf("Expected 2 packets, got %d", len(received))
	}
}

func TestH264StreamHandler_FUAIDRInjection(t *testing.T) {
	codec := &core.Codec{
		Name:        core.CodecH264,
		PayloadType: 96,
		FmtpLine:    "sprop-parameter-sets=Z0IAKeKQFAe2AtwEBAaQeJEV,aM48gA==",
	}

	var received []*rtp.Packet
	handler := newH264StreamHandler(codec, func(pkt *rtp.Packet) {
		clone := *pkt
		clone.Payload = append([]byte{}, pkt.Payload...)
		received = append(received, &clone)
	})

	// Send FU-A start fragment for IDR (NAL type 5) - should trigger injection
	fuaStart := &rtp.Packet{
		Header: rtp.Header{PayloadType: 96, Timestamp: 1000, SSRC: 12345},
		Payload: []byte{
			0x7C,       // FU indicator: NRI=3, Type=28 (FU-A)
			0x85,       // FU header: S=1, E=0, Type=5 (IDR)
			0x00, 0x00, // fragment data
		},
	}
	handler.handlePacket(fuaStart)

	// Should receive SPS + PPS + FU-A fragment = 3 packets
	if len(received) != 3 {
		t.Fatalf("Expected 3 packets (SPS, PPS, FU-A start), got %d", len(received))
	}

	// Check SPS (type 7)
	if received[0].Payload[0]&0x1F != 7 {
		t.Errorf("First packet should be SPS (type 7), got %d", received[0].Payload[0]&0x1F)
	}

	// Check PPS (type 8)
	if received[1].Payload[0]&0x1F != 8 {
		t.Errorf("Second packet should be PPS (type 8), got %d", received[1].Payload[0]&0x1F)
	}

	// Check FU-A (type 28)
	if received[2].Payload[0]&0x1F != 28 {
		t.Errorf("Third packet should be FU-A (type 28), got %d", received[2].Payload[0]&0x1F)
	}

	// sentPS should be false (reset after IDR for next injection)
	if handler.sentPS {
		t.Error("sentPS should be false after IDR (reset for next IDR)")
	}

	// Non-start FU-A fragments should not trigger injection
	fuaEnd := &rtp.Packet{
		Header: rtp.Header{PayloadType: 96, Timestamp: 1000, SSRC: 12345, Marker: true},
		Payload: []byte{
			0x7C,       // FU indicator
			0x45,       // FU header: S=0, E=1, Type=5
			0x00, 0x00, // fragment data
		},
	}
	handler.handlePacket(fuaEnd)

	// Should now have 4 packets total (no injection for non-start fragment)
	if len(received) != 4 {
		t.Fatalf("Expected 4 packets total, got %d", len(received))
	}

	// NEXT IDR start fragment should trigger injection again
	fuaStart2 := &rtp.Packet{
		Header: rtp.Header{PayloadType: 96, Timestamp: 2000, SSRC: 12345},
		Payload: []byte{
			0x7C,       // FU indicator: NRI=3, Type=28 (FU-A)
			0x85,       // FU header: S=1, E=0, Type=5 (IDR)
			0x00, 0x00, // fragment data
		},
	}
	handler.handlePacket(fuaStart2)

	// Should have 7 packets total (previous 4 + SPS + PPS + new FU-A start)
	if len(received) != 7 {
		t.Fatalf("Expected 7 packets total (second IDR should inject), got %d", len(received))
	}
}

func TestH264StreamHandler_STAPAPassthrough(t *testing.T) {
	codec := &core.Codec{
		Name:        core.CodecH264,
		PayloadType: 96,
		FmtpLine:    "sprop-parameter-sets=Z0IAKeKQFAe2AtwEBAaQeJEV,aM48gA==",
	}

	var received []*rtp.Packet
	handler := newH264StreamHandler(codec, func(pkt *rtp.Packet) {
		clone := *pkt
		clone.Payload = append([]byte{}, pkt.Payload...)
		received = append(received, &clone)
	})

	// Build a STAP-A packet containing SPS + PPS
	sps := []byte{0x67, 0x42, 0x00, 0x1f} // NAL type 7 (SPS)
	pps := []byte{0x68, 0xce, 0x3c, 0x80} // NAL type 8 (PPS)

	stapa := []byte{0x18} // STAP-A indicator (type 24)
	// Add SPS
	stapa = append(stapa, byte(len(sps)>>8), byte(len(sps)))
	stapa = append(stapa, sps...)
	// Add PPS
	stapa = append(stapa, byte(len(pps)>>8), byte(len(pps)))
	stapa = append(stapa, pps...)

	packet := &rtp.Packet{
		Header:  rtp.Header{PayloadType: 96, SequenceNumber: 200, Timestamp: 1000, SSRC: 12345, Marker: true},
		Payload: stapa,
	}
	handler.handlePacket(packet)

	// STAP-A should pass through as single packet (not unpacked)
	if len(received) != 1 {
		t.Fatalf("Expected 1 packet (STAP-A passthrough), got %d", len(received))
	}

	// Check it's still STAP-A (type 24)
	if received[0].Payload[0]&0x1F != 24 {
		t.Errorf("Packet should be STAP-A (type 24), got %d", received[0].Payload[0]&0x1F)
	}

	// Check sequence number
	if received[0].SequenceNumber != 200 {
		t.Errorf("Expected sequence number 200, got %d", received[0].SequenceNumber)
	}

	// Marker bit should be preserved
	if !received[0].Marker {
		t.Error("Marker bit should be preserved")
	}

	// sentPS should be true because STAP-A contained SPS/PPS
	if !handler.sentPS {
		t.Error("sentPS should be true after STAP-A with SPS/PPS")
	}

	// Immediately following IDR should NOT trigger injection (just saw SPS/PPS)
	iframe := &rtp.Packet{
		Header:  rtp.Header{PayloadType: 96, SequenceNumber: 201, Timestamp: 2000, SSRC: 12345, Marker: true},
		Payload: []byte{0x65, 0x00, 0x00, 0x00}, // NAL type 5 (IDR)
	}
	handler.handlePacket(iframe)

	// Should have 2 packets (STAP-A + IDR), no injection because STAP-A had SPS/PPS
	if len(received) != 2 {
		t.Fatalf("Expected 2 packets (STAP-A + IDR, no injection), got %d", len(received))
	}

	// But NEXT IDR should trigger injection (sentPS was reset)
	iframe2 := &rtp.Packet{
		Header:  rtp.Header{PayloadType: 96, SequenceNumber: 202, Timestamp: 3000, SSRC: 12345, Marker: true},
		Payload: []byte{0x65, 0x00, 0x00, 0x00}, // NAL type 5 (IDR)
	}
	handler.handlePacket(iframe2)

	// Should have 5 packets (STAP-A + IDR + SPS + PPS + IDR)
	if len(received) != 5 {
		t.Fatalf("Expected 5 packets (second IDR should trigger injection), got %d", len(received))
	}
}

func TestH264StreamHandler_StapAContainsPS(t *testing.T) {
	codec := &core.Codec{
		Name:        core.CodecH264,
		PayloadType: 96,
		FmtpLine:    "sprop-parameter-sets=Z0IAKeKQFAe2AtwEBAaQeJEV,aM48gA==",
	}

	handler := newH264StreamHandler(codec, func(_ *rtp.Packet) {})

	// STAP-A with SPS + PPS
	sps := []byte{0x67, 0x42, 0x00, 0x1f}
	pps := []byte{0x68, 0xce, 0x3c, 0x80}
	stapa := []byte{0x18}
	stapa = append(stapa, byte(len(sps)>>8), byte(len(sps)))
	stapa = append(stapa, sps...)
	stapa = append(stapa, byte(len(pps)>>8), byte(len(pps)))
	stapa = append(stapa, pps...)

	if !handler.stapAContainsPS(stapa) {
		t.Error("stapAContainsPS should return true for STAP-A with SPS/PPS")
	}

	// STAP-A without parameter sets (just P-frame slices)
	slice := []byte{0x01, 0x00, 0x00} // NAL type 1
	stapaNoPS := []byte{0x18}
	stapaNoPS = append(stapaNoPS, byte(len(slice)>>8), byte(len(slice)))
	stapaNoPS = append(stapaNoPS, slice...)

	if handler.stapAContainsPS(stapaNoPS) {
		t.Error("stapAContainsPS should return false for STAP-A without SPS/PPS")
	}
}
